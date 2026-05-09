package models

import "time"

// SystemState represents a snapshot of the system's performance metrics
type SystemState struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUUsage       float64   `json:"cpu_usage"`        // Total CPU utilization percentage
	CPUPerCore     []float64 `json:"cpu_per_core"`     // Per-core CPU utilization
	MemoryUsed     uint64    `json:"memory_used"`      // Used memory in bytes
	MemoryTotal    uint64    `json:"memory_total"`     // Total memory in bytes
	SwapUsed       uint64    `json:"swap_used"`        // Used swap in bytes
	SwapTotal      uint64    `json:"swap_total"`       // Total swap in bytes
	DiskReadBytes  uint64    `json:"disk_read_bytes"`  // Cumulative read bytes (or rate if processed)
	DiskWriteBytes uint64    `json:"disk_write_bytes"` // Cumulative write bytes
	DiskIOPS       float64   `json:"disk_iops"`        // Disk operations per second
	NetSentBytes   uint64    `json:"net_sent_bytes"`   // Cumulative sent bytes
	NetRecvBytes   uint64    `json:"net_recv_bytes"`   // Cumulative received bytes
	LoadAvg1       float64   `json:"load_avg_1"`       // 1-minute load average
	LoadAvg5       float64   `json:"load_avg_5"`       // 5-minute load average
	LoadAvg15      float64   `json:"load_avg_15"`      // 15-minute load average
	Temperature    float64   `json:"temperature"`      // System temperature in Celsius
	TopProcesses   string    `json:"top_processes"`    // Human readable summary of top processes
}

type AIAction struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	Description string `json:"description"`
	IsSafe      bool   `json:"isSafe"`
}

type AIAnalysis struct {
	Status             string     `json:"status"` // Healthy, Warning, Critical
	Summary            string     `json:"summary"`
	DetailedAnalysis   string     `json:"detailedAnalysis"`
	RecommendedActions []AIAction `json:"recommendedActions"`
}
