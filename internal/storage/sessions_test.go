package storage

import (
	"errors"
	"testing"
	"time"
)

func seedUser(t *testing.T, store *Store, id string) {
	t.Helper()
	if err := store.CreateUser(UserRecord{ID: id, Email: id + "@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	rec := SessionRecord{TokenHash: "t1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), LastSeenAt: now}
	if err := store.CreateSession(rec); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != "u1" || !got.ExpiresAt.Equal(rec.ExpiresAt) || !got.CreatedAt.Equal(now) {
		t.Fatalf("unexpected session: %+v", got)
	}
	if _, err := store.GetSession("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session error = %v, want ErrNotFound", err)
	}
}

func TestTouchSessionExtendsExpiry(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	_ = store.CreateSession(SessionRecord{TokenHash: "t1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	later := now.Add(30 * time.Minute)
	if err := store.TouchSession("t1", later, later.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetSession("t1")
	if !got.LastSeenAt.Equal(later) || !got.ExpiresAt.Equal(later.Add(24*time.Hour)) {
		t.Fatalf("touch not applied: %+v", got)
	}
}

func TestDeleteSessionAndDeleteUserSessionsKeepsOne(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Now()
	for _, h := range []string{"a", "b", "c"} {
		_ = store.CreateSession(SessionRecord{TokenHash: h, UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	}
	if err := store.DeleteSession("a"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUserSessions("u1", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("b"); err != nil {
		t.Fatalf("kept session missing: %v", err)
	}
	for _, h := range []string{"a", "c"} {
		if _, err := store.GetSession(h); !errors.Is(err, ErrNotFound) {
			t.Fatalf("session %q should be gone, got %v", h, err)
		}
	}
}

func TestDeletingUserCascadesSessions(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Now()
	_ = store.CreateSession(SessionRecord{TokenHash: "a", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	if err := store.DeleteUser("u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session survived user deletion: %v", err)
	}
}

func TestPruneExpiredSessions(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	_ = store.CreateSession(SessionRecord{TokenHash: "old", UserID: "u1", CreatedAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(-time.Minute), LastSeenAt: now.Add(-time.Hour)})
	_ = store.CreateSession(SessionRecord{TokenHash: "live", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	n, err := store.PruneExpiredSessions(now)
	if err != nil || n != 1 {
		t.Fatalf("PruneExpiredSessions = %d, %v; want 1, nil", n, err)
	}
	if _, err := store.GetSession("live"); err != nil {
		t.Fatalf("live session pruned: %v", err)
	}
}
