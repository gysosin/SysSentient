package agent

import (
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

// realisticSample approximates a production sample: the per-core, process and
// filesystem slices are what make a row ~4 KB on the wire.
func realisticSample() models.SystemState {
	cores := make([]float64, 16)
	procs := make([]models.Process, 10)
	for i := range procs {
		procs[i] = models.Process{
			PID: int32(i), Name: "some-service-name", User: "root", CPU: 1.5, Memory: 128, State: "S",
		}
	}
	return models.SystemState{
		HostID: "bench-host", Hostname: "bench", Timestamp: time.Now(),
		CPUUsage: 42, CPUPerCore: cores, Processes: procs,
	}
}

// BenchmarkSpoolAppendWhenFull measures the cost of one append against a full
// buffer — the state an agent is in throughout a long outage.
func BenchmarkSpoolAppendWhenFull(b *testing.B) {
	spool, err := NewSpool(filepath.Join(b.TempDir(), "spool.jsonl"), 5000)
	if err != nil {
		b.Fatalf("NewSpool: %v", err)
	}
	sample := realisticSample()
	for range 5000 {
		if err := spool.Append(sample); err != nil {
			b.Fatalf("seed: %v", err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		if err := spool.Append(sample); err != nil {
			b.Fatalf("Append: %v", err)
		}
	}
}
