package collector

import (
	"math"
	"strings"
	"sys-sentient/internal/models"
	"testing"
	"time"
)

func TestCompactProcessNameNormalizesAndTruncates(t *testing.T) {
	raw := "chrome --type=renderer,\n--some-long-flag=" + strings.Repeat("x", 120)

	got := compactProcessName(raw)
	if strings.Contains(got, ",") || strings.Contains(got, "\n") {
		t.Fatalf("expected compact name to remove commas and newlines, got %q", got)
	}
	if len(got) != 80 {
		t.Fatalf("expected compact name to be 80 chars, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected compact name to end with ellipsis, got %q", got)
	}
}

func TestFormatTopProcessesOmitsUsernames(t *testing.T) {
	got := formatTopProcesses([]models.Process{
		{PID: 42, Name: "chrome", User: "alice", CPU: 12.5, Memory: 256},
	})

	if strings.Contains(got, "alice") {
		t.Fatalf("expected summary to omit usernames, got %q", got)
	}
	if got != "chrome (12.5%, 256MB)" {
		t.Fatalf("unexpected process summary: %q", got)
	}
}

func TestCounterDeltaHandlesCounterReset(t *testing.T) {
	if got := counterDelta(150, 100); got != 50 {
		t.Fatalf("expected normal delta 50, got %d", got)
	}
	if got := counterDelta(10, 100); got != 0 {
		t.Fatalf("expected reset delta 0, got %d", got)
	}
}

func TestNewCollectorTopProcesses(t *testing.T) {
	tests := []struct {
		name  string
		given int
		want  int
	}{
		{name: "configured value is used", given: 25, want: 25},
		{name: "one is honoured", given: 1, want: 1},
		{name: "zero falls back to default", given: 0, want: defaultTopProcesses},
		{name: "negative falls back to default", given: -5, want: defaultTopProcesses},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCollector(tt.given)
			if c.topProcesses != tt.want {
				t.Fatalf("NewCollector(%d).topProcesses = %d, want %d", tt.given, c.topProcesses, tt.want)
			}
			if c.procCache == nil {
				t.Fatal("NewCollector() left procCache nil")
			}
		})
	}
}

func TestCollectPopulatesHostIdentity(t *testing.T) {
	// Hostname is the first step toward a multi-host model: every sample must
	// be attributable to the machine that produced it. Previously SystemState
	// carried no host identity at all.
	c := NewCollector(3)

	state, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if state.Hostname == "" {
		t.Fatal("Collect() left Hostname empty")
	}
	if state.UptimeSeconds == 0 {
		t.Fatal("Collect() left UptimeSeconds zero")
	}
}

func TestCollectReportsFilesystemCapacity(t *testing.T) {
	// "Disk full" is the most common production incident and the collector
	// could not see it: disk.Usage() was never called, so only IO byte counters
	// existed. Every real filesystem must report a usable capacity.
	c := NewCollector(3)

	state, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(state.Filesystems) == 0 {
		t.Fatal("Collect() reported no filesystems")
	}

	var foundRoot bool
	for _, fs := range state.Filesystems {
		if fs.Mountpoint == "" {
			t.Errorf("filesystem with empty mountpoint: %+v", fs)
		}
		if fs.TotalBytes == 0 {
			t.Errorf("filesystem %q reported zero total bytes", fs.Mountpoint)
		}
		if fs.UsedPercent < 0 || fs.UsedPercent > 100 {
			t.Errorf("filesystem %q used percent out of range: %v", fs.Mountpoint, fs.UsedPercent)
		}
		if fs.UsedBytes > fs.TotalBytes {
			t.Errorf("filesystem %q used > total", fs.Mountpoint)
		}
		if fs.Mountpoint == "/" {
			foundRoot = true
		}
	}

	if !foundRoot {
		t.Errorf("root filesystem not reported; got %d filesystems", len(state.Filesystems))
	}
}

func TestFilesystemsAreDeduplicatedAndBounded(t *testing.T) {
	// Bind mounts and overlay/container filesystems make the raw partition list
	// long and repetitive; the payload ships on every WebSocket frame.
	c := NewCollector(3)

	state, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(state.Filesystems) > maxFilesystems {
		t.Fatalf("reported %d filesystems, want at most %d", len(state.Filesystems), maxFilesystems)
	}

	seen := make(map[string]bool, len(state.Filesystems))
	for _, fs := range state.Filesystems {
		if seen[fs.Mountpoint] {
			t.Fatalf("duplicate mountpoint reported: %q", fs.Mountpoint)
		}
		seen[fs.Mountpoint] = true
	}
}

func TestProcessCPUPercentUsesDelta(t *testing.T) {
	// gopsutil's p.CPUPercent() is a LIFETIME average: total CPU time divided by
	// process age. A process that pegged a core an hour ago and is now idle
	// still ranks first, while one spiking right now ranks low. The collector
	// must derive current usage from the delta between polls instead.
	tests := []struct {
		name          string
		baseSeconds   float64
		sampleSeconds float64
		elapsed       time.Duration
		want          float64
	}{
		{name: "one full core", baseSeconds: 10, sampleSeconds: 20, elapsed: 10 * time.Second, want: 100},
		{name: "half a core", baseSeconds: 10, sampleSeconds: 15, elapsed: 10 * time.Second, want: 50},
		{name: "quarter core", baseSeconds: 10, sampleSeconds: 12.5, elapsed: 10 * time.Second, want: 25},
		{name: "idle despite a huge lifetime total", baseSeconds: 5000, sampleSeconds: 5000, elapsed: 10 * time.Second, want: 0},
		{name: "counter reset reports zero, not negative", baseSeconds: 50, sampleSeconds: 10, elapsed: 10 * time.Second, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh collector per case: processCPUPercent advances the baseline
			// as a side effect, so cases must not share one.
			c := NewCollector(5)
			now := time.Now()

			const pid int32 = 100
			c.lastProcCPU[pid] = procCPUSample{
				totalSeconds: tt.baseSeconds,
				at:           now.Add(-tt.elapsed),
			}

			got := c.processCPUPercent(pid, tt.sampleSeconds, now)
			if math.Abs(got-tt.want) > 0.5 {
				t.Fatalf("processCPUPercent() = %.2f, want ~%.2f", got, tt.want)
			}

			// The new reading must become the next baseline.
			if c.lastProcCPU[pid].totalSeconds != tt.sampleSeconds {
				t.Fatalf("baseline = %.2f, want %.2f", c.lastProcCPU[pid].totalSeconds, tt.sampleSeconds)
			}
		})
	}
}

func TestProcessCPUPercentFirstSampleIsZero(t *testing.T) {
	// An unseen PID has no baseline. Reporting its lifetime average here would
	// reintroduce exactly the bug this replaces.
	c := NewCollector(5)

	if got := c.processCPUPercent(999, 1234, time.Now()); got != 0 {
		t.Fatalf("first sample for an unseen pid = %.2f, want 0", got)
	}

	// The sample must be recorded so the next poll can produce a delta.
	if _, ok := c.lastProcCPU[999]; !ok {
		t.Fatal("processCPUPercent() did not record a baseline for the new pid")
	}
}
