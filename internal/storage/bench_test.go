package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

// sampleState is representative of what the collector actually writes: a
// per-core slice, ten processes and a handful of filesystems, all of which are
// JSON-encoded into the row. A benchmark against an empty struct would measure
// the driver and miss the encoding, which is where most of the 4.3 KB per row
// comes from.
func sampleState(cores int) *models.SystemState {
	perCore := make([]float64, cores)
	for i := range perCore {
		perCore[i] = float64(i%100) + 0.5
	}

	procs := make([]models.Process, 10)
	for i := range procs {
		procs[i] = models.Process{
			PID:    int32(1000 + i),
			Name:   fmt.Sprintf("process-%d", i),
			User:   "syssentient",
			CPU:    float64(i),
			Memory: uint64(i) * 1024,
			State:  "S",
		}
	}

	fs := make([]models.Filesystem, 4)
	for i := range fs {
		fs[i] = models.Filesystem{
			Mountpoint:  fmt.Sprintf("/mnt/%d", i),
			Device:      fmt.Sprintf("/dev/sd%c1", 'a'+i),
			FSType:      "ext4",
			TotalBytes:  500 << 30,
			UsedBytes:   250 << 30,
			FreeBytes:   250 << 30,
			UsedPercent: 50,
		}
	}

	return &models.SystemState{
		HostID:      "bench-host",
		Hostname:    "bench",
		Timestamp:   time.Now().UTC(),
		CPUUsage:    42.5,
		CPUPerCore:  perCore,
		MemoryUsed:  8 << 30,
		MemoryTotal: 16 << 30,
		Processes:   procs,
		Filesystems: fs,
	}
}

// BenchmarkSave measures the write path the collector hits every poll.
//
// Recorded so the cgo -> pure-Go driver swap can be judged on numbers rather
// than on the claim that "it should be fine at a 2s interval".
func BenchmarkSave(b *testing.B) {
	store, err := NewStore(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })

	state := sampleState(8)

	b.ReportAllocs()
	for b.Loop() {
		state.Timestamp = time.Now().UTC()
		if err := store.Save(state); err != nil {
			b.Fatalf("save: %v", err)
		}
	}
}

// BenchmarkGetRecent measures the read path every dashboard poll hits.
func BenchmarkGetRecent(b *testing.B) {
	store, err := NewStore(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })

	state := sampleState(8)
	for range 500 {
		state.Timestamp = time.Now().UTC()
		if err := store.Save(state); err != nil {
			b.Fatalf("seed: %v", err)
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.GetRecent(100); err != nil {
			b.Fatalf("read: %v", err)
		}
	}
}
