package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
)

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func firstCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no %s cookie in response", sessionCookieName)
	return nil
}

func TestSetupStatusReflectsUserCount(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	rec := doJSON(t, srv.routes(), http.MethodGet, "/api/auth/setup", nil)
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`"needsSetup":true`)) {
		t.Fatalf("fresh install: %d %s", rec.Code, rec.Body.String())
	}
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	rec = doJSON(t, srv.routes(), http.MethodGet, "/api/auth/setup", nil)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"needsSetup":false`)) {
		t.Fatalf("after first user: %s", rec.Body.String())
	}
}

func TestSetupRequiresTheToken(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{})
	tok, _ := auth.NewSetupToken()
	srv.setupToken = tok
	rec := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": "wrong", "email": "admin@example.com", "password": "correct horse battery"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token status = %d, want 403", rec.Code)
	}
	// A bad password must not burn the token.
	rec = doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": tok.String(), "email": "admin@example.com", "password": "short"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", rec.Code)
	}
	rec = doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": tok.String(), "email": "Admin@Example.com", "password": "correct horse battery"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(t, rec)
	me := doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, cookie)
	if me.Code != 200 || !bytes.Contains(me.Body.Bytes(), []byte(`"email":"admin@example.com"`)) || !bytes.Contains(me.Body.Bytes(), []byte(`"role":"admin"`)) {
		t.Fatalf("me after setup: %d %s", me.Code, me.Body.String())
	}
	rec = doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": tok.String(), "email": "second@example.com", "password": "correct horse battery"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d, want 409", rec.Code)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	wrongPw := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "admin@example.com", "password": "not the password"})
	noUser := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "ghost@example.com", "password": "not the password"})
	if wrongPw.Code != 401 || noUser.Code != 401 {
		t.Fatalf("statuses = %d/%d, want 401/401", wrongPw.Code, noUser.Code)
	}
	if wrongPw.Body.String() != noUser.Body.String() {
		t.Fatalf("bodies differ:\n%s\n%s", wrongPw.Body.String(), noUser.Body.String())
	}
	if len(wrongPw.Result().Cookies()) != 0 {
		t.Fatal("failed login set a cookie")
	}
}

func TestLoginLogoutLifecycle(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	rec := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "ADMIN@example.com", "password": "correct horse battery"})
	if rec.Code != 200 {
		t.Fatalf("login status = %d: %s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(t, rec)
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, cookie).Code != 200 {
		t.Fatal("me with fresh cookie should be 200")
	}
	if u, _ := store.GetUserByID("a1"); u.LastLoginAt == nil {
		t.Fatal("login did not record last_login_at")
	}
	out := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/logout", nil, cookie)
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", out.Code)
	}
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, cookie).Code != 401 {
		t.Fatal("session survived logout")
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	login := func() *http.Cookie {
		return firstCookie(t, doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "admin@example.com", "password": "correct horse battery"}))
	}
	laptop, phone := login(), login()
	bad := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/password", map[string]string{"currentPassword": "wrong wrong wrong", "newPassword": "new horse battery staple"}, laptop)
	if bad.Code != 401 {
		t.Fatalf("wrong current password status = %d, want 401", bad.Code)
	}
	ok := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/password", map[string]string{"currentPassword": "correct horse battery", "newPassword": "new horse battery staple"}, laptop)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("change status = %d, want 204: %s", ok.Code, ok.Body.String())
	}
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, laptop).Code != 200 {
		t.Fatal("the changing session must stay signed in")
	}
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, phone).Code != 401 {
		t.Fatal("other sessions must be revoked")
	}
	if doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "admin@example.com", "password": "new horse battery staple"}).Code != 200 {
		t.Fatal("new password does not log in")
	}
}

func doJSONWithKey(t *testing.T, h http.Handler, method, path, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
