package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func batchOf(n int) []*models.SystemState {
	out := make([]*models.SystemState, n)
	for i := range out {
		out[i] = &models.SystemState{
			HostID: "h", Hostname: "alpha",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second), MemoryTotal: 1,
		}
	}
	return out
}

// An agent flushes 60 samples at a time. Each used to be its own autocommit
// transaction, so the flush cost 60 fsyncs contending for one write lock.
func BenchmarkSaveBatch(b *testing.B) {
	s, _ := NewStore(filepath.Join(b.TempDir(), "b.db"))
	b.Cleanup(func() { _ = s.Close() })
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.SaveBatch(batchOf(60)); err != nil {
			b.Fatal(err)
		}
	}
}

// The old shape, for comparison.
func BenchmarkSaveIndividually(b *testing.B) {
	s, _ := NewStore(filepath.Join(b.TempDir(), "i.db"))
	b.Cleanup(func() { _ = s.Close() })
	b.ReportAllocs()
	for b.Loop() {
		for _, m := range batchOf(60) {
			if err := s.Save(m); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestSaveBatchStoresEverySample(t *testing.T) {
	t.Parallel()
	s, err := NewStore(filepath.Join(t.TempDir(), "sb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	stored, err := s.SaveBatch(batchOf(25))
	if err != nil {
		t.Fatalf("SaveBatch: %v", err)
	}
	if stored != 25 {
		t.Fatalf("stored %d, want 25", stored)
	}

	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM metrics`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 25 {
		t.Errorf("%d rows in the table, want 25", count)
	}
}

// A batch that is entirely empty must not open a transaction at all.
func TestSaveBatchHandlesEmpty(t *testing.T) {
	t.Parallel()
	s, err := NewStore(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	stored, err := s.SaveBatch(nil)
	if err != nil || stored != 0 {
		t.Fatalf("SaveBatch(nil) = %d, %v; want 0, nil", stored, err)
	}
}

var _ = fmt.Sprintf
