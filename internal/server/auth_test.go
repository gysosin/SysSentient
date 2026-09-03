package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
	"sys-sentient/internal/storage"
)

func newAuthTestServer(t *testing.T, cfg config.ServerConfig) (*Server, *storage.Store) {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	srv := NewServer(cfg, testPrivacy(), store, nil, nil, nil).
		WithAuth(config.AuthConfig{SessionIdleHours: 24, SessionMaxDays: 30, LoginRatePerMinute: 5}, nil)
	t.Cleanup(func() {
		srv.Hub.Close()
		_ = store.Close()
	})
	return srv, store
}

func seedAccount(t *testing.T, store *storage.Store, id, email, password string, role auth.Role) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(storage.UserRecord{ID: id, Email: email, PasswordHash: hash, Role: string(role), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

// sessionCookie mints a session directly, so middleware can be tested before
// the login handler exists.
func sessionCookie(t *testing.T, srv *Server, userID string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := srv.issueSession(rec, req, userID, time.Now()); err != nil {
		t.Fatalf("issueSession: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func TestProtectedRouteRejectsAnonymous(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestProtectedRouteAcceptsMachineKey(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{APIKey: "machine-key"})
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.Header.Set("X-API-Key", "machine-key")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestProtectedRouteAcceptsSessionCookie(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleViewer)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.AddCookie(sessionCookie(t, srv, "u1"))
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleAdmin)

	plain := sessionCookie(t, srv, "u1")
	if plain.Name != sessionCookieName || !plain.HttpOnly || plain.SameSite != http.SameSiteStrictMode || plain.Path != "/" {
		t.Fatalf("cookie attributes wrong: %+v", plain)
	}
	if plain.Secure {
		t.Fatal("Secure must not be set on a plain-HTTP request")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if err := srv.issueSession(rec, req, "u1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !rec.Result().Cookies()[0].Secure {
		t.Fatal("Secure must be set when forwarded over https")
	}
}

func TestViewerCannotCallAdminEndpoints(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "v1", "viewer@example.com", "correct horse battery", auth.RoleViewer)
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.AddCookie(sessionCookie(t, srv, "v1"))
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.AddCookie(sessionCookie(t, srv, "a1"))
	rec = httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Fatalf("admin status = %d, must pass the auth gate", rec.Code)
	}
}

func TestCrossSiteMutationRejected(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.AddCookie(sessionCookie(t, srv, "a1"))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestInsecureModeBypassesAuth(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{Insecure: true})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleAdmin)
	token, _ := auth.NewToken()
	past := time.Now().Add(-time.Minute)
	_ = store.CreateSession(storage.SessionRecord{TokenHash: auth.HashToken(token), UserID: "u1", CreatedAt: past.Add(-time.Hour), ExpiresAt: past, LastSeenAt: past})
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWebSocketRejectsQueryKeyAndAcceptsCookie(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{APIKey: "machine-key"})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleViewer)

	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws/metrics?api_key=machine-key", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("query-string key status = %d, want 401 (query auth removed)", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/metrics", nil)
	req.AddCookie(sessionCookie(t, srv, "u1"))
	rec = httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("cookie-authenticated upgrade was rejected as unauthenticated")
	}
}
