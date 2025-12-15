package collector

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"sys-sentient/internal/models"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type Collector struct {
}

func NewCollector() *Collector {
	return &Collector{}
}

// Collect gathers current system metrics
func (c *Collector) Collect() (*models.SystemState, error) {
	now := time.Now()

	// 1. CPU Usage (Global)
	// Blocking call for 500ms to get a sample
	cpuPercent, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get cpu stats: %v", err)
	}
	totalCPU := 0.0
	if len(cpuPercent) > 0 {
		totalCPU = cpuPercent[0]
	}

	// 2. Memory
	vMem, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %v", err)
	}

	// 3. IO Counters (Disk)
	// false = aggregation of all partitions
	diskCounters, err := disk.IOCounters()
	if err != nil {
		return nil, fmt.Errorf("failed to get disk stats: %v", err)
	}
	var readBytes, writeBytes uint64
	for _, stat := range diskCounters {
		readBytes += stat.ReadBytes
		writeBytes += stat.WriteBytes
	}

	// 4. Net Counters
	// false = all interfaces aggregated
	netCounters, err := net.IOCounters(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get net stats: %v", err)
	}
	var sentBytes, recvBytes uint64
	if len(netCounters) > 0 {
		sentBytes = netCounters[0].BytesSent
		recvBytes = netCounters[0].BytesRecv
	}

	// 5. Top Processes
	topProcs, err := getTopProcesses(3) // Get top 3
	if err != nil {
		// Non-fatal, just log or ignore
		topProcs = "Error collecting processes"
	}

	state := &models.SystemState{
		Timestamp:      now,
		CPUUsage:       totalCPU,
		MemoryUsed:     vMem.Used,
		MemoryTotal:    vMem.Total,
		DiskReadBytes:  readBytes,
		DiskWriteBytes: writeBytes,
		NetSentBytes:   sentBytes,
		NetRecvBytes:   recvBytes,
		TopProcesses:   topProcs,
	}

	return state, nil
}

type procStat struct {
	Name string
	CPU  float64
}

func getTopProcesses(limit int) (string, error) {
	procs, err := process.Processes()
	if err != nil {
		return "", err
	}

	var stats []procStat
	for _, p := range procs {
		// CPUPercent(0) returns 0 on first call usually, but we try
		// For accurate per-process CPU, we need to map previous times.
		// For this 'skeleton' version, we might accept that it's instantaneous or 0 initially.
		// A better approach for daemon: Maintain a cache of process times.
		// But for now, let's just try getting the value.
		c, _ := p.CPUPercent()
		if c > 0 {
			n, _ := p.Name()
			stats = append(stats, procStat{Name: n, CPU: c})
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].CPU > stats[j].CPU
	})

	var result []string
	count := 0
	for _, s := range stats {
		if count >= limit {
			break
		}
		result = append(result, fmt.Sprintf("%s (%.1f%%)", s.Name, s.CPU))
		count++
	}

	if len(result) == 0 {
		return "None", nil
	}
	return strings.Join(result, ", "), nil
}
