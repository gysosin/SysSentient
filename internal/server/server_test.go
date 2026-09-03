package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sys-sentient/internal/alerting"
	"sys-sentient/internal/config"
	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
	"testing"
	"testing/fstest"
	"time"
)

type fakeLogCollector struct {
	content string
	err     error
}

func (f fakeLogCollector) GetLogsWithTimeout(time.Duration) (string, error) {
	return f.content, f.err
}

func (f fakeLogCollector) GetLogsContextWithTimeout(context.Context, time.Duration) (string, error) {
	return f.content, f.err
}

func TestNewHTTPServerSetsProductionTimeouts(t *testing.T) {
	srv := newHTTPServer(":8080", http.NewServeMux())

	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout must be set to defend against slow headers")
	}
	if srv.ReadTimeout <= 0 {
		t.Fatal("ReadTimeout must be set to bound request reads")
	}
	if srv.WriteTimeout <= 0 {
		t.Fatal("WriteTimeout must be set to bound stuck responses")
	}
	if srv.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout must be set to bound idle keep-alive connections")
	}
}

func TestServerOriginPolicyAllowsConfiguredOrigins(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080", "http://localhost:3000"},
	}, testPrivacy(), nil, nil, nil, nil)

	if !srv.isOriginAllowed("http://localhost:3000") {
		t.Fatal("expected configured origin to be allowed")
	}
	if srv.isOriginAllowed("https://evil.example") {
		t.Fatal("expected unconfigured origin to be rejected")
	}
}

func TestServerOriginPolicyAllowsEmptyOrigin(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080"},
	}, testPrivacy(), nil, nil, nil, nil)

	if !srv.isOriginAllowed("") {
		t.Fatal("expected empty origin to be allowed for same-origin and non-browser clients")
	}
}

func TestCORSRejectsDisallowedPreflightOrigin(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080"},
	}, testPrivacy(), nil, nil, nil, nil)
	handler := srv.enableCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/metrics", nil)
	req.Header.Set("Origin", "https://evil.example")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected disallowed preflight status 403, got %d", rec.Code)
	}
}

func TestCORSMarksOriginVaryForOriginRequests(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080"},
	}, testPrivacy(), nil, nil, nil, nil)
	handler := srv.enableCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Vary") != "Origin" {
		t.Fatalf("expected Vary: Origin, got %q", rec.Header().Get("Vary"))
	}
}

func TestHandleHealthReportsStorageStatus(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	srv := NewServer(config.ServerConfig{}, testPrivacy(), store, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected healthy status 200, got %d", rec.Code)
	}
	var healthy map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &healthy); err != nil {
		t.Fatalf("expected JSON health response: %v", err)
	}
	if healthy["database"] != "ok" {
		t.Fatalf("expected database status ok, got %v", healthy["database"])
	}

	if err := store.Close(); err != nil {
		t.Fatalf("failed to close test store: %v", err)
	}

	rec = httptest.NewRecorder()
	srv.handleHealth(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unhealthy status 503, got %d", rec.Code)
	}
}

