package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sys-sentient/internal/ai"
	"sys-sentient/internal/alerting"
	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
	"sys-sentient/internal/logs"
	"sys-sentient/internal/pii"
	"sys-sentient/internal/storage"
	"sys-sentient/internal/version"
	"sys-sentient/web"
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
	evaluator      *alerting.Evaluator
	dispatcher     *alerting.Dispatcher
	agentAuth      *AuthMiddleware
	// analyzeLimiter guards a paid third-party call; logsLimiter guards an
	// endpoint that shells out to journalctl/dmesg on every request.
	analyzeLimiter *rateLimiter
	logsLimiter    *rateLimiter
	// loginLimiter bounds password attempts per client IP; each attempt costs
	// an argon2 verification, so this also caps memory pressure.
	loginLimiter *rateLimiter
	sessionIdle  time.Duration
	sessionMax   time.Duration
	// setupToken is non-nil only until the first admin exists.
	setupToken *auth.SetupToken
}

type logCollector interface {
	GetLogsWithTimeout(time.Duration) (string, error)
	GetLogsContextWithTimeout(context.Context, time.Duration) (string, error)
}

func NewServer(cfg config.ServerConfig, privacy config.PrivacyConfig, store *storage.Store, aiService *ai.AIService, evaluator *alerting.Evaluator, dispatcher *alerting.Dispatcher) *Server {
	hub := NewHub()
	go hub.Run()

	return &Server{
		store:     store,
		config:    cfg,
		aiService: aiService,
		Hub:       hub,
		logReader: logs.NewLogReader(50),
		// Honour the operator's privacy settings. This used to hardcode all-on,
		// so the HTTP path silently disagreed with the daemon's own scrubber.
		scrubber:       pii.NewScrubber(privacy.MaskIPs, privacy.MaskEmails, privacy.MaskUsernames),
		authMiddleware: NewAuthMiddleware(cfg.APIKey),
		evaluator:      evaluator,
		dispatcher:     dispatcher,
		// Falls back to the dashboard key only when no dedicated agent key is
		// set, so a single-node install still works out of the box.
		agentAuth: NewAuthMiddleware(firstNonEmpty(cfg.AgentKey, cfg.APIKey)),
		// 5 analyses then one per minute: enough for interactive use, far
		// below anything that would run up a Gemini bill.
		analyzeLimiter: newRateLimiter(5, time.Minute),
		logsLimiter:    newRateLimiter(30, 2*time.Second),
		loginLimiter:   newRateLimiter(5, 12*time.Second),
		sessionIdle:    24 * time.Hour,
		sessionMax:     30 * 24 * time.Hour,
	}
}

// WithAuth applies the operator's session settings and the first-run setup
// token. Tests that never log in can skip it and take the defaults.
func (s *Server) WithAuth(cfg config.AuthConfig, setup *auth.SetupToken) *Server {
	s.sessionIdle = time.Duration(cfg.SessionIdleHours) * time.Hour
	s.sessionMax = time.Duration(cfg.SessionMaxDays) * 24 * time.Hour
	s.loginLimiter = newRateLimiter(cfg.LoginRatePerMinute, time.Minute/time.Duration(cfg.LoginRatePerMinute))
	s.setupToken = setup
	return s
}

