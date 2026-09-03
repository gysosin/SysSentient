package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

const sessionCookieName = "sys_session"

// sessionTouchInterval bounds how often a sliding expiry is written back, so
// the dashboard's two-second polling does not turn into a write per request.
const sessionTouchInterval = 5 * time.Minute

// principal is who a request acts as and how they proved it. Handlers that
// need to revoke "this" session read tokenHash; handlers that gate on browser
// origin read viaCookie.
type principal struct {
	user      auth.User
	tokenHash string
	viaCookie bool
}

type principalKey struct{}

func withPrincipal(ctx context.Context, p principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func principalFrom(ctx context.Context) (principal, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p, ok
}

var (
	// apiKeyPrincipal is the synthetic identity behind the machine token.
	apiKeyPrincipal = principal{user: auth.User{ID: "api-key", Email: "api-key", Role: auth.RoleAdmin}}
	// insecurePrincipal is used when server.insecure disables auth.
	insecurePrincipal = principal{user: auth.User{ID: "insecure", Email: "insecure", Role: auth.RoleAdmin}}
)

// requestIsSecure decides the cookie's Secure flag per request. Setting it
// unconditionally would silently break every plain-HTTP LAN install, because
// browsers drop Secure cookies on http://.
func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string, now time.Time) error {
	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	rec := storage.SessionRecord{
		TokenHash:  auth.HashToken(token),
		UserID:     userID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.sessionIdle),
		LastSeenAt: now,
	}
	if err := s.store.CreateSession(rec); err != nil {
		return err
	}
	// #nosec G124 -- HttpOnly and SameSite=Strict are set unconditionally;
	// Secure is decided per request by requestIsSecure() because setting it
	// on a plain-HTTP LAN install would make browsers drop the cookie and
	// nobody could sign in. The daemon warns at startup when TLS is absent.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
		// The browser keeps the cookie for the absolute cap; the server
		// enforces the shorter idle window on every request.
		MaxAge: int(s.sessionMax.Seconds()),
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	// #nosec G124 -- same rationale as issueSession; this expires the cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// resolveSession turns a cookie into a principal, enforcing both the idle
// and the absolute expiry, and sliding the idle window on activity.
func (s *Server) resolveSession(r *http.Request, now time.Time) (principal, bool) {
	if s.store == nil {
		return principal{}, false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return principal{}, false
	}
	hash := auth.HashToken(c.Value)
	rec, err := s.store.GetSession(hash)
	if err != nil {
		return principal{}, false
	}
	if now.After(rec.ExpiresAt) || now.After(rec.CreatedAt.Add(s.sessionMax)) {
		_ = s.store.DeleteSession(hash)
		return principal{}, false
	}
	u, err := s.store.GetUserByID(rec.UserID)
	if err != nil {
		return principal{}, false
	}
	role, err := auth.ParseRole(u.Role)
	if err != nil {
		return principal{}, false
	}
	if now.Sub(rec.LastSeenAt) > sessionTouchInterval {
		_ = s.store.TouchSession(hash, now, now.Add(s.sessionIdle))
	}
	return principal{
		user:      auth.User{ID: u.ID, Email: u.Email, Role: role},
		tokenHash: hash,
		viaCookie: true,
	}, true
}
