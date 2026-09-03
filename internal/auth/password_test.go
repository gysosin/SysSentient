package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword(encoded, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected the original password to verify")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	encoded, _ := HashPassword("correct horse battery staple")
	ok, err := VerifyPassword(encoded, "correct horse battery stapl3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	t.Parallel()
	a, _ := HashPassword("correct horse battery staple")
	b, _ := HashPassword("correct horse battery staple")
	if a == b {
		t.Fatal("two hashes of the same password must differ (salt)")
	}
}

func TestHashPasswordEncodesPHCWithParameters(t *testing.T) {
	t.Parallel()
	encoded, _ := HashPassword("correct horse battery staple")
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected PHC prefix: %s", encoded)
	}
}

func TestPasswordPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		password string
		want     error
	}{
		{"eleven chars rejected", "abcdefghijk", ErrPasswordTooShort},
		{"twelve chars accepted", "abcdefghijkl", nil},
		{"twelve two-byte runes accepted", strings.Repeat("α", 12), nil},
		{"129 chars rejected", strings.Repeat("a", 129), ErrPasswordTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidatePassword(tc.password); !errors.Is(got, tc.want) {
				t.Fatalf("ValidatePassword(%q) = %v, want %v", tc.password, got, tc.want)
			}
		})
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "plaintext", "$argon2id$v=19$m=65536,t=3,p=2$onlysalt", "$bcrypt$x$y$z$w"} {
		if _, err := VerifyPassword(bad, "whatever-password"); !errors.Is(err, ErrMalformedHash) {
			t.Errorf("VerifyPassword(%q) error = %v, want ErrMalformedHash", bad, err)
		}
	}
}

func TestVerifyPasswordRejectsTamperedHash(t *testing.T) {
	t.Parallel()
	const password = "correct horse battery staple"
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Tamper with a character in the middle of the key rather than the last
	// one. The final base64 character of a 32-byte key carries only 4
	// significant bits, so flipping it can leave the decoded bytes identical —
	// which made the previous version of this test fail roughly one run in
	// sixteen and look like an authentication bypass.
	i := len(encoded) - 8
	swap := byte('A')
	if encoded[i] == 'A' {
		swap = 'B'
	}
	tampered := encoded[:i] + string(swap) + encoded[i+1:]
	if tampered == encoded {
		t.Fatal("tamper produced an identical string")
	}

	ok, _ := VerifyPassword(tampered, password)
	if ok {
		t.Fatal("tampered hash verified")
	}
}

// The final base64 character of the key holds 4 data bits and 2 padding bits.
// A non-strict decoder ignores those padding bits, so two distinct strings
// decode to the same key and both verify. Anything but the canonical encoding
// must be rejected outright.
func TestVerifyPasswordRejectsNonCanonicalBase64(t *testing.T) {
	t.Parallel()
	const password = "correct horse battery staple"
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	last := encoded[len(encoded)-1]
	// Canonical final characters have their low two bits clear; setting one
	// produces a different string that decodes to identical bytes.
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	idx := strings.IndexByte(alphabet, last)
	if idx < 0 || idx%4 != 0 {
		t.Skipf("key ends in %q, which is not a canonical boundary character", last)
	}
	noncanonical := encoded[:len(encoded)-1] + string(alphabet[idx+1])

	ok, err := VerifyPassword(noncanonical, password)
	if ok {
		t.Fatal("non-canonical encoding of the same key verified")
	}
	if !errors.Is(err, ErrMalformedHash) {
		t.Fatalf("error = %v, want ErrMalformedHash", err)
	}
}

// A stored hash is data, not trusted input: a corrupt or hostile row must not
// be able to ask argon2 for an arbitrarily large key.
func TestVerifyPasswordRejectsOversizedKey(t *testing.T) {
	t.Parallel()
	oversized := base64.RawStdEncoding.EncodeToString(make([]byte, maxKeyLength+1))
	encoded := "$argon2id$v=19$m=65536,t=3,p=2$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, saltLength)) + "$" + oversized
	if _, err := VerifyPassword(encoded, "correct horse battery"); !errors.Is(err, ErrMalformedHash) {
		t.Fatalf("oversized key error = %v, want ErrMalformedHash", err)
	}
}