func (s *Server) Start() error {
	handler := s.routes()

	addr := fmt.Sprintf(":%d", s.config.Port)
	slog.Info("API server starting", "addr", addr)
	if s.config.Insecure {
		slog.Warn("server.insecure is set: authentication is DISABLED; anyone who can reach this port has full admin access")
	} else {
		slog.Info("authentication enabled: session login or X-API-Key required for /api and /ws")
		slog.Warn("serving plain HTTP: session cookies will not carry the Secure flag; terminate TLS in front of this daemon for production")
	}
	slog.Info("websocket endpoint ready", "path", "/ws/metrics")
	s.httpServer = newHTTPServer(addr, handler)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// routes builds the full handler chain. Kept separate from Start so tests
// can drive the real mux, middleware included, without opening a socket.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Public. /metrics is unauthenticated like node_exporter: scrapers rarely
	// carry credentials.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handlePrometheusMetrics)
	mux.HandleFunc("GET /api/auth/setup", s.handleSetupStatus)
	mux.HandleFunc("POST /api/auth/setup", rateLimit(s.loginLimiter, "12", s.handleSetup))
	mux.HandleFunc("POST /api/auth/login", rateLimit(s.loginLimiter, "12", s.handleLogin))

	// Any authenticated principal.
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("POST /api/auth/password", s.requireAuth(s.handleChangePassword))
	mux.HandleFunc("GET /api/metrics", s.requireAuth(s.handleMetrics))
	mux.HandleFunc("GET /api/insights", s.requireAuth(s.handleInsights))
	mux.HandleFunc("GET /api/logs", s.requireAuth(rateLimit(s.logsLimiter, "2", s.handleLogs)))
	mux.HandleFunc("GET /api/hosts", s.requireAuth(s.handleHosts))
	// Export can stream tens of thousands of rows out of the same database the
	// collector is writing to, so it is rate limited like the other expensive
	// reads rather than left open.
	mux.HandleFunc("GET /api/export", s.requireAuth(rateLimit(s.logsLimiter, "2", s.handleExport)))
	mux.HandleFunc("GET /api/alerts", s.requireAuth(s.handleAlerts))
	mux.HandleFunc("GET /api/alerts/rules", s.requireAuth(s.handleAlertRules))
	mux.HandleFunc("GET /api/alerts/history", s.requireAuth(s.handleAlertHistory))

	// Admin only: anything that spends money, changes state, or manages people.
	mux.HandleFunc("POST /api/analyze", s.requireAdmin(rateLimit(s.analyzeLimiter, "60", s.handleAnalyze)))
	mux.HandleFunc("POST /api/alerts/{ruleID}/acknowledge", s.requireAdmin(s.handleAcknowledgeAlert))
	mux.HandleFunc("GET /api/users", s.requireAdmin(s.handleListUsers))
	mux.HandleFunc("POST /api/users", s.requireAdmin(s.handleCreateUser))
	mux.HandleFunc("DELETE /api/users/{id}", s.requireAdmin(s.handleDeleteUser))

	// Agents authenticate with their own key.
	mux.HandleFunc("POST /api/ingest", s.agentAuth.AuthenticateFunc(s.handleIngest))

	// WebSocket: same principals as the REST API. The browser sends the
	// cookie on a same-origin upgrade; scripts send X-API-Key.
	mux.HandleFunc("GET /ws/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !s.isOriginAllowed(r.Header.Get("Origin")) {
			writeJSONError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		if _, ok := s.authenticate(r); !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		r = markWebSocketOriginValidated(r)
		ServeWs(s.Hub, w, r)
	})

	mux.Handle("/", staticHandler(dashboardFS()))
	return s.securityHeaders(s.enableCORS(mux))
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

const (
	// defaultMetricsLimit matches what the dashboard charts render.
	defaultMetricsLimit = 50
	// maxMetricsLimit bounds a single query so one request cannot pull an
	// entire retention window into memory.
	maxMetricsLimit = 5000
)