func TestWriteInsightResponsePreservesJSONObject(t *testing.T) {
	rec := httptest.NewRecorder()
	raw := `{"status":"Healthy","summary":"ok","recommendedActions":[]}`

	if err := writeInsightResponse(rec, raw); err != nil {
		t.Fatalf("writeInsightResponse returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if payload["status"] != "Healthy" {
		t.Fatalf("expected status to be preserved, got %v", payload["status"])
	}
}

func TestWriteInsightResponseWrapsPlainText(t *testing.T) {
	rec := httptest.NewRecorder()

	if err := writeInsightResponse(rec, "plain diagnostic text"); err != nil {
		t.Fatalf("writeInsightResponse returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if payload["detailedAnalysis"] != "plain diagnostic text" {
		t.Fatalf("expected plain text to be wrapped, got %v", payload["detailedAnalysis"])
	}
	if _, ok := payload["recommendedActions"].([]any); !ok {
		t.Fatalf("expected recommendedActions array, got %T", payload["recommendedActions"])
	}
}

func TestHandleLogsReturnsScrubbedContent(t *testing.T) {
	srv := NewServer(config.ServerConfig{}, testPrivacy(), nil, nil, nil, nil)
	srv.logReader = fakeLogCollector{
		content: "May 09 kernel warning for admin@example.com from 10.0.0.8 in /home/alice/app",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	srv.handleLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected logs response to disable caching, got %q", rec.Header().Get("Cache-Control"))
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON logs response: %v", err)
	}

	content := payload["content"]
	for _, sensitive := range []string{"admin@example.com", "10.0.0.8", "/home/alice"} {
		if strings.Contains(content, sensitive) {
			t.Fatalf("expected log content to redact %q, got %q", sensitive, content)
		}
	}
	if !strings.Contains(content, "[EMAIL_REDACTED]") || !strings.Contains(content, "[IP_REDACTED]") {
		t.Fatalf("expected redaction markers in log content, got %q", content)
	}
}

func TestHandleLogsReportsCollectionFailure(t *testing.T) {
	srv := NewServer(config.ServerConfig{}, testPrivacy(), nil, nil, nil, nil)
	srv.logReader = fakeLogCollector{err: errors.New("journal unavailable")}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	srv.handleLogs(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON error response: %v", err)
	}
	if payload["error"] != "failed to collect logs" {
		t.Fatalf("expected sanitized log error, got %q", payload["error"])
	}
}

func TestHandleAnalyzeRejectsUnexpectedRequestBody(t *testing.T) {
	srv := NewServer(config.ServerConfig{}, testPrivacy(), nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", strings.NewReader(`{"scope":"all"}`))
	srv.handleAnalyze(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON error response: %v", err)
	}
	if payload["error"] != "request body not supported" {
		t.Fatalf("expected unsupported body error, got %q", payload["error"])
	}
}

// testPrivacy mirrors the all-masking default the server used to hardcode, so
// existing assertions keep their meaning after privacy became configurable.
func testPrivacy() config.PrivacyConfig {
	return config.PrivacyConfig{MaskIPs: true, MaskEmails: true, MaskUsernames: true}
}

func TestHandleLogsHonoursPrivacyConfig(t *testing.T) {
	const raw = "conn from 10.0.0.5 for user@example.com"

	tests := []struct {
		name        string
		privacy     config.PrivacyConfig
		wantRedact  bool
		wantLiteral string
	}{
		{
			name:        "masking enabled redacts",
			privacy:     config.PrivacyConfig{MaskIPs: true, MaskEmails: true, MaskUsernames: true},
			wantRedact:  true,
			wantLiteral: "[IP_REDACTED]",
		},
		{
			name:        "masking disabled passes through",
			privacy:     config.PrivacyConfig{},
			wantRedact:  false,
			wantLiteral: "10.0.0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServer(config.ServerConfig{}, tt.privacy, nil, nil, nil, nil)
			got := srv.scrubber.SanitizeLog(raw)

			if !strings.Contains(got, tt.wantLiteral) {
				t.Fatalf("SanitizeLog(%q) = %q, want it to contain %q", raw, got, tt.wantLiteral)
			}
			if tt.wantRedact && strings.Contains(got, "10.0.0.5") {
				t.Fatalf("SanitizeLog(%q) = %q, raw IP survived despite masking", raw, got)
			}
		})
	}
}

func TestSPAFallbackServesIndexForClientRoutes(t *testing.T) {
	// The dashboard is a single-page app with client-side routes. A plain
	// http.FileServer 404s on /processes, so a refresh or a shared link would
	// break. Unknown non-API paths must return index.html.
	dir := t.TempDir()
	distDir := filepath.Join(dir, "web", "dist")
	if err := os.MkdirAll(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<!doctype html>SPA"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler := staticHandler(os.DirFS(distDir))

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		{name: "root serves index", path: "/", wantCode: http.StatusOK, wantBody: "SPA"},
		{name: "client route falls back to index", path: "/processes", wantCode: http.StatusOK, wantBody: "SPA"},
		{name: "nested client route falls back", path: "/alerts/detail", wantCode: http.StatusOK, wantBody: "SPA"},
		{name: "real asset is served", path: "/assets/app.js", wantCode: http.StatusOK, wantBody: "console.log(1)"},
		{name: "missing asset 404s rather than serving html", path: "/assets/missing.js", wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("GET %s body = %q, want it to contain %q", tt.path, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestStaticHandlerDoesNotListDirectories(t *testing.T) {
	// http.FileServer directory-lists any folder without an index.html.
	dir := t.TempDir()
	distDir := filepath.Join(dir, "web", "dist")
	if err := os.MkdirAll(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("SPA"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "assets", "secret.js"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/", nil)
	rec := httptest.NewRecorder()
	staticHandler(os.DirFS(distDir)).ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), "secret.js") {
		t.Fatalf("directory listing leaked file names: %q", rec.Body.String())
	}
}

func TestSecurityHeadersArePresent(t *testing.T) {
	// The server previously sent no security headers at all: no CSP, no
	// clickjacking protection, no MIME-sniffing protection.
	srv := NewServer(config.ServerConfig{}, testPrivacy(), nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, expected := range want {
		if got := rec.Header().Get(header); got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy not set")
	}
	for _, directive := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP %q missing %q", csp, directive)
		}
	}
	// The dashboard loads its font from Google Fonts; the CSP must permit it
	// or the UI silently falls back.
	if !strings.Contains(csp, "fonts.googleapis.com") {
		t.Errorf("CSP %q does not allow the Google Fonts stylesheet", csp)
	}
}

func TestPrometheusMetricsExposition(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	if err := store.Save(&models.SystemState{
		Hostname: "web-01", Timestamp: time.Now(), CPUUsage: 42.5,
		MemoryUsed: 8 << 30, MemoryTotal: 16 << 30, LoadAvg1: 1.5, Temperature: 65,
		Filesystems: []models.Filesystem{
			{Mountpoint: "/", FSType: "btrfs", TotalBytes: 100, FreeBytes: 40, UsedPercent: 60},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	srv := NewServer(config.ServerConfig{}, testPrivacy(), store, nil, alerting.NewEvaluator(nil), nil)

	rec := httptest.NewRecorder()
	srv.handlePrometheusMetrics(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain exposition", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"# HELP sys_sentient_build_info",
		"# TYPE sys_sentient_cpu_usage_percent gauge",
		"sys_sentient_cpu_usage_percent{host=\"web-01\"} 42.5",
		"sys_sentient_goroutines",
		"sys_sentient_filesystem_used_percent{fstype=\"btrfs\",host=\"web-01\",mountpoint=\"/\"} 60",
		"sys_sentient_alerts_active{state=\"firing\"} 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q\n---\n%s", want, body)
		}
	}
}

func TestPrometheusLabelsAreEscapedAndOrdered(t *testing.T) {
	// Mountpoints can contain quotes and backslashes; unescaped they would
	// produce an exposition Prometheus refuses to parse.
	got := formatLabels(map[string]string{
		"mountpoint": `/mnt/a"b\c`,
		"host":       "h1",
		"empty":      "",
	})

	if !strings.HasPrefix(got, `{host="h1"`) {
		t.Fatalf("labels not sorted: %s", got)
	}
	if !strings.Contains(got, `\"`) || !strings.Contains(got, `\\`) {
		t.Fatalf("label value not escaped: %s", got)
	}
	if strings.Contains(got, "empty=") {
		t.Fatalf("empty label should be omitted: %s", got)
	}
}

func TestHealthReportsVersionAndCollectorLiveness(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "health.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	srv := NewServer(config.ServerConfig{}, testPrivacy(), store, nil, nil, nil)

	// A stale sample must degrade health: a wedged collector previously still
	// reported "healthy" because only the database was pinged.
	if err := store.Save(&models.SystemState{Hostname: "h", Timestamp: time.Now().Add(-10 * time.Minute)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	rec := httptest.NewRecorder()
	srv.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body not JSON: %v", err)
	}

	if body["version"] == nil || body["version"] == "" {
		t.Error("health response carries no version")
	}
	if body["collector"] != "stale" {
		t.Errorf("collector = %v, want stale", body["collector"])
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded for a stale collector", body["status"])
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want 503 when degraded", rec.Code)
	}
}

// staticHandler must work over any fs.FS, not just a real directory: the
// daemon serves the dashboard from an embed.FS in every packaged build, and
// only from disk during development. Exercising it against a synthetic FS
// catches an os-path assumption creeping back in, which is what made the
// binary non-relocatable in the first place.
func TestStaticHandlerServesFromSyntheticFS(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/app-abc.js": {Data: []byte("console.log(1)")},
		"favicon.svg":       {Data: []byte("<svg/>")},
	}
	handler := staticHandler(fsys)

	for _, tc := range []struct {
		name, path string
		wantStatus int
	}{
		{"root serves index", "/", http.StatusOK},
		{"real asset is served", "/assets/app-abc.js", http.StatusOK},
		{"real file is served", "/favicon.svg", http.StatusOK},
		// A client-side route has no file behind it and must reach the SPA.
		{"unknown route falls back to index", "/settings", http.StatusOK},
		// A missing asset must fail loudly rather than silently returning
		// HTML, which would otherwise surface as a confusing MIME-type error.
		{"missing asset is not masked by the fallback", "/assets/nope.js", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("GET %s = %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
		})
	}
}
