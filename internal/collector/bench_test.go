package collector

import (
	"testing"
	"time"
)

// Collect is called on every tick of the daemon's main loop, so its cost is
// paid continuously for the life of the process. The baseline measured on the
// daemon was 4.1% of one core at a two-second interval, which is several times
// what a monitoring agent should cost the machine it monitors.
func BenchmarkCollect(b *testing.B) {
	c := NewCollector(10)
	// Warm the caches: the first call has no previous sample to diff against
	// and pays a different, one-off cost.
	if _, err := c.Collect(); err != nil {
		b.Fatalf("warmup: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Collect(); err != nil {
			b.Fatalf("collect: %v", err)
		}
	}
}

// getTopProcesses walks every PID on the machine. It is the part of Collect
// whose cost scales with how busy the host is, so it is measured separately.
func BenchmarkGetTopProcesses(b *testing.B) {
	c := NewCollector(10)
	_, _ = c.getTopProcesses(10, time.Now())

	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.getTopProcesses(10, time.Now()); err != nil {
			b.Fatalf("processes: %v", err)
		}
	}
}