// staleSampleThreshold is how long without a new sample before the daemon
// reports itself degraded rather than healthy.
const staleSampleThreshold = 60 * time.Second

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	build := version.Get()
	response := map[string]any{
		"status":   "healthy",
		"service":  "sys-sentient",
		"database": "not_configured",
		// Without this there was no way to tell what was deployed on a fleet.
		"version": build.Version,
		"commit":  build.Commit,
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

	if s.Hub != nil {
		response["websocket_clients"] = s.Hub.ClientCount()
	}

	// Collector liveness: a wedged collector previously still reported
	// "healthy" because only the database was checked.
	if s.store != nil {
		if recent, err := s.store.GetRecent(1); err == nil && len(recent) > 0 {
			age := time.Since(recent[0].Timestamp)
			response["last_sample_age_seconds"] = int(age.Seconds())
			if age > staleSampleThreshold {
				response["status"] = "degraded"
				response["collector"] = "stale"
				statusCode = http.StatusServiceUnavailable
			} else {
				response["collector"] = "ok"
			}
		} else {
			response["collector"] = "no_samples"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	writeJSONBody(w, response)
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Stop the hub so its goroutine exits and connected dashboards are
	// disconnected rather than left hanging on a dead server.
	if s.Hub != nil {
		s.Hub.Close()
	}
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// ?host= scopes to one machine; omitted returns every host, which keeps
	// single-node deployments working unchanged.
	hostID := r.URL.Query().Get("host")

	limit := defaultMetricsLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= maxMetricsLimit {
			limit = parsed
		}
	}

	metrics, err := s.store.GetRecentForHost(hostID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load metrics")
		return
	}
	setProtectedJSONHeaders(w)
	writeJSONBody(w, metrics)
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	insights, err := s.store.GetRecentInsights(10)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load insights")
		return
	}
	setProtectedJSONHeaders(w)
	writeJSONBody(w, insights)
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
	writeJSONBody(w, map[string]string{
		"collectedAt": time.Now().UTC().Format(time.RFC3339),
		"content":     s.scrubber.SanitizeLog(rawLogs),
	})
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if rejectUnexpectedRequestBody(w, r) {
		return
	}

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

	// Process names and TopProcesses carry paths, hostnames and argv-derived
	// secrets; they are interpolated straight into the prompt.
	insight, err := s.aiService.AnalyzeSystemState(r.Context(), s.scrubber.SanitizeState(state), logs)
	if err != nil {
		slog.Error("error analyzing system state", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "AI analysis failed")
		return
	}

	// Save it
	if err := s.store.SaveInsight(insight); err != nil {
		// Log error but return success to user
		slog.Error("error saving insight", "error", err)
	}

	setProtectedJSONHeaders(w)
	if err := writeInsightResponse(w, insight); err != nil {
		slog.Error("error writing insight response", "error", err)
	}
}

func rejectUnexpectedRequestBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, 0)
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "request body not supported")
		return true
	}
	return false
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

// dashboardFS picks the embedded dashboard, falling back to disk.
//
// The embedded copy is what makes the binary relocatable and therefore
// packageable. The disk fallback exists so `npm run build` during development
// is picked up without recompiling the daemon, and so a binary built before
// the dashboard was ever built still serves something rather than a blank
// page. Which one is in use is logged, because "the UI is stale" is otherwise
// an unpleasant thing to debug.
func dashboardFS() fs.FS {
	const devDir = "./web/dist"

	if _, err := os.Stat(filepath.Join(devDir, "index.html")); err == nil {
		slog.Info("serving dashboard from disk", "path", devDir)
		return os.DirFS(devDir)
	}

	if embedded, ok := web.Dist(); ok {
		return embedded
	}

	slog.Warn("no dashboard available; the API works but / will 404",
		"hint", "run `make web` and rebuild, or start the daemon from a directory containing web/dist")
	return os.DirFS(devDir)
}

