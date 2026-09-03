package config

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func base() *Runtime {
	return NewRuntime(&Config{
		Collector: CollectorConfig{PollIntervalSeconds: 2},
		Database:  DatabaseConfig{MetricsRetentionHours: 24, MinuteRollupDays: 30, FiveMinuteRollupDays: 365},
		Logging:   LoggingConfig{Level: "info"},
	})
}

func ptr[T any](v T) *T { return &v }

// The setting the whole feature exists for: changing the interval must retime
// the collector, not merely record a number.
func TestApplyPollIntervalNotifiesTheCollector(t *testing.T) {
	t.Parallel()
	rt := base()

	var got time.Duration
	rt.OnPollIntervalChange(func(d time.Duration) { got = d })

	if _, err := rt.Apply(RuntimeUpdate{PollIntervalSeconds: ptr(10)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != 10*time.Second {
		t.Errorf("collector was notified with %v, want 10s", got)
	}
	if v := rt.Values().PollIntervalSeconds; v != 10 {
		t.Errorf("stored interval %d, want 10", v)
	}
}

// Setting a value to what it already is must not churn the collector.
func TestApplyDoesNotNotifyWhenUnchanged(t *testing.T) {
	t.Parallel()
	rt := base()
	notified := 0
	rt.OnPollIntervalChange(func(time.Duration) { notified++ })

	if _, err := rt.Apply(RuntimeUpdate{PollIntervalSeconds: ptr(2)}); err != nil {
		t.Fatal(err)
	}
	if notified != 0 {
		t.Errorf("collector retimed %d times for an unchanged value", notified)
	}
}

// A partial update must leave the other fields alone. Zeroing them would be
// the obvious bug, and it would silently shorten retention to nothing.
func TestApplyIsPartial(t *testing.T) {
	t.Parallel()
	rt := base()
	before := rt.Values()

	if _, err := rt.Apply(RuntimeUpdate{LogLevel: ptr("debug")}); err != nil {
		t.Fatal(err)
	}
	after := rt.Values()

	if after.LogLevel != "debug" {
		t.Errorf("log level = %q, want debug", after.LogLevel)
	}
	if after.PollIntervalSeconds != before.PollIntervalSeconds ||
		after.MetricsRetentionHours != before.MetricsRetentionHours ||
		after.MinuteRollupDays != before.MinuteRollupDays ||
		after.FiveMinuteRollupDays != before.FiveMinuteRollupDays {
		t.Errorf("unrelated settings changed: %+v -> %+v", before, after)
	}
}

// An invalid update must change nothing at all, not partially apply.
func TestApplyRejectsInvalidWithoutMutating(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		update  RuntimeUpdate
		wantErr string
	}{
		{"interval too small", RuntimeUpdate{PollIntervalSeconds: ptr(0)}, "poll interval"},
		{"interval too large", RuntimeUpdate{PollIntervalSeconds: ptr(4000)}, "poll interval"},
		{"retention below an hour", RuntimeUpdate{MetricsRetentionHours: ptr(0)}, "metrics retention"},
		{"unknown log level", RuntimeUpdate{LogLevel: ptr("verbose")}, "log level"},
		// Individually valid, contradictory together: the coarse tier would be
		// deleted before the fine tier it is derived from.
		{"five-minute shorter than minute", RuntimeUpdate{FiveMinuteRollupDays: ptr(7)}, "must be at least the minute retention"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := base()
			before := rt.Values()

			_, err := rt.Apply(tc.update)
			if err == nil {
				t.Fatalf("Apply(%+v) succeeded, want an error", tc.update)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tc.wantErr)
			}
			if rt.Values() != before {
				t.Errorf("rejected update still mutated state: %+v -> %+v", before, rt.Values())
			}
		})
	}
}

// The collector reads PollInterval on every tick while the API may be writing
// it, so this has to be race free.
func TestRuntimeIsSafeUnderConcurrentAccess(t *testing.T) {
	t.Parallel()
	rt := base()

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = rt.Apply(RuntimeUpdate{PollIntervalSeconds: ptr(1 + i%5)}) }()
		go func() { defer wg.Done(); _ = rt.PollInterval(); _ = rt.Values() }()
	}
	wg.Wait()
}
