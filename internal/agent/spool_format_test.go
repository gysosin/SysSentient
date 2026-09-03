package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func TestSpoolWritesOneLinePerSample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	spool, err := NewSpool(path, 10)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	for i := range 3 {
		if err := spool.Append(models.SystemState{HostID: "h", CPUUsage: float64(i), Timestamp: time.Now()}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 — appends are not line-oriented", len(lines))
	}
	for i, line := range lines {
		var s models.SystemState
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Errorf("line %d is not a standalone JSON object: %v", i, err)
		}
	}
}

func TestSpoolReadsLegacyArrayFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	// What an agent from before the append-only change left behind.
	legacy := `[{"host_id":"h","cpu_usage":1},{"host_id":"h","cpu_usage":2}]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	spool, err := NewSpool(path, 10)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	n, err := spool.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	// An upgrade must not silently discard a buffered outage.
	if n != 2 {
		t.Fatalf("Len() = %d, want 2 — legacy buffer was dropped on upgrade", n)
	}
}

func TestSpoolSurvivesOneCorruptLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	content := `{"host_id":"h","cpu_usage":1}` + "\n" +
		`{ this line is broken` + "\n" +
		`{"host_id":"h","cpu_usage":3}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	spool, err := NewSpool(path, 10)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	n, err := spool.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	// The point of lines over one array: damage is contained to its own record.
	if n != 2 {
		t.Fatalf("Len() = %d, want 2 good samples around one corrupt line", n)
	}
}

func TestSpoolAppendAfterTruncatedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	// A sample cut off mid-line by a crash or a full disk.
	if err := os.WriteFile(path, []byte(`{"host_id":"h","cpu_us`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	spool, err := NewSpool(path, 10)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	if err := spool.Append(models.SystemState{HostID: "h", CPUUsage: 9, Timestamp: time.Now()}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// The new sample must survive: it must not be concatenated onto the
	// broken line and lost with it.
	n, err := spool.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if n != 1 {
		t.Fatalf("Len() = %d, want 1 — the good sample was corrupted by the truncated line", n)
	}
}

func TestSpoolEnforcesCapacityDespiteLazyCompaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	const capacity = 20
	spool, err := NewSpool(path, capacity)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}

	for i := range 200 {
		if err := spool.Append(models.SystemState{HostID: "h", CPUUsage: float64(i), Timestamp: time.Now()}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	n, err := spool.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	// Lazy compaction allows a bounded overshoot, never unbounded growth.
	if n > capacity+capacity/10 {
		t.Fatalf("Len() = %d, want <= %d — the buffer is not bounded", n, capacity+capacity/10)
	}

	// And the samples kept must be the most recent ones.
	samples, err := spool.Peek(capacity + capacity/10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	newest := samples[len(samples)-1]
	if newest.CPUUsage != 199 {
		t.Errorf("newest sample CPUUsage = %v, want 199 — oldest were not the ones dropped", newest.CPUUsage)
	}
}

func TestSpoolReadsLegacyArrayFollowedByAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	// Exactly what an upgraded agent leaves on disk: the old single-array
	// spool, with new line-oriented samples appended after it. Observed live —
	// the agent silently buffered forever and logged nothing.
	mixed := `[{"host_id":"h","cpu_usage":1},{"host_id":"h","cpu_usage":2}]` + "\n" +
		`{"host_id":"h","cpu_usage":3}` + "\n" +
		`{"host_id":"h","cpu_usage":4}` + "\n"
	if err := os.WriteFile(path, []byte(mixed), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	spool, err := NewSpool(path, 10)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	n, err := spool.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if n != 4 {
		t.Fatalf("Len() = %d, want 4 — samples on either side of the format change were lost", n)
	}

	samples, err := spool.Peek(10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	// Order must hold across the boundary, oldest first.
	for i, want := range []float64{1, 2, 3, 4} {
		if samples[i].CPUUsage != want {
			t.Errorf("samples[%d].CPUUsage = %v, want %v", i, samples[i].CPUUsage, want)
		}
	}
}