// serveIndex writes the SPA entry point. http.ServeFile needs a real path, so
// with an fs.FS the file is opened and copied through http.ServeContent, which
// keeps conditional requests and range handling working.
func serveIndex(w http.ResponseWriter, r *http.Request, root fs.FS) {
	f, err := root.Open("index.html")
	if err != nil {
		http.Error(w, "dashboard not built", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	readSeeker, ok := f.(io.ReadSeeker)
	if !ok {
		// embed.FS files implement io.ReadSeeker; a fallback keeps any other
		// FS implementation working rather than failing the request.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, f)
		return
	}

	var modTime time.Time
	if info, err := f.Stat(); err == nil {
		modTime = info.ModTime()
	}
	http.ServeContent(w, r, "index.html", modTime, readSeeker)
}

// staticHandler serves the built dashboard with single-page-app semantics.
//
// Two behaviours a bare http.FileServer gets wrong here:
//
//  1. Client-side routes (/processes, /alerts, ...) have no file on disk, so a
//     refresh or a shared link 404s. Unknown non-asset paths fall back to
//     index.html and let the router resolve them.
//  2. http.FileServer directory-lists any folder without an index.html, which
//     exposes asset filenames on an unauthenticated endpoint. Directory
//     requests are redirected to the app instead.
//
// Requests under the hashed-asset prefix keep real 404s: silently returning
// HTML for a missing .js makes cache and deploy bugs invisible.
func staticHandler(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fileServer.ServeHTTP(w, r)
			return
		}

		cleaned := path.Clean("/" + r.URL.Path)

		// Never fall back for assets — a missing bundle must fail loudly.
		if strings.HasPrefix(cleaned, "/assets/") {
			if strings.HasSuffix(cleaned, "/") {
				http.NotFound(w, r)
				return
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		if cleaned != "/" {
			// fs.FS paths are slash-separated and never rooted, so trim the
			// leading slash rather than converting to an OS path.
			if info, err := fs.Stat(root, strings.TrimPrefix(cleaned, "/")); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		serveIndex(w, r, root)
	})
}

// handleAlerts returns the currently pending and firing alerts.
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	active := []alerting.Alert{}
	if s.evaluator != nil {
		// ?host= scopes the view in a fleet; omitted means every host.
		active = s.evaluator.ActiveForHost(r.URL.Query().Get("host"))
	}
	setProtectedJSONHeaders(w)
	writeJSONBody(w, active)
}

// handleAlertRules returns the configured rules so the UI can show what is
// being evaluated rather than leaving thresholds invisible.
func (s *Server) handleAlertRules(w http.ResponseWriter, r *http.Request) {
	type ruleView struct {
		alerting.Rule
		// For is a time.Duration, which marshals as raw nanoseconds. Emit a
		// human-readable form alongside it so clients need no unit knowledge.
		ForSeconds float64 `json:"for_seconds"`
		ForLabel   string  `json:"for_label"`
	}

	rules := []alerting.Rule{}
	if s.evaluator != nil {
		rules = s.evaluator.Rules()
	}

	views := make([]ruleView, 0, len(rules))
	for _, rule := range rules {
		views = append(views, ruleView{
			Rule:       rule,
			ForSeconds: rule.For.Seconds(),
			ForLabel:   rule.For.String(),
		})
	}
	setProtectedJSONHeaders(w)
	writeJSONBody(w, views)
}

// handleAlertHistory returns recent alert transitions.
func (s *Server) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	if s.store == nil {
		setProtectedJSONHeaders(w)
		writeJSONBody(w, []storage.AlertEvent{})
		return
	}

	events, err := s.store.GetRecentAlertEvents(limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read alert history")
		return
	}
	setProtectedJSONHeaders(w)
	writeJSONBody(w, events)
}

// handleAcknowledgeAlert silences notifications for an active alert without
// resolving it.
func (s *Server) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	if rejectUnexpectedRequestBody(w, r) {
		return
	}

	ruleID := r.PathValue("ruleID")
	if ruleID == "" {
		writeJSONError(w, http.StatusBadRequest, "rule id required")
		return
	}
	if s.evaluator == nil || !s.evaluator.Acknowledge(r.URL.Query().Get("host"), ruleID) {
		writeJSONError(w, http.StatusNotFound, "no active alert for that rule")
		return
	}
	setProtectedJSONHeaders(w)
	writeJSONBody(w, map[string]string{"status": "acknowledged", "rule_id": ruleID})
}

// securityHeaders applies baseline browser protections.
//
// The server previously sent none of these. The CSP is deliberately tight —
// the dashboard is a self-contained bundle — with the single exception of the
// Google Fonts stylesheet the page links at load time.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com data:; " +
		"img-src 'self' data:; " +
		"connect-src 'self' ws: wss:; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// writeJSONBody encodes the response body.
//
// The status line and headers are already flushed by the time this runs, so an
// encoding failure cannot be turned into an HTTP error — the only useful thing
// left is to record it rather than discard it silently.
func writeJSONBody(w http.ResponseWriter, payload any) {
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write JSON response body", "error", err)
	}
}

// firstNonEmpty returns the first non-empty value, used for config fallbacks.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
