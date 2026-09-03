package storage

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func sampleWithPayload() *models.SystemState {
	return &models.SystemState{
		HostID: "h1", Hostname: "web-01", Timestamp: time.Now(), CPUUsage: 42,
		Processes: []models.Process{
			{PID: 1, Name: "systemd", User: "root", CPU: 0.5, Memory: 12, State: "S"},
			{PID: 2, Name: "postgres", User: "postgres", CPU: 12.5, Memory: 512, State: "R"},
		},
		Filesystems: []models.Filesystem{
			{
				Mountpoint: "/", Device: "/dev/dm-0", FSType: "btrfs",
				TotalBytes: 1000, UsedBytes: 250, FreeBytes: 750, UsedPercent: 25,
				InodesUsedPercent: 3,
			},
		},
	}
}

func TestFilesystemDerivedFieldsSurviveRoundTrip(t *testing.T) {
	s := newAgentStore(t)
	if err := s.Save(sampleWithPayload()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent: %v", err)
	}
	if len(got) != 1 || len(got[0].Filesystems) != 1 {
		t.Fatalf("got %d samples", len(got))
	}

	fs := got[0].Filesystems[0]
	// Recomputed on read rather than stored, but the API shape must not change.
	if fs.FreeBytes != 750 {
		t.Errorf("FreeBytes = %d, want 750", fs.FreeBytes)
	}
	if fs.UsedPercent != 25 {
		t.Errorf("UsedPercent = %v, want 25", fs.UsedPercent)
	}
	if fs.Mountpoint != "/" || fs.Device != "/dev/dm-0" || fs.FSType != "btrfs" {
		t.Errorf("identity fields lost: %+v", fs)
	}
	if fs.InodesUsedPercent != 3 {
		t.Errorf("InodesUsedPercent = %v, want 3", fs.InodesUsedPercent)
	}
}

func TestTopProcessesIsDerivedNotStored(t *testing.T) {
	s := newAgentStore(t)
	if err := s.Save(sampleWithPayload()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Nothing is written to the column any more...
	var stored string
	if err := s.db.QueryRow(`SELECT COALESCE(top_processes, '') FROM metrics`).Scan(&stored); err != nil {
		t.Fatalf("read column: %v", err)
	}
	if stored != "" {
		t.Errorf("top_processes still written: %q", stored)
	}

	// ...but every consumer still sees it populated. The AI prompt reads this
	// back out of storage, so an empty value would silently degrade analysis.
	got, err := s.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent: %v", err)
	}
	if !strings.Contains(got[0].TopProcesses, "postgres") {
		t.Errorf("TopProcesses = %q, want it to name the processes", got[0].TopProcesses)
	}
}

func TestReadsRowsWrittenBeforeCompaction(t *testing.T) {
	s := newAgentStore(t)
	if err := s.Save(sampleWithPayload()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Rewrite the row in the old, fully-expanded shape, including a free_bytes
	// that is deliberately not total-minus-used — reserved blocks make that a
	// real case, and a stored value must win over the derivation.
	legacy := `[{"mountpoint":"/","device":"/dev/dm-0","fstype":"btrfs",` +
		`"total_bytes":1000,"used_bytes":250,"free_bytes":700,` +
		`"used_percent":25,"inodes_used_percent":3}]`
	if _, err := s.db.Exec(`UPDATE metrics SET filesystems = ?, top_processes = ?`,
		legacy, "old-summary (1.0%, 1MB)"); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	got, err := s.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent: %v", err)
	}
	if fs := got[0].Filesystems[0]; fs.FreeBytes != 700 {
		t.Errorf("FreeBytes = %d, want the stored 700, not a recomputed 750", fs.FreeBytes)
	}
	// A row that has a real stored summary must keep it.
	if got[0].TopProcesses != "old-summary (1.0%, 1MB)" {
		t.Errorf("TopProcesses = %q, want the stored value preserved", got[0].TopProcesses)
	}
}

func TestFilesystemCompactionIsLossless(t *testing.T) {
	// Real values from this machine. On btrfs and ext4 the reported free space
	// is NOT total minus used — reserved blocks — so a compaction that assumed
	// the derivation would silently corrupt both fields. Only vfat, which
	// reserves nothing, matches.
	cases := []models.Filesystem{
		{Mountpoint: "/", FSType: "btrfs", TotalBytes: 497313382400, UsedBytes: 253232746496,
			FreeBytes: 242961608704, UsedPercent: 51.034991},
		{Mountpoint: "/boot", FSType: "ext4", TotalBytes: 2043994112, UsedBytes: 559714304,
			FreeBytes: 1360130048, UsedPercent: 29.020281},
		{Mountpoint: "/boot/efi", FSType: "vfat", TotalBytes: 627900416, UsedBytes: 21004288,
			FreeBytes: 606896128, UsedPercent: 3.345162},
		// A sample carrying a percentage but no byte count, which the naive
		// derivation would report as 0%.
		{Mountpoint: "/data", FSType: "xfs", TotalBytes: 100, FreeBytes: 40, UsedPercent: 60},
	}

	encoded, err := marshalFilesystems(cases)
	if err != nil {
		t.Fatalf("marshalFilesystems: %v", err)
	}
	got := unmarshalFilesystems(encoded)

	if len(got) != len(cases) {
		t.Fatalf("got %d entries, want %d", len(got), len(cases))
	}
	for i, want := range cases {
		if got[i].FreeBytes != want.FreeBytes {
			t.Errorf("%s: FreeBytes = %d, want %d", want.Mountpoint, got[i].FreeBytes, want.FreeBytes)
		}
		if math.Abs(got[i].UsedPercent-want.UsedPercent) > percentTolerance {
			t.Errorf("%s: UsedPercent = %v, want %v", want.Mountpoint, got[i].UsedPercent, want.UsedPercent)
		}
		if got[i].TotalBytes != want.TotalBytes || got[i].UsedBytes != want.UsedBytes {
			t.Errorf("%s: byte counts changed: %+v", want.Mountpoint, got[i])
		}
	}
}

func TestFilesystemCompactionSavesWhereDerivationHolds(t *testing.T) {
	// A filesystem with no reserved blocks: both derived fields are redundant
	// and are dropped. This is the only case where the encoding saves anything,
	// which is why the overall footprint win comes from top_processes instead.
	fs := []models.Filesystem{{
		Mountpoint: "/boot/efi", Device: "/dev/nvme0n1p1", FSType: "vfat",
		TotalBytes: 627900416, UsedBytes: 21004288,
		FreeBytes: 606896128, UsedPercent: 3.3451619,
	}}

	compact, err := marshalFilesystems(fs)
	if err != nil {
		t.Fatalf("marshalFilesystems: %v", err)
	}
	full, err := json.Marshal(fs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if len(compact) >= len(full) {
		t.Fatalf("compact is %d bytes, full is %d — nothing was dropped", len(compact), len(full))
	}
	t.Logf("vfat entry: %d -> %d bytes", len(full), len(compact))
}

func TestUnmarshalFilesystemsToleratesCorruptValue(t *testing.T) {
	// One damaged column must not make the whole sample unreadable.
	if got := unmarshalFilesystems([]byte("{not json")); len(got) != 0 {
		t.Errorf("got %d entries from a corrupt value, want 0", len(got))
	}
	if got := unmarshalFilesystems(nil); got == nil {
		t.Error("nil input returned a nil slice; callers expect an empty one")
	}
}
