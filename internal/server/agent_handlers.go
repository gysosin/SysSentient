package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

// defaultJoinTokenTTL bounds how long an unused invitation stays valid.
//
// An hour is long enough to paste a command into another machine's shell and
// short enough that a token left in shell history or a chat message is inert
// by the time anyone finds it.
const defaultJoinTokenTTL = time.Hour

// maxJoinTokenTTL caps what an operator may ask for. A token that never
// expires is a permanent enrolment credential in plain text.
const maxJoinTokenTTL = 24 * time.Hour

type createTokenRequest struct {
	Label      string `json:"label"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type createTokenResponse struct {
	// Token is returned exactly once, at creation. Only its hash is stored, so
	// there is no endpoint that can ever show it again.
	Token     string    `json:"token"`
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expires_at"`
	// Command is the ready-to-paste enrolment line for the target machine.
	Command string `json:"command"`
	// Bootstrap is the one-liner for a machine that has nothing installed yet,
	// keyed by platform family.
	Bootstrap map[string]string `json:"bootstrap"`
}

// handleCreateJoinToken mints a single-use invitation for a new machine.
func (s *Server) handleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}

	var req createTokenRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	label := strings.TrimSpace(req.Label)
	if len(label) > 64 {
		writeJSONError(w, http.StatusBadRequest, "label must be 64 characters or fewer")
		return
	}

	ttl := defaultJoinTokenTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl > maxJoinTokenTTL {
		writeJSONError(w, http.StatusBadRequest, "ttl_seconds exceeds the 24 hour maximum")
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not generate token")
		return
	}
	id, err := auth.NewID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not generate token id")
		return
	}

	p, _ := principalFrom(r.Context())
	now := time.Now()
	expiresAt := now.Add(ttl)

	if err := s.store.CreateJoinToken(id, auth.HashToken(token), label, p.user.Email, now, expiresAt); err != nil {
		slog.Error("create join token", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not create join token")
		return
	}

	slog.Info("join token issued", "id", id, "label", label, "by", p.user.Email)

	writeJSONStatus(w, http.StatusCreated, createTokenResponse{
		Token:     token,
		ID:        id,
		Label:     label,
		ExpiresAt: expiresAt,
		Command:   joinCommand(s.publicURL(r), token),
		Bootstrap: bootstrapCommands(s.publicURL(r), token),
	})
}

// joinCommand renders the line an operator pastes on a machine that already
// has the binary.
//
// --install-service is included because an agent that only runs in somebody's
// shell stops monitoring the moment they close it, which is not what anyone
// enrolling a machine intends.
func joinCommand(serverURL, token string) string {
	return "sys-sentient agent join --server " + serverURL + " --token " + token + " --install-service"
}

// bootstrapCommands render the one-liners for a machine with nothing installed.
//
// The Devices screen used to say "install SysSentient on the machine, then run
// the command below" and offer nothing to install it with — you had to already
// have the binary to be told how to enrol it.
func bootstrapCommands(serverURL, token string) map[string]string {
	return map[string]string{
		"unix": "curl -fsSL " + serverURL + "/install.sh | sh -s -- --server " +
			serverURL + " --token " + token,
		"windows": "iwr -useb " + serverURL + "/install.ps1 | iex; " +
			"Install-SysSentient -Server " + serverURL + " -Token " + token,
	}
}

// publicURL reconstructs the address the agent should call back on.
//
// Falls back to the request's own Host, since a server behind a proxy has no
// other way to know the name clients actually reach it by.
func (s *Server) publicURL(r *http.Request) string {
	if u := strings.TrimSpace(s.config.PublicURL); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

type joinRequest struct {
	Token    string `json:"token"`
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
}

type joinResponse struct {
	AgentID  string `json:"agent_id"`
	AgentKey string `json:"agent_key"`
	Label    string `json:"label"`
}

// handleAgentJoin redeems an invitation for a per-agent credential.
//
// Deliberately unauthenticated: the token *is* the credential, and a machine
// that has not enrolled yet has nothing else to present.
func (s *Server) handleAgentJoin(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}

	var req joinRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Token == "" || req.HostID == "" {
		writeJSONError(w, http.StatusBadRequest, "token and host_id are required")
		return
	}

	agentKey, err := auth.NewToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not generate agent key")
		return
	}
	agentID, err := auth.NewID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not generate agent id")
		return
	}

	agent, err := s.store.RedeemJoinToken(
		auth.HashToken(req.Token), agentID, auth.HashToken(agentKey),
		req.HostID, req.Hostname, req.Version, time.Now())
	if errors.Is(err, storage.ErrTokenNotFound) {
		// One message for expired, spent and never-issued alike, so a caller
		// cannot probe for which tokens exist.
		writeJSONError(w, http.StatusUnauthorized, "join token is invalid or has expired")
		return
	}
	if err != nil {
		slog.Error("redeem join token", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not complete enrolment")
		return
	}

	slog.Info("agent enrolled", "agent_id", agent.ID, "host_id", agent.HostID, "label", agent.Label)

	writeJSONStatus(w, http.StatusCreated, joinResponse{
		AgentID:  agent.ID,
		AgentKey: agentKey,
		Label:    agent.Label,
	})
}

// handleListAgents returns the enrolled fleet.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	agents, err := s.store.ListAgents()
	if err != nil {
		slog.Error("list agents", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not list agents")
		return
	}
	writeJSONBody(w, map[string]any{"agents": agents})
}

// handleListJoinTokens returns invitations that are still redeemable.
func (s *Server) handleListJoinTokens(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	tokens, err := s.store.ListJoinTokens(time.Now())
	if err != nil {
		slog.Error("list join tokens", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not list join tokens")
		return
	}
	writeJSONBody(w, map[string]any{"tokens": tokens})
}

// handleRevokeAgent withdraws a credential.
func (s *Server) handleRevokeAgent(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "agent id is required")
		return
	}

	err := s.store.RevokeAgent(id, time.Now())
	if errors.Is(err, storage.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "no such active agent")
		return
	}
	if err != nil {
		slog.Error("revoke agent", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not revoke agent")
		return
	}

	p, _ := principalFrom(r.Context())
	slog.Warn("agent revoked", "agent_id", id, "by", p.user.Email)
	w.WriteHeader(http.StatusNoContent)
}

// authenticateAgent gates the ingest path.
//
// Per-agent credentials are tried first, then the shared key. Keeping the
// shared key working matters for upgrades: an existing fleet configured with
// one static key must keep reporting across a server upgrade, and can migrate
// machine by machine rather than all at once.
func (s *Server) authenticateAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.Insecure {
			next(w, r)
			return
		}

		key := apiKeyFromRequest(r)
		if key == "" {
			writeJSONError(w, http.StatusUnauthorized, "agent credential required")
			return
		}

		if s.store != nil {
			agent, err := s.store.AgentByKey(auth.HashToken(key))
			switch {
			case err == nil:
				next(w, r.WithContext(withAgent(r.Context(), agent)))
				return
			case errors.Is(err, storage.ErrAgentRevoked):
				// Told plainly, so the agent can stop retrying and log something
				// an operator can act on rather than looping forever on a 401.
				writeJSONError(w, http.StatusForbidden, "this agent credential has been revoked")
				return
			case !errors.Is(err, storage.ErrNotFound):
				slog.Error("agent lookup", "error", err)
				writeJSONError(w, http.StatusInternalServerError, "could not verify agent credential")
				return
			}
		}

		// Fall back to the shared key.
		if s.agentAuth.validAPIKey(key) {
			next(w, r)
			return
		}
		writeJSONError(w, http.StatusUnauthorized, "invalid agent credential")
	}
}
