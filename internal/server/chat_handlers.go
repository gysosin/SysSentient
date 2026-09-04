package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"sys-sentient/internal/ai"
)

// maxChatQuestion bounds one question.
const maxChatQuestion = 4000

// maxChatHistory bounds how much conversation is carried back.
//
// Every turn is re-sent on the next request, so an unbounded history is a
// bill that grows with the length of the conversation.
const maxChatHistory = 20

type chatRequest struct {
	Question string           `json:"question"`
	History  []ai.ChatMessage `json:"history"`
}

// handleChat answers a question about the fleet, using tools.
//
// Admin-only and rate-limited alongside the one-shot analysis: a chat turn can
// make several model calls, so it is the more expensive of the two.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if s.aiService == nil {
		writeJSONError(w, http.StatusServiceUnavailable,
			"AI is not configured; set gemini.api_key to use the assistant")
		return
	}

	var req chatRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Question) == 0 || len(req.Question) > maxChatQuestion {
		writeJSONError(w, http.StatusBadRequest,
			"question must be between 1 and 4000 characters")
		return
	}
	if len(req.History) > maxChatHistory {
		// Trim rather than reject: a long conversation is the user's fault
		// only in the sense that they kept talking.
		req.History = req.History[len(req.History)-maxChatHistory:]
	}

	reply, err := s.aiService.Chat(r.Context(), s.toolbox(), req.History, req.Question)
	if err != nil {
		slog.Error("assistant chat failed", "error", err)
		// The budget message names the cap and is worth passing through; other
		// failures are not.
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	slog.Info("assistant answered", "tools", reply.ToolCalls)
	setProtectedJSONHeaders(w)
	writeJSONBody(w, reply)
}
