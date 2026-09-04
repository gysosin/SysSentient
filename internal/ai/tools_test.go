package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// stubBox records what it was asked and returns canned text.
type stubBox struct {
	calls    []string
	from, to time.Time
	at       time.Time
	limit    int
	fail     bool
	empty    bool
}

func (b *stubBox) note(name string) (string, error) {
	b.calls = append(b.calls, name)
	if b.fail {
		return "", errors.New("storage is down")
	}
	if b.empty {
		return "", nil
	}
	return name + " ok", nil
}

func (b *stubBox) QueryMetrics(_ context.Context, _ string, from, to time.Time) (string, error) {
	b.from, b.to = from, to
	return b.note("query_metrics")
}
func (b *stubBox) TopProcesses(_ context.Context, _ string, at time.Time) (string, error) {
	b.at = at
	return b.note("top_processes")
}
func (b *stubBox) ListHosts(context.Context) (string, error) { return b.note("list_hosts") }
func (b *stubBox) RecentAlerts(_ context.Context, limit int) (string, error) {
	b.limit = limit
	return b.note("recent_alerts")
}
func (b *stubBox) RecentLogs(context.Context) (string, error) { return b.note("recent_logs") }
func (b *stubBox) RecentInsights(_ context.Context, limit int) (string, error) {
	b.limit = limit
	return b.note("recent_insights")
}

func TestDispatchParsesTimestamps(t *testing.T) {
	box := &stubBox{}
	out := dispatchTool(context.Background(), box, toolQueryMetrics, map[string]any{
		"from": "2026-09-04T10:00:00Z",
		"to":   "2026-09-04T11:00:00Z",
	})
	if !strings.Contains(out, "ok") {
		t.Fatalf("dispatch returned %q", out)
	}
	if box.from.UTC().Hour() != 10 || box.to.UTC().Hour() != 11 {
		t.Errorf("window = %v..%v", box.from, box.to)
	}
}

func TestDispatchReportsBadArgumentsToTheModel(t *testing.T) {
	box := &stubBox{}
	// Returned as text, not as an error: the model can correct itself and try
	// again, where aborting the turn would lose the work already done.
	for name, args := range map[string]map[string]any{
		"missing from":     {"to": "2026-09-04T11:00:00Z"},
		"unparseable from": {"from": "yesterday", "to": "2026-09-04T11:00:00Z"},
		"missing at":       {},
	} {
		t.Run(name, func(t *testing.T) {
			tool := toolQueryMetrics
			if name == "missing at" {
				tool = toolTopProcesses
			}
			out := dispatchTool(context.Background(), box, tool, args)
			if !strings.HasPrefix(out, "error:") {
				t.Errorf("got %q, want an error string the model can read", out)
			}
		})
	}
}

func TestDispatchDistinguishesEmptyFromBroken(t *testing.T) {
	empty := dispatchTool(context.Background(), &stubBox{empty: true}, toolListHosts, nil)
	if empty != "no data for that request" {
		t.Errorf("empty result = %q", empty)
	}

	broken := dispatchTool(context.Background(), &stubBox{fail: true}, toolListHosts, nil)
	// "Nothing there" and "I could not look" are different answers, and the
	// model must be able to tell them apart.
	if !strings.HasPrefix(broken, "error:") {
		t.Errorf("failure = %q, want an error prefix", broken)
	}
}

func TestDispatchAppliesLimitDefaults(t *testing.T) {
	box := &stubBox{}
	dispatchTool(context.Background(), box, toolRecentAlerts, map[string]any{})
	if box.limit != 20 {
		t.Errorf("alerts limit = %d, want the default 20", box.limit)
	}
	dispatchTool(context.Background(), box, toolRecentAlerts, map[string]any{"limit": float64(5)})
	if box.limit != 5 {
		t.Errorf("alerts limit = %d, want the requested 5", box.limit)
	}
}

func TestDispatchRejectsAnUnknownTool(t *testing.T) {
	out := dispatchTool(context.Background(), &stubBox{}, "rm_rf", nil)
	if !strings.Contains(out, "unknown tool") {
		t.Errorf("got %q", out)
	}
}

func TestToolDeclarationsCoverEveryDispatchedTool(t *testing.T) {
	declared := map[string]bool{}
	for _, tool := range toolDeclarations() {
		for _, fn := range tool.FunctionDeclarations {
			declared[fn.Name] = true
			// A model picks between tools from these sentences alone.
			if len(fn.Description) < 40 {
				t.Errorf("%s has a description too short to choose from", fn.Name)
			}
		}
	}
	// A tool the dispatcher handles but never declares is unreachable; one
	// declared but not handled is an error the model cannot avoid.
	for _, name := range []string{
		toolQueryMetrics, toolTopProcesses, toolListHosts,
		toolRecentAlerts, toolRecentLogs, toolRecentInsights,
	} {
		if !declared[name] {
			t.Errorf("%s is dispatched but not declared", name)
		}
	}
	if len(declared) != 6 {
		t.Errorf("declared %d tools, want 6", len(declared))
	}
}
