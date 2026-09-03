package auth

import (
	"errors"
	"strings"
)

// Role is what a user may do. Two levels for now; the check is centralised so
// adding more later is a one-place change.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

var (
	ErrInvalidRole  = errors.New("invalid role")
	ErrInvalidEmail = errors.New("invalid email address")
)

// ParseRole validates a role supplied by a client.
func ParseRole(s string) (Role, error) {
	switch Role(strings.TrimSpace(s)) {
	case RoleAdmin:
		return RoleAdmin, nil
	case RoleViewer:
		return RoleViewer, nil
	default:
		return "", ErrInvalidRole
	}
}

// User is the identity attached to an authenticated request. It carries no
// secrets so it can be serialised to the dashboard as-is.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  Role   `json:"role"`
}

// NormalizeEmail lower-cases and trims an address and rejects obviously
// malformed input. Deliverability is not checked; uniqueness is the
// database's job.
func NormalizeEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, " \t\r\n") {
		return "", ErrInvalidEmail
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return "", ErrInvalidEmail
	}
	return s, nil
}
