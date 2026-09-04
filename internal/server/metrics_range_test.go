package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
)

func getMetrics(t *testing.T, srv *Server, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/metrics"+query, nil)
	rec := httptest.NewRecorder()
	srv.handleMetrics(rec, req)
	return rec
}

func seedRange(t *testing.T, srv *Server, hostID string, start time.Time, n int) {
	t.Helper()
	batch := make([]*models.SystemState, 0, n)
	for i := range n {
		batch = append(batch, &models.SystemState{
			HostID: hostID, Hostname: hostID,
			Timestamp: start.Add(time.Duration(i) * time.Second),
			CPUUsage:  float64(i % 100),
		})
	}
	if _, err := srv.store.SaveBatch(batch); err != nil {
		t.Fatalf("SaveBatch: %v", err)
	}
}

func TestMetricsWithoutAWindowKeepsTheOldShape(t *testing.T) {
	srv, _ := ingestServer(t)
	seedRange(t, srv, "h1", time.Now().Add(-10*time.Minute), 100)

	rec := getMetrics(t, srv, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics = %d", rec.Code)
	}
	// Existing clients receive a bare array, not the new envelope.
	var arr []models.SystemState
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Fatalf("unbounded response is not an array: %v", err)
	}
	if len(arr) == 0 {
		t.Error("no samples returned")
	}
}

func TestMetricsWithAWindowReturnsOnlyThatWindow(t *testing.T) {
	srv, _ := ingestServer(t)
	start := time.Now().Add(-30 * time.Minute).Truncate(time.Second)
	seedRange(t, srv, "h1", start, 600)

	from := start.Add(5 * time.Minute)
	to := start.Add(7 * time.Minute)
	rec := getMetrics(t, srv, "?host=h1&from="+url.QueryEscape(from.Format(time.RFC3339))+"&to="+url.QueryEscape(to.Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("range query = %d (%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Resolution string               `json:"resolution"`
		Count      int                  `json:"count"`
		Metrics    []models.SystemState `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Resolution != storage.ResolutionRaw {
		t.Errorf("resolution = %q, want raw for a recent 2-minute window", resp.Resolution)
	}
	if resp.Count != 121 {
		t.Errorf("count = %d, want 121", resp.Count)
	}
	for _, m := range resp.Metrics {
		if m.Timestamp.Before(from) || m.Timestamp.After(to) {
			t.Fatalf("sample %v falls outside the window", m.Timestamp)
		}
	}
}

func TestMetricsRejectsBadParametersInsteadOfGuessing(t *testing.T) {
	srv, _ := ingestServer(t)
	now := time.Now()

	cases := map[string]string{
		"unparseable from":   "?from=yesterday",
		"unparseable to":     "?from=" + url.QueryEscape(now.Format(time.RFC3339)) + "&to=soon",
		"inverted window":    "?from=" + url.QueryEscape(now.Format(time.RFC3339)) + "&to=" + url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339)),
		"to without from":    "?to=" + url.QueryEscape(now.Format(time.RFC3339)),
		"unknown resolution": "?resolution=10m",
		"non-numeric limit":  "?limit=lots",
		"zero limit":         "?limit=0",
		"negative limit":     "?limit=-5",
		"limit over the cap": "?limit=999999",
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			// Silently substituting a default returns the wrong answer while
			// looking like it worked.
			if rec := getMetrics(t, srv, query); rec.Code != http.StatusBadRequest {
				t.Errorf("GET /api/metrics%s = %d, want 400", query, rec.Code)
			}
		})
	}
}

func TestMetricsAutoResolutionEscalatesForLongWindows(t *testing.T) {
	srv, _ := ingestServer(t)
	now := time.Now()

	for _, tc := range []struct{ name, from, want string }{
		{"a month", now.AddDate(0, -1, 0).Format(time.RFC3339), storage.RollupFiveMinute},
		{"a day", now.Add(-24 * time.Hour).Format(time.RFC3339), storage.RollupMinute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := getMetrics(t, srv, "?from="+url.QueryEscape(tc.from)+"&to="+url.QueryEscape(now.Format(time.RFC3339)))
			if rec.Code != http.StatusOK {
				t.Fatalf("= %d (%s)", rec.Code, rec.Body.String())
			}
			var resp struct {
				Resolution string `json:"resolution"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			// A month of raw samples is millions of rows nobody can plot.
			if resp.Resolution != tc.want {
				t.Errorf("resolution = %q, want %q", resp.Resolution, tc.want)
			}
		})
	}
}

func TestMetricsHonoursAnExplicitResolution(t *testing.T) {
	srv, _ := ingestServer(t)
	now := time.Now()
	rec := getMetrics(t, srv, "?resolution=5m&from="+url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("= %d (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Resolution string `json:"resolution"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Resolution != storage.RollupFiveMinute {
		t.Errorf("resolution = %q, want the requested 5m", resp.Resolution)
	}
}

func TestMetricsAcceptsAnUnencodedTimezoneOffset(t *testing.T) {
	srv, _ := ingestServer(t)
	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	seedRange(t, srv, "h1", start, 120)

	// What curl sends when the operator does not encode the "+" in "+05:30":
	// the plus arrives as a space. The timestamp is correct; only its
	// transport mangled it.
	mangled := strings.Replace(start.Format(time.RFC3339), "+", " ", 1)
	rec := getMetrics(t, srv, "?host=h1&from="+url.QueryEscape(mangled))
	if rec.Code != http.StatusOK {
		t.Fatalf("= %d (%s)", rec.Code, rec.Body.String())
	}
}
