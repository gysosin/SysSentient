package server

import (
	"encoding/json"
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
	logReader      *logs.LogReader
	scrubber       *pii.Scrubber
	authMiddleware *AuthMiddleware
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
	mux.HandleFunc("POST /api/analyze", s.authMiddleware.AuthenticateFunc(s.handleAnalyze))

	// WebSocket endpoint for real-time metrics (protected)
	mux.HandleFunc("GET /ws/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !s.isOriginAllowed(r.Header.Get("Origin")) {
			http.Error(w, "Forbidden: origin not allowed", http.StatusForbidden)
			return
		}

		// Note: WebSocket auth via query param for compatibility
		if !s.validWebSocketAPIKey(r.URL.Query().Get("api_key")) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
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
	return newHTTPServer(addr, handler).ListenAndServe()
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"sys-sentient"}`))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.store.GetRecent(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	insights, err := s.store.GetRecentInsights(10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(insights)
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.aiService == nil {
		http.Error(w, "AI Service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Get latest state
	states, err := s.store.GetRecent(1)
	if err != nil || len(states) == 0 {
		http.Error(w, "No metrics available", http.StatusInternalServerError)
		return
	}
	state := states[0]

	// Trigger analysis
	// Collect real logs
	rawLogs, err := s.logReader.GetLogsWithTimeout(5 * time.Second)
	if err != nil {
		rawLogs = "Failed to collect logs for manual analysis."
	}
	logs := s.scrubber.SanitizeLog(rawLogs)

	insight, err := s.aiService.AnalyzeSystemState(r.Context(), state, logs)
	if err != nil {
		fmt.Printf("Error analyzing system state: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save it
	if err := s.store.SaveInsight(insight); err != nil {
		// Log error but return success to user
		fmt.Printf("Error saving insight: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(insight))
}

func (s *Server) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" && s.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

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
