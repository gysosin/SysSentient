package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sys-sentient/internal/ai"
	"sys-sentient/internal/config"
	"sys-sentient/internal/storage"
)

type Server struct {
	store     *storage.Store
	config    config.ServerConfig
	aiService *ai.AIService
}

func NewServer(cfg config.ServerConfig, store *storage.Store, aiService *ai.AIService) *Server {
	return &Server{
		store:     store,
		config:    cfg,
		aiService: aiService,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Using Go 1.22+ routing patterns
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/insights", s.handleInsights)
	mux.HandleFunc("POST /api/analyze", s.handleAnalyze)

	// Serve Static Files
	// Check if directory exists or just serve.
	// For Single Page App, might need to serve index.html on 404, but strict file server is okay for now.
	fs := http.FileServer(http.Dir("./web/dist"))
	mux.Handle("/", fs)

	handler := s.enableCORS(mux)

	addr := fmt.Sprintf(":%d", s.config.Port)
	fmt.Printf("Starting API Server on %s\n", addr)
	return http.ListenAndServe(addr, handler)
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
	// In real app we might want to read logs again.
	logs := "Manual analysis request. No logs provided."
	
	insight, err := s.aiService.AnalyzeSystemState(r.Context(), state, logs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save it
	if err := s.store.SaveInsight(insight); err != nil {
		// Log error but return success to user
		fmt.Printf("Error saving insight: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"insight": insight})
}

func (s *Server) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
