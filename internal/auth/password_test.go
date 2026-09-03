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
	encoded, _ := HashPassword("correct horse battery staple")
	last := encoded[len(encoded)-1]
	swap := byte('A')
	if last == 'A' {
		swap = 'B'
	}
	tampered := encoded[:len(encoded)-1] + string(swap)
	ok, _ := VerifyPassword(tampered, "correct horse battery staple")
	if ok {
		t.Fatal("tampered hash verified")
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
