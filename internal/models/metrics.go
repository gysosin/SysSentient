package models

import (
	"fmt"
	"strings"
	"time"
)

// SystemState represents a snapshot of the system's performance metrics
type SystemState struct {
	// HostID is a stable identifier for the machine that produced this
	// sample, derived from the systemd machine-id. Hostnames collide and
	// change; the ID lets a renamed host keep its history.
	HostID      string    `json:"host_id"`
	Hostname    string    `json:"hostname"`
	Timestamp   time.Time `json:"timestamp"`
	CPUUsage    float64   `json:"cpu_usage"`    // Total CPU utilization percentage
	CPUPerCore  []float64 `json:"cpu_per_core"` // Per-core CPU utilization
	MemoryUsed  uint64    `json:"memory_used"`  // Used memory in bytes
	MemoryTotal uint64    `json:"memory_total"` // Total memory in bytes
	SwapUsed    uint64    `json:"swap_used"`    // Used swap in bytes
	SwapTotal   uint64    `json:"swap_total"`   // Total swap in bytes
	// MemoryCached and MemoryBuffers are what the "used" figure hides. Page
	// cache is reclaimable on demand, so a host reporting 90% memory used with
	// most of it cached is healthy, while the same number with none cached is
	// about to start swapping. Reporting only the total invites false alarms.
	MemoryCached   uint64  `json:"memory_cached"`    // Reclaimable page cache
	MemoryBuffers  uint64  `json:"memory_buffers"`   // Block-layer buffers
	DiskReadBytes  uint64  `json:"disk_read_bytes"`  // Cumulative read bytes (or rate if processed)
	DiskWriteBytes uint64  `json:"disk_write_bytes"` // Cumulative write bytes
	DiskIOPS       float64 `json:"disk_iops"`        // Disk operations per second
	NetSentBytes   uint64  `json:"net_sent_bytes"`   // Cumulative sent bytes
	NetRecvBytes   uint64  `json:"net_recv_bytes"`   // Cumulative received bytes
	LoadAvg1       float64 `json:"load_avg_1"`       // 1-minute load average
	LoadAvg5       float64 `json:"load_avg_5"`       // 5-minute load average
	LoadAvg15      float64 `json:"load_avg_15"`      // 15-minute load average
	Temperature    float64 `json:"temperature"`      // System temperature in Celsius
	UptimeSeconds  uint64  `json:"uptime_seconds"`   // Host uptime; the dashboard previously
	// fabricated this from a client-side page-load counter that reset on refresh.
	TopProcesses string    `json:"top_processes"` // Human readable summary of top processes
	Processes    []Process `json:"processes"`     // Structured top processes
	// Filesystems reports capacity per mounted filesystem. Without this the
	// product cannot detect a full disk — the most common outage cause.
	Filesystems []Filesystem `json:"filesystems"`
}

// Filesystem is capacity for one mounted filesystem.
type Filesystem struct {
	Mountpoint        string  `json:"mountpoint"`
	Device            string  `json:"device"`
	FSType            string  `json:"fstype"`
	TotalBytes        uint64  `json:"total_bytes"`
	UsedBytes         uint64  `json:"used_bytes"`
	FreeBytes         uint64  `json:"free_bytes"`
	UsedPercent       float64 `json:"used_percent"`
	InodesUsedPercent float64 `json:"inodes_used_percent"`
}

type Process struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	User   string  `json:"user"`
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"` // Resident memory in MB
	State  string  `json:"state"`
}

type AIAction struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	Description string `json:"description"`
	IsSafe      bool   `json:"isSafe"`
}

type AIAnalysis struct {
	Status string `json:"status"` // Healthy, Warning, Critical
	// Free-text fields are FlexText because a model asked for bullet points
	// returns an array of them, and losing a good answer to a type mismatch
	// helps nobody. See FlexText.
	Summary            FlexText   `json:"summary"`
	DetailedAnalysis   FlexText   `json:"detailedAnalysis"`
	RecommendedActions []AIAction `json:"recommendedActions"`
}

// FormatTopProcesses renders the process list as the human-readable summary
// used in AI prompts and by older dashboard builds.
//
// It lives here, rather than in the collector, because the storage layer
// derives this string on read instead of storing it: the text is a pure
// function of Processes, and persisting both wrote the same information twice
// on every sample for 11% of each row.
func FormatTopProcesses(processes []Process) string {
	if len(processes) == 0 {
		return "None"
	}

	parts := make([]string, 0, len(processes))
	for _, p := range processes {
		parts = append(parts, fmt.Sprintf("%s (%.1f%%, %dMB)", p.Name, p.CPU, p.Memory))
	}
	return strings.Join(parts, ", ")
}
