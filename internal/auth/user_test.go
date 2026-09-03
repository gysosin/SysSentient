package auth

import (
	"errors"
	"testing"
)

func TestParseRole(t *testing.T) {
	t.Parallel()
	if r, err := ParseRole("admin"); err != nil || r != RoleAdmin {
		t.Fatalf("ParseRole(admin) = %v, %v", r, err)
	}
	if r, err := ParseRole("viewer"); err != nil || r != RoleViewer {
		t.Fatalf("ParseRole(viewer) = %v, %v", r, err)
	}
	if _, err := ParseRole("root"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("ParseRole(root) error = %v, want ErrInvalidRole", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
		err  error
	}{
		{"  Ops@Example.COM ", "ops@example.com", nil},
		{"ops@example.com", "ops@example.com", nil},
		{"", "", ErrInvalidEmail},
		{"no-at-sign", "", ErrInvalidEmail},
		{"@example.com", "", ErrInvalidEmail},
		{"ops@", "", ErrInvalidEmail},
		{"two@@example.com", "", ErrInvalidEmail},
		{"sp ace@example.com", "", ErrInvalidEmail},
	}
	for _, tc := range cases {
		got, err := NormalizeEmail(tc.in)
		if !errors.Is(err, tc.err) || got != tc.want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}
