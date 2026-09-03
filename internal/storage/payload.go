package storage

import (
	"encoding/json"
	"math"

	"sys-sentient/internal/models"
)

// storedFilesystem is the on-disk shape of a filesystem entry.
//
// FreeBytes and UsedPercent are omitted: both are exact functions of
// TotalBytes and UsedBytes, and writing them on every sample stored the same
// information three times. They are recomputed on read, so the API shape is
// unchanged.
type storedFilesystem struct {
	Mountpoint        string  `json:"mountpoint"`
	Device            string  `json:"device"`
	FSType            string  `json:"fstype"`
	TotalBytes        uint64  `json:"total_bytes"`
	UsedBytes         uint64  `json:"used_bytes"`
	InodesUsedPercent float64 `json:"inodes_used_percent"`

	// FreeBytes and UsedPercent are read-only compatibility fields: rows
	// written before this change still carry them, and they must survive a
	// round trip rather than being silently zeroed.
	FreeBytes   *uint64  `json:"free_bytes,omitempty"`
	UsedPercent *float64 `json:"used_percent,omitempty"`
}

// percentTolerance is how far a stored percentage may sit from the derived one
// and still be treated as the same value. Floating point rounding through JSON
// makes an exact comparison meaningless.
const percentTolerance = 0.01

// marshalFilesystems encodes the compact on-disk form.
//
// A derived field is omitted only when it actually agrees with the derivation.
// If a collector reports a free-space figure that is not total minus used —
// reserved blocks make that normal on ext4, and a sample may carry a
// percentage without a byte count at all — the reported value is kept. That
// makes the compaction lossless by construction rather than by assumption.
func marshalFilesystems(filesystems []models.Filesystem) ([]byte, error) {
	compact := make([]storedFilesystem, 0, len(filesystems))
	for _, fs := range filesystems {
		entry := storedFilesystem{
			Mountpoint:        fs.Mountpoint,
			Device:            fs.Device,
			FSType:            fs.FSType,
			TotalBytes:        fs.TotalBytes,
			UsedBytes:         fs.UsedBytes,
			InodesUsedPercent: fs.InodesUsedPercent,
		}
		if fs.FreeBytes != derivedFree(fs.TotalBytes, fs.UsedBytes) {
			free := fs.FreeBytes
			entry.FreeBytes = &free
		}
		if math.Abs(fs.UsedPercent-derivedPercent(fs.TotalBytes, fs.UsedBytes)) > percentTolerance {
			percent := fs.UsedPercent
			entry.UsedPercent = &percent
		}
		compact = append(compact, entry)
	}
	return json.Marshal(compact)
}

func derivedFree(total, used uint64) uint64 {
	if total < used {
		return 0
	}
	return total - used
}

func derivedPercent(total, used uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

// unmarshalFilesystems decodes a stored value and restores the derived fields.
//
// A corrupt or absent value yields an empty slice rather than an error: one bad
// JSON column must not make an otherwise good sample unreadable.
func unmarshalFilesystems(raw []byte) []models.Filesystem {
	if len(raw) == 0 {
		return []models.Filesystem{}
	}
	var stored []storedFilesystem
	if err := json.Unmarshal(raw, &stored); err != nil {
		return []models.Filesystem{}
	}

	out := make([]models.Filesystem, 0, len(stored))
	for _, fs := range stored {
		out = append(out, models.Filesystem{
			Mountpoint:        fs.Mountpoint,
			Device:            fs.Device,
			FSType:            fs.FSType,
			TotalBytes:        fs.TotalBytes,
			UsedBytes:         fs.UsedBytes,
			InodesUsedPercent: fs.InodesUsedPercent,
			FreeBytes:         freeBytes(fs),
			UsedPercent:       usedPercent(fs),
		})
	}
	return out
}

// freeBytes prefers a stored value, so a row written before this change keeps
// whatever the collector reported — free space is not always total minus used
// once reserved blocks are involved.
func freeBytes(fs storedFilesystem) uint64 {
	if fs.FreeBytes != nil {
		return *fs.FreeBytes
	}
	return derivedFree(fs.TotalBytes, fs.UsedBytes)
}

func usedPercent(fs storedFilesystem) float64 {
	if fs.UsedPercent != nil {
		return *fs.UsedPercent
	}
	return derivedPercent(fs.TotalBytes, fs.UsedBytes)
}

// restoreTopProcesses fills the human-readable summary from the structured
// list. Storage stops writing the summary, because it is a pure function of
// Processes and was 11% of every row; consumers still expect it populated.
func restoreTopProcesses(m *models.SystemState) {
	if m.TopProcesses == "" && len(m.Processes) > 0 {
		m.TopProcesses = models.FormatTopProcesses(m.Processes)
	}
}
