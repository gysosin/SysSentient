package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"sys-sentient/internal/auth"
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
		if r.URL.Path == "/health" || !isProtectedRouteNamespace(r.URL.Path) {
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

		if !a.validAPIKey(providedKey) {
			writeJSONError(w, http.StatusUnauthorized, "invalid or missing API key")
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

func (a *AuthMiddleware) validAPIKey(providedKey string) bool {
	if providedKey == "" || a.apiKey == "" {
		return false
	}

	expectedHash := sha256.Sum256([]byte(a.apiKey))
	providedHash := sha256.Sum256([]byte(providedKey))
	return subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) == 1
}

func isProtectedRouteNamespace(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") || path == "/ws" || strings.HasPrefix(path, "/ws/")
}

func apiKeyFromRequest(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// authenticate resolves the caller: insecure mode, then the machine key,
// then a session cookie. Query-string credentials are deliberately not read.
func (s *Server) authenticate(r *http.Request) (principal, bool) {
	if s.config.Insecure {
		return insecurePrincipal, true
	}
	if key := apiKeyFromRequest(r); key != "" && s.authMiddleware.validAPIKey(key) {
		return apiKeyPrincipal, true
	}
	return s.resolveSession(r, time.Now())
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// requireAuth gates a handler on any authenticated principal. Cookie
// sessions additionally refuse cross-site mutations: SameSite=Strict already
// stops browsers sending the cookie, this is the belt to that brace.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.authenticate(r)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if p.viaCookie && isMutating(r.Method) && strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			writeJSONError(w, http.StatusForbidden, "cross-site request rejected")
			return
		}
		next(w, r.WithContext(withPrincipal(r.Context(), p)))
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if p, _ := principalFrom(r.Context()); p.user.Role != auth.RoleAdmin {
			writeJSONError(w, http.StatusForbidden, "admin role required")
			return
		}
		next(w, r)
	})
}
