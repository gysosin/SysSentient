package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

// The case this exists for: a backup taken while the daemon is writing.
// Copying the file with cp here produces a corrupt result, because the
// database and its write-ahead log are two files that must agree.
func TestBackupIsConsistentUnderConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "live.db")

	s, err := NewStore(src)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = s.Save(&models.SystemState{
				HostID: "h", Hostname: "alpha",
				Timestamp: time.Now(), CPUUsage: float64(i % 100), MemoryTotal: 1,
			})
		}
	}()

	// Let some writes land, then back up mid-flight.
	time.Sleep(80 * time.Millisecond)
	dest := filepath.Join(dir, "backup.db")
	if err := s.Backup(dest); err != nil {
		close(stop)
		wg.Wait()
		t.Fatalf("backup: %v", err)
	}
	close(stop)
	wg.Wait()

	// A backup needs no -wal or -shm beside it; VACUUM INTO writes a fully
	// checkpointed file. If those exist, the copy is not self-contained.
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dest + suffix); err == nil {
			t.Errorf("backup left a %s file; the copy is not self-contained", suffix)
		}
	}

	restored, err := NewStore(dest)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	defer func() { _ = restored.Close() }()

	result, err := restored.IntegrityCheck()
	if err != nil {
		t.Fatalf("integrity check: %v", err)
	}
	if result != "ok" {
		t.Fatalf("restored database is corrupt: %s", result)
	}

	rows, err := restored.GetRecent(10)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if len(rows) == 0 {
		t.Error("restored database is readable but empty")
	}
}

// The database holds password hashes and session tokens, so a backup must not
// land world-readable regardless of the umask.
func TestBackupIsNotWorldReadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	dest := filepath.Join(dir, "out.db")
	if err := s.Backup(dest); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("backup permissions are %04o; group and other must have no access", perm)
	}
}

// Overwriting silently would destroy a previous backup, so refuse.
func TestBackupRefusesToOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	dest := filepath.Join(dir, "existing.db")
	if err := os.WriteFile(dest, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(dest); err == nil {
		t.Fatal("expected Backup to refuse an existing destination")
	}
	// And it must not have touched it.
	data, _ := os.ReadFile(dest)
	if string(data) != "precious" {
		t.Error("Backup modified the existing file it refused to overwrite")
	}
}
