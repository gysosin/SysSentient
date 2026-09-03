// Package auth owns credentials: password hashing, session tokens, roles and
// the one-time first-run setup token. It deliberately has no dependency on
// net/http or storage so every piece can be tested in isolation.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	// MinPasswordLength follows NIST 800-63B: length over composition rules.
	MinPasswordLength = 12
	// MaxPasswordLength bounds hashing cost; argon2 has no bcrypt-style cap.
	MaxPasswordLength = 128

	// Above the OWASP minimum (19 MiB, t=2). 64 MiB per attempt is fine on a
	// monitoring server; the login rate limiter bounds concurrent hashes.
	argonMemory  uint32 = 64 * 1024 // KiB
	argonTime    uint32 = 3
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	saltLength          = 16
	// maxKeyLength bounds the key size read back out of a stored hash, so the
	// value handed to argon2 can never be attacker-chosen and unbounded.
	maxKeyLength = 1024
)

var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d characters", MaxPasswordLength)
	ErrMalformedHash    = errors.New("malformed password hash")
)

// ValidatePassword applies the length policy. Length is counted in runes so a
// user typing non-ASCII characters is not penalised.
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if n > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns a PHC-formatted argon2id hash. Parameters travel with
// the hash so they can be raised later without a migration.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the PHC hash. A mismatch is
// (false, nil); only an unparseable hash is an error.
func VerifyPassword(encoded, password string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	// decodeHash bounds len(key) to maxKeyLength, so this conversion cannot
	// overflow.
	candidate := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(key))) // #nosec G115 -- bounded by maxKeyLength in decodeHash
	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<key>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	// Strict: the non-strict decoder silently ignores the trailing padding
	// bits of the final base64 character, so two different encodings decode to
	// the same bytes. A 32-byte key encodes to 43 characters whose last one
	// carries only 4 significant bits, which means a stored hash ending in "A"
	// and the same hash ending in "B" verify identically. A stored hash is
	// data, not trusted input, and it has exactly one canonical encoding —
	// anything else is corrupt or tampered with and must not verify.
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	// An upper bound matters: the decoded length is fed to argon2 as the
	// requested key size, so a corrupt or hostile row could otherwise ask for
	// an enormous allocation. Real argon2id outputs are tens of bytes.
	if err != nil || len(key) == 0 || len(key) > maxKeyLength {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	return p, salt, key, nil
}

// dummyHash is verified against when a login names an email that does not
// exist, so the response takes as long as a real mismatch and the timing does
// not enumerate accounts. Computed lazily: hashing costs 64 MiB and tens of
// milliseconds, which tests that never log in should not pay.
var dummyHash = sync.OnceValue(func() string {
	h, err := HashPassword("timing-equaliser-not-a-real-password")
	if err != nil {
		panic(err)
	}
	return h
})

// VerifyDummy burns the same work as a real verification and discards the
// result.
func VerifyDummy(password string) {
	_, _ = VerifyPassword(dummyHash(), password)
}
