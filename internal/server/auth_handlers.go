package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

const (
	// maxAuthBodyBytes caps credential payloads; nothing legitimate is bigger.
	maxAuthBodyBytes = 4 << 10
	// loginFailureMessage is shared by "no such user" and "wrong password" so
	// the response cannot be used to enumerate accounts.
	loginFailureMessage = "invalid email or password"
)

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	setProtectedJSONHeaders(w)
	w.WriteHeader(status)
	writeJSONBody(w, payload)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func userResponse(u auth.User) map[string]any {
	return map[string]any{"user": u}
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage not initialized")
		return
	}
	n, err := s.store.CountUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read users")
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]bool{"needsSetup": n == 0 && !s.config.Insecure})
}

type setupRequest struct {
	Token    string `json:"token"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleSetup creates the first admin. Everything that can fail on user
// input is checked before the one-time token is consumed, so a typo does
// not force a daemon restart to mint a new one.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	n, err := s.store.CountUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read users")
		return
	}
	if n > 0 {
		writeJSONError(w, http.StatusConflict, "setup already completed")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.setupToken == nil || !s.setupToken.Consume(req.Token) {
		writeJSONError(w, http.StatusForbidden, "invalid setup token")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := auth.NewID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	now := time.Now()
	if err := s.store.CreateUser(storage.UserRecord{ID: id, Email: email, PasswordHash: hash, Role: string(auth.RoleAdmin), CreatedAt: now}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	_ = s.store.TouchLastLogin(id, now)
	if err := s.issueSession(w, r, id, now); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSONStatus(w, http.StatusCreated, userResponse(auth.User{ID: id, Email: email, Role: auth.RoleAdmin}))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		auth.VerifyDummy(req.Password)
		writeJSONError(w, http.StatusUnauthorized, loginFailureMessage)
		return
	}
	u, err := s.store.GetUserByEmail(email)
	if errors.Is(err, storage.ErrNotFound) {
		// Burn the same work as a real check so timing does not reveal
		// which emails exist.
		auth.VerifyDummy(req.Password)
		writeJSONError(w, http.StatusUnauthorized, loginFailureMessage)
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, req.Password)
	if err != nil || !ok {
		writeJSONError(w, http.StatusUnauthorized, loginFailureMessage)
		return
	}
	role, err := auth.ParseRole(u.Role)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "user has an invalid role")
		return
	}
	now := time.Now()
	_ = s.store.TouchLastLogin(u.ID, now)
	if err := s.issueSession(w, r, u.ID, now); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSONStatus(w, http.StatusOK, userResponse(auth.User{ID: u.ID, Email: u.Email, Role: role}))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if p, ok := principalFrom(r.Context()); ok && p.viaCookie {
		_ = s.store.DeleteSession(p.tokenHash)
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	writeJSONStatus(w, http.StatusOK, userResponse(p.user))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !p.viaCookie {
		writeJSONError(w, http.StatusBadRequest, "password change requires a signed-in browser session")
		return
	}
	var req changePasswordRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := s.store.GetUserByID(p.user.ID)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, req.CurrentPassword)
	if err != nil || !ok {
		writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdatePasswordHash(u.ID, hash); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	// Sign out every other device; the one that changed the password stays.
	_ = s.store.DeleteUserSessions(u.ID, p.tokenHash)
	w.WriteHeader(http.StatusNoContent)
}
