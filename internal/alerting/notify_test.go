package alerting

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sampleAlert() Alert {
	return Alert{
		RuleID: "cpu-high", RuleName: "CPU sustained high", Metric: MetricCPUUsage,
		State: StateFiring, Severity: SeverityCritical, Value: 96.5, Threshold: 90,
		Hostname: "web-01", StartedAt: time.Now(), FiredAt: time.Now(),
	}
}

func TestWebhookNotifierPostsAlertJSON(t *testing.T) {
	var (
		mu     sync.Mutex
		gotCT  string
		gotRaw []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotCT = r.Header.Get("Content-Type")
		gotRaw = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := NewWebhookNotifier(srv.URL).Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotCT)
	}

	var decoded Alert
	if err := json.Unmarshal(gotRaw, &decoded); err != nil {
		t.Fatalf("payload is not valid Alert JSON: %v (%s)", err, gotRaw)
	}
	if decoded.RuleID != "cpu-high" || decoded.Value != 96.5 || decoded.Hostname != "web-01" {
		t.Fatalf("payload lost fields: %+v", decoded)
	}
}

func TestSlackNotifierFormatsFiringAndResolved(t *testing.T) {
	var (
		mu   sync.Mutex
		body string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(raw)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)

	if err := notifier.Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatalf("Notify(firing) error = %v", err)
	}
	mu.Lock()
	firing := body
	mu.Unlock()
	if !strings.Contains(firing, "FIRING") || !strings.Contains(firing, "web-01") {
		t.Fatalf("firing payload = %s, want FIRING and the hostname", firing)
	}

	resolved := sampleAlert()
	resolved.State = StateResolved
	if err := notifier.Notify(context.Background(), resolved); err != nil {
		t.Fatalf("Notify(resolved) error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(body, "RESOLVED") {
		t.Fatalf("resolved payload = %s, want RESOLVED", body)
	}
}

func TestNotifierReportsNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := NewWebhookNotifier(srv.URL).Notify(context.Background(), sampleAlert()); err == nil {
		t.Fatal("Notify() = nil, want an error for a 500 response")
	}
}

func TestDispatcherSkipsAcknowledgedAlerts(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dispatcher := NewDispatcher(discardLogger(), NewWebhookNotifier(srv.URL))

	acked := sampleAlert()
	acked.Acknowledged = true

	dispatcher.Dispatch(context.Background(), []Alert{acked, sampleAlert()})

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("notifier called %d times, want 1 (the acknowledged alert must be skipped)", calls)
	}
}

func TestDispatcherContinuesAfterAChannelFails(t *testing.T) {
	// One broken channel must not stop the others.
	var mu sync.Mutex
	good := 0
	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		good++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer goodSrv.Close()

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer badSrv.Close()

	dispatcher := NewDispatcher(discardLogger(),
		NewWebhookNotifier(badSrv.URL),
		NewWebhookNotifier(goodSrv.URL),
	)
	dispatcher.Dispatch(context.Background(), []Alert{sampleAlert()})

	mu.Lock()
	defer mu.Unlock()
	if good != 1 {
		t.Fatalf("healthy channel called %d times, want 1", good)
	}
}

func TestBuildNotifiers(t *testing.T) {
	tests := []struct {
		name    string
		webhook string
		slack   string
		want    int
	}{
		{name: "none configured", want: 0},
		{name: "webhook only", webhook: "https://example.test/hook", want: 1},
		{name: "slack only", slack: "https://hooks.slack.test/x", want: 1},
		{name: "both", webhook: "https://example.test/hook", slack: "https://hooks.slack.test/x", want: 2},
		{name: "whitespace is not a url", webhook: "   ", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildNotifiers(tt.webhook, tt.slack)
			if len(got) != tt.want {
				t.Fatalf("BuildNotifiers() = %d notifiers, want %d", len(got), tt.want)
			}
		})
	}
}

func TestDispatcherWithNoChannelsIsSafe(t *testing.T) {
	d := NewDispatcher(discardLogger())
	if d.Enabled() {
		t.Fatal("Enabled() = true with no channels")
	}
	// Must not panic.
	d.Dispatch(context.Background(), []Alert{sampleAlert()})
}
