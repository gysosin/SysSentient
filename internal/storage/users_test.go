package storage

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newUserStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestCreateUserAndCount(t *testing.T) {
	store := newUserStore(t)
	if n, _ := store.CountUsers(); n != 0 {
		t.Fatalf("fresh store has %d users", n)
	}
	err := store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if n, _ := store.CountUsers(); n != 1 {
		t.Fatalf("CountUsers = %d, want 1", n)
	}
	if n, _ := store.CountAdmins(); n != 1 {
		t.Fatalf("CountAdmins = %d, want 1", n)
	}
}

func TestCreateUserRejectsDuplicateEmailCaseInsensitively(t *testing.T) {
	store := newUserStore(t)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()})
	err := store.CreateUser(UserRecord{ID: "u2", Email: "OPS@example.com", PasswordHash: "h", Role: "viewer", CreatedAt: time.Now()})
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("second insert error = %v, want ErrDuplicateEmail", err)
	}
}

func TestGetUserByEmailIsCaseInsensitiveAndRoundTrips(t *testing.T) {
	store := newUserStore(t)
	created := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "hash-1", Role: "viewer", CreatedAt: created})
	u, err := store.GetUserByEmail("Ops@Example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if u.ID != "u1" || u.PasswordHash != "hash-1" || u.Role != "viewer" || !u.CreatedAt.Equal(created) || u.LastLoginAt != nil {
		t.Fatalf("unexpected record: %+v", u)
	}
}

func TestGetUserMissingReturnsErrNotFound(t *testing.T) {
	store := newUserStore(t)
	if _, err := store.GetUserByEmail("nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByEmail error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetUserByID("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByID error = %v, want ErrNotFound", err)
	}
}

func TestUpdatePasswordHashAndTouchLastLogin(t *testing.T) {
	store := newUserStore(t)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "old", Role: "admin", CreatedAt: time.Now()})
	if err := store.UpdatePasswordHash("u1", "new"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 2, 12, 30, 0, 0, time.UTC)
	if err := store.TouchLastLogin("u1", at); err != nil {
		t.Fatal(err)
	}
	u, _ := store.GetUserByID("u1")
	if u.PasswordHash != "new" {
		t.Fatalf("hash = %q, want new", u.PasswordHash)
	}
	if u.LastLoginAt == nil || !u.LastLoginAt.Equal(at) {
		t.Fatalf("LastLoginAt = %v, want %v", u.LastLoginAt, at)
	}
}

func TestListAndDeleteUsers(t *testing.T) {
	store := newUserStore(t)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "a@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()})
	_ = store.CreateUser(UserRecord{ID: "u2", Email: "b@example.com", PasswordHash: "h", Role: "viewer", CreatedAt: time.Now()})
	users, err := store.ListUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("ListUsers = %d users, %v", len(users), err)
	}
	if err := store.DeleteUser("u2"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser("u2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
	if n, _ := store.CountUsers(); n != 1 {
		t.Fatalf("CountUsers after delete = %d, want 1", n)
	}
}
