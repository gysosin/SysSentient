package logs

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Every platform must supply at least one log source. An empty list would make
// the AI prompt silently lose its log context on that platform — the daemon
// would keep running and the analysis would just get quietly worse, which is
// the kind of degradation nobody notices until it matters.
func TestPlatformLogSourcesIsNotEmpty(t *testing.T) {
	t.Parallel()
	if len(platformLogSources) == 0 {
		t.Fatal("no log sources registered for this platform")
	}
	for i, src := range platformLogSources {
		if strings.TrimSpace(src.heading) == "" {
			t.Errorf("source %d has no heading", i)
		}
		if src.fetch == nil {
			t.Errorf("source %q has no fetch function", src.heading)
		}
	}
}

// A source that fails must not take the others down with it. journalctl may be
// absent, dmesg needs privileges the hardened systemd unit deliberately drops,
// and the Windows event log may be unreadable for the service account — so
// partial failure is the normal case, not an exceptional one.
func TestGetRecentLogsSurvivesAFailingSource(t *testing.T) {
	t.Parallel()
	r := NewLogReader(10)

	calls := 0
	r.runCommand = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("command not found")
		}
		return []byte("something went wrong\n"), nil
	}

	out, err := r.GetRecentLogsContext(context.Background())
	if err != nil {
		t.Fatalf("GetRecentLogsContext returned an error: %v", err)
	}
	if calls != len(platformLogSources) {
		t.Fatalf("tried %d sources, want all %d", calls, len(platformLogSources))
	}
	if len(platformLogSources) > 1 && !strings.Contains(out, "something went wrong") {
		t.Fatalf("output lost the surviving source's content:\n%s", out)
	}
}

// No source producing anything is a valid answer, not an error: a machine with
// nothing to report is the common case.
func TestGetRecentLogsReportsQuietSystem(t *testing.T) {
	t.Parallel()
	r := NewLogReader(10)
	r.runCommand = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("unavailable")
	}

	out, err := r.GetRecentLogsContext(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No recent errors") {
		t.Fatalf("want the quiet-system message, got: %q", out)
	}
}
