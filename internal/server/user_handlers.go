package server

import (
	"errors"
	"net/http"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

// managedUser is the admin-facing view of an account. No hash, ever.
type managedUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

func toManagedUser(u storage.UserRecord) managedUser {
	return managedUser{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt}
}

func (s *Server) handleListUsers(w http.ResponseWriter, _ *http.Request) {
	records, err := s.store.ListUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	users := make([]managedUser, 0, len(records))
	for _, rec := range records {
		users = append(users, toManagedUser(rec))
	}
	writeJSONStatus(w, http.StatusOK, users)
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	role, err := auth.ParseRole(req.Role)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "role must be admin or viewer")
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
	rec := storage.UserRecord{ID: id, Email: email, PasswordHash: hash, Role: string(role), CreatedAt: time.Now()}
	if err := s.store.CreateUser(rec); err != nil {
		if errors.Is(err, storage.ErrDuplicateEmail) {
			writeJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSONStatus(w, http.StatusCreated, toManagedUser(rec))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, _ := principalFrom(r.Context())
	if id == p.user.ID {
		writeJSONError(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	target, err := s.store.GetUserByID(id)
	if errors.Is(err, storage.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	// Refuse to leave the console with no administrator. The self-delete guard
	// above catches the common case; this catches deletion by another admin
	// principal, including the machine API key, which has no user row.
	if target.Role == string(auth.RoleAdmin) {
		n, err := s.store.CountAdmins()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to count admins")
			return
		}
		if n <= 1 {
			writeJSONError(w, http.StatusConflict, "cannot delete the last admin")
			return
		}
	}
	if err := s.store.DeleteUser(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
