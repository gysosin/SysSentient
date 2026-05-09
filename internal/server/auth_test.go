package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareAllowsRequestsWhenDisabled(t *testing.T) {
	auth := NewAuthMiddleware("")
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	rec := httptest.NewRecorder()

	auth.AuthenticateFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected request to pass when auth is disabled, got %d", rec.Code)
	}
}

func TestAuthMiddlewareRequiresAPIKeyForAPIRequests(t *testing.T) {
	auth := NewAuthMiddleware("expected-key")
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	rec := httptest.NewRecorder()

	auth.AuthenticateFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized response, got %d", rec.Code)
	}
}

func TestAuthMiddlewareAllowsHeaderAPIKey(t *testing.T) {
	auth := NewAuthMiddleware("expected-key")
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.Header.Set("X-API-Key", "expected-key")
	rec := httptest.NewRecorder()

	auth.AuthenticateFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected request to pass with matching key, got %d", rec.Code)
	}
}

func TestAuthMiddlewareAllowsBearerAPIKey(t *testing.T) {
	auth := NewAuthMiddleware("expected-key")
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer expected-key")
	rec := httptest.NewRecorder()

	auth.AuthenticateFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected request to pass with matching bearer key, got %d", rec.Code)
	}
}

func TestAuthMiddlewareKeepsHealthPublic(t *testing.T) {
	auth := NewAuthMiddleware("expected-key")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	auth.AuthenticateFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected health check to stay public, got %d", rec.Code)
	}
}
