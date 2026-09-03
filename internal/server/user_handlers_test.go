package server

import (
	"bytes"
	"net/http"
	"testing"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
)

func TestUserManagementIsAdminOnly(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "v1", "viewer@example.com", "correct horse battery", auth.RoleViewer)
	rec := doJSON(t, srv.routes(), http.MethodGet, "/api/users", nil, sessionCookie(t, srv, "v1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer list status = %d, want 403", rec.Code)
	}
}

func TestAdminListsCreatesAndDeletesUsers(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	admin := sessionCookie(t, srv, "a1")

	created := doJSON(t, srv.routes(), http.MethodPost, "/api/users",
		map[string]string{"email": "new@example.com", "password": "correct horse battery", "role": "viewer"}, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	dup := doJSON(t, srv.routes(), http.MethodPost, "/api/users",
		map[string]string{"email": "NEW@example.com", "password": "correct horse battery", "role": "viewer"}, admin)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", dup.Code)
	}
	badRole := doJSON(t, srv.routes(), http.MethodPost, "/api/users",
		map[string]string{"email": "x@example.com", "password": "correct horse battery", "role": "root"}, admin)
	if badRole.Code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", badRole.Code)
	}

	list := doJSON(t, srv.routes(), http.MethodGet, "/api/users", nil, admin)
	if list.Code != 200 || !bytes.Contains(list.Body.Bytes(), []byte(`"email":"new@example.com"`)) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	if doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "new@example.com", "password": "correct horse battery"}).Code != 200 {
		t.Fatal("created user cannot log in")
	}

	newUser, _ := store.GetUserByEmail("new@example.com")
	del := doJSON(t, srv.routes(), http.MethodDelete, "/api/users/"+newUser.ID, nil, admin)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", del.Code)
	}
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/"+newUser.ID, nil, admin).Code != http.StatusNotFound {
		t.Fatal("deleting a missing user should be 404")
	}
}

func TestCannotDeleteSelfOrLastAdmin(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	seedAccount(t, store, "a2", "second@example.com", "correct horse battery", auth.RoleAdmin)
	a1 := sessionCookie(t, srv, "a1")
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a1", nil, a1).Code != http.StatusBadRequest {
		t.Fatal("deleting yourself should be 400")
	}
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a2", nil, a1).Code != http.StatusNoContent {
		t.Fatal("deleting another admin while two exist should succeed")
	}
	// a1 is now the only admin. A second admin can delete a1, but nobody can
	// remove the final admin: a3 deleting itself hits the self guard, and a
	// viewer cannot reach the endpoint at all.
	seedAccount(t, store, "a3", "third@example.com", "correct horse battery", auth.RoleAdmin)
	a3 := sessionCookie(t, srv, "a3")
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a1", nil, a3).Code != http.StatusNoContent {
		t.Fatal("deleting a1 while a3 exists should succeed")
	}
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a3", nil, a3).Code != http.StatusBadRequest {
		t.Fatal("a3 deleting itself is 400 (self guard fires before last-admin guard)")
	}
	// Last-admin guard proper: a4 (admin) tries to delete a3 while a3 is the
	// only *other* admin — allowed (two admins). Then a4 alone: a viewer-turned
	// scenario is impossible, so prove the guard via the API key principal,
	// which is an admin without a user row.
	srv2, store2 := newAuthTestServer(t, config.ServerConfig{APIKey: "machine-key"})
	seedAccount(t, store2, "only", "only@example.com", "correct horse battery", auth.RoleAdmin)
	req := doJSONWithKey(t, srv2.routes(), http.MethodDelete, "/api/users/only", "machine-key")
	if req.Code != http.StatusConflict {
		t.Fatalf("deleting the last admin status = %d, want 409", req.Code)
	}
}
