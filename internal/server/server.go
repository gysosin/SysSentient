package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sys-sentient/internal/ai"
	"sys-sentient/internal/config"
	"sys-sentient/internal/logs"
	"sys-sentient/internal/pii"
	"sys-sentient/internal/storage"
	"time"
)

type Server struct {
	store          *storage.Store
	config         config.ServerConfig
	aiService      *ai.AIService
	Hub            *Hub
	logReader      logCollector
	scrubber       *pii.Scrubber
	authMiddleware *AuthMiddleware
	httpServer     *http.Server
}

type logCollector interface {
	GetLogsWithTimeout(time.Duration) (string, error)
	GetLogsContextWithTimeout(context.Context, time.Duration) (string, error)
}

func NewServer(cfg config.ServerConfig, store *storage.Store, aiService *ai.AIService) *Server {
	hub := NewHub()
	go hub.Run()

	return &Server{
		store:          store,
		config:         cfg,
		aiService:      aiService,
		Hub:            hub,
		logReader:      logs.NewLogReader(50),
		scrubber:       pii.NewScrubber(true, true, true),
		authMiddleware: NewAuthMiddleware(cfg.APIKey),
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check endpoint (public)
	mux.HandleFunc("GET /health", s.handleHealth)

	// Protected API endpoints
	mux.HandleFunc("GET /api/metrics", s.authMiddleware.AuthenticateFunc(s.handleMetrics))
	mux.HandleFunc("GET /api/insights", s.authMiddleware.AuthenticateFunc(s.handleInsights))
	mux.HandleFunc("GET /api/logs", s.authMiddleware.AuthenticateFunc(s.handleLogs))
	mux.HandleFunc("POST /api/analyze", s.authMiddleware.AuthenticateFunc(s.handleAnalyze))

	// WebSocket endpoint for real-time metrics (protected)
	mux.HandleFunc("GET /ws/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !s.isOriginAllowed(r.Header.Get("Origin")) {
			writeJSONError(w, http.StatusForbidden, "origin not allowed")
			return
		}

		// Note: WebSocket auth via query param for compatibility
		if !s.validWebSocketAPIKey(r.URL.Query().Get("api_key")) {
			writeJSONError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		r = markWebSocketOriginValidated(r)
		ServeWs(s.Hub, w, r)
	})

	// Serve Static Files (public)
	fs := http.FileServer(http.Dir("./web/dist"))
	mux.Handle("/", fs)

	handler := s.enableCORS(mux)

	addr := fmt.Sprintf(":%d", s.config.Port)
	fmt.Printf("Starting API Server on %s\n", addr)
	if s.authMiddleware.enabled {
		fmt.Println("⚠️  Authentication enabled. API key required for protected endpoints.")
	} else {
		fmt.Println("⚠️  WARNING: No API key configured. Server is running without authentication!")
	}
	fmt.Printf("WebSocket endpoint: ws://localhost%s/ws/metrics\n", addr)
	s.httpServer = newHTTPServer(addr, handler)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status":   "healthy",
		"service":  "sys-sentient",
		"database": "not_configured",
	}
	statusCode := http.StatusOK

	if s.store != nil {
		if err := s.store.Ping(); err != nil {
			response["status"] = "unhealthy"
			response["database"] = "unavailable"
			statusCode = http.StatusServiceUnavailable
		} else {
			response["database"] = "ok"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.store.GetRecent(50)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load metrics")
		return
	}
	setProtectedJSONHeaders(w)
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	insights, err := s.store.GetRecentInsights(10)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load insights")
		return
	}
	setProtectedJSONHeaders(w)
	json.NewEncoder(w).Encode(insights)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if s.logReader == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "log reader not initialized")
		return
	}

	rawLogs, err := s.logReader.GetLogsContextWithTimeout(r.Context(), 3*time.Second)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to collect logs")
		return
	}

	setProtectedJSONHeaders(w)
	json.NewEncoder(w).Encode(map[string]string{
		"collectedAt": time.Now().UTC().Format(time.RFC3339),
		"content":     s.scrubber.SanitizeLog(rawLogs),
	})
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.aiService == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AI service not initialized")
		return
	}

	// Get latest state
	states, err := s.store.GetRecent(1)
	if err != nil || len(states) == 0 {
		writeJSONError(w, http.StatusInternalServerError, "no metrics available")
		return
	}
	state := states[0]

	// Trigger analysis
	// Collect real logs
	rawLogs, err := s.logReader.GetLogsContextWithTimeout(r.Context(), 5*time.Second)
	if err != nil {
		rawLogs = "Failed to collect logs for manual analysis."
	}
	logs := s.scrubber.SanitizeLog(rawLogs)

	insight, err := s.aiService.AnalyzeSystemState(r.Context(), state, logs)
	if err != nil {
		fmt.Printf("Error analyzing system state: %v\n", err)
		writeJSONError(w, http.StatusInternalServerError, "AI analysis failed")
		return
	}

	// Save it
	if err := s.store.SaveInsight(insight); err != nil {
		// Log error but return success to user
		fmt.Printf("Error saving insight: %v\n", err)
	}

	setProtectedJSONHeaders(w)
	if err := writeInsightResponse(w, insight); err != nil {
		fmt.Printf("Error writing insight response: %v\n", err)
	}
}

func (s *Server) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" {
			w.Header().Add("Vary", "Origin")
			if !s.isOriginAllowed(origin) {
				if r.Method == http.MethodOptions {
					writeJSONError(w, http.StatusForbidden, "origin not allowed")
					return
				}
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) isOriginAllowed(origin string) bool {
	if origin == "" {
		return true
	}

	for _, allowedOrigin := range s.config.AllowedOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			return true
		}
	}
	return false
}

func (s *Server) validWebSocketAPIKey(providedKey string) bool {
	if !s.authMiddleware.enabled {
		return true
	}
	return s.authMiddleware.validAPIKey(providedKey)
}

func writeInsightResponse(w http.ResponseWriter, insight string) error {
	var payload map[string]any
	if err := json.Unmarshal([]byte(insight), &payload); err == nil && payload != nil {
		return json.NewEncoder(w).Encode(payload)
	}

	return json.NewEncoder(w).Encode(map[string]any{
		"status":             "Warning",
		"summary":            "AI Analysis Generated",
		"detailedAnalysis":   insight,
		"recommendedActions": []map[string]any{},
	})
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	setProtectedJSONHeaders(w)
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func setProtectedJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}
