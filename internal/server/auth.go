package server

import (
	"net/http"
	"strings"
)

// AuthMiddleware provides API key authentication
type AuthMiddleware struct {
	apiKey  string
	enabled bool
}

// NewAuthMiddleware creates authentication middleware
func NewAuthMiddleware(apiKey string) *AuthMiddleware {
	return &AuthMiddleware{
		apiKey:  apiKey,
		enabled: apiKey != "",
	}
}

// Authenticate wraps a handler with API key checking
func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if not enabled
		if !a.enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Allow public endpoints (health check, static files)
		if r.URL.Path == "/health" || !strings.HasPrefix(r.URL.Path, "/api") && !strings.HasPrefix(r.URL.Path, "/ws") {
			next.ServeHTTP(w, r)
			return
		}

		// Check API key in header
		providedKey := r.Header.Get("X-API-Key")
		if providedKey == "" {
			// Also check Authorization header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				providedKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if providedKey != a.apiKey {
			http.Error(w, "Unauthorized: Invalid or missing API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// AuthenticateFunc wraps a HandlerFunc with authentication
func (a *AuthMiddleware) AuthenticateFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Authenticate(next).ServeHTTP(w, r)
	}
}
