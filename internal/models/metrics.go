package models

import "time"

// SystemState represents a snapshot of the system's performance metrics
type SystemState struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUUsage       float64   `json:"cpu_usage"`        // Total CPU utilization percentage
	MemoryUsed     uint64    `json:"memory_used"`      // Used memory in bytes
	MemoryTotal    uint64    `json:"memory_total"`     // Total memory in bytes
	DiskReadBytes  uint64    `json:"disk_read_bytes"`  // Cumulative read bytes (or rate if processed)
	DiskWriteBytes uint64    `json:"disk_write_bytes"` // Cumulative write bytes
	NetSentBytes   uint64    `json:"net_sent_bytes"`   // Cumulative sent bytes
	NetRecvBytes   uint64    `json:"net_recv_bytes"`   // Cumulative received bytes
	TopProcesses   string    `json:"top_processes"`    // Human readable summary of top processes
}
