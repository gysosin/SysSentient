package collector

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"sys-sentient/internal/models"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type Collector struct {
	procCache       map[int32]*process.Process
	lastCollectTime time.Time
	lastReadOps     uint64
	lastWriteOps    uint64
}

func NewCollector() *Collector {
	return &Collector{
		procCache: make(map[int32]*process.Process),
	}
}

// Collect gathers current system metrics
func (c *Collector) Collect() (*models.SystemState, error) {
	now := time.Now()

	// 1. CPU Usage (Global + Per-Core)
	// Blocking call for 500ms to get a sample
	cpuPercent, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get cpu stats: %v", err)
	}
	totalCPU := 0.0
	if len(cpuPercent) > 0 {
		totalCPU = cpuPercent[0]
	}

	// Per-core CPU (non-blocking since we already waited above)
	cpuPerCore, _ := cpu.Percent(0, true)

	// 2. Memory
	vMem, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %v", err)
	}

	// 2b. Swap Memory
	swapMem, _ := mem.SwapMemory()
	var swapUsed, swapTotal uint64
	if swapMem != nil {
		swapUsed = swapMem.Used
		swapTotal = swapMem.Total
	}

	// 3. IO Counters (Disk)
	diskCounters, err := disk.IOCounters()
	if err != nil {
		return nil, fmt.Errorf("failed to get disk stats: %v", err)
	}
	var readBytes, writeBytes, readOps, writeOps uint64
	for _, stat := range diskCounters {
		readBytes += stat.ReadBytes
		writeBytes += stat.WriteBytes
		readOps += stat.ReadCount
		writeOps += stat.WriteCount
	}

	// Calculate IOPS
	var diskIOPS float64
	if !c.lastCollectTime.IsZero() {
		dt := now.Sub(c.lastCollectTime).Seconds()
		if dt > 0 {
			totalOps := (readOps - c.lastReadOps) + (writeOps - c.lastWriteOps)
			diskIOPS = float64(totalOps) / dt
		}
	}
	c.lastReadOps = readOps
	c.lastWriteOps = writeOps
	c.lastCollectTime = now

	// 4. Net Counters
	netCounters, err := net.IOCounters(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get net stats: %v", err)
	}
	var sentBytes, recvBytes uint64
	if len(netCounters) > 0 {
		sentBytes = netCounters[0].BytesSent
		recvBytes = netCounters[0].BytesRecv
	}

	// 5. Load Average
	loadAvg, _ := load.Avg()
	var loadAvg1, loadAvg5, loadAvg15 float64
	if loadAvg != nil {
		loadAvg1 = loadAvg.Load1
		loadAvg5 = loadAvg.Load5
		loadAvg15 = loadAvg.Load15
	}

	// 6. Temperature
	var temp float64
	temps, _ := host.SensorsTemperatures()
	if len(temps) > 0 {
		// Just take the first sensor for now
		temp = temps[0].Temperature
	}

	// 7. Top Processes
	processes, err := c.getTopProcesses(3)
	topProcs := formatTopProcesses(processes)
	if err != nil {
		topProcs = "Error collecting processes"
	}

	state := &models.SystemState{
		Timestamp:      now,
		CPUUsage:       totalCPU,
		CPUPerCore:     cpuPerCore,
		MemoryUsed:     vMem.Used,
		MemoryTotal:    vMem.Total,
		SwapUsed:       swapUsed,
		SwapTotal:      swapTotal,
		DiskReadBytes:  readBytes,
		DiskWriteBytes: writeBytes,
		DiskIOPS:       diskIOPS,
		NetSentBytes:   sentBytes,
		NetRecvBytes:   recvBytes,
		LoadAvg1:       loadAvg1,
		LoadAvg5:       loadAvg5,
		LoadAvg15:      loadAvg15,
		Temperature:    temp,
		TopProcesses:   topProcs,
		Processes:      processes,
	}

	return state, nil
}

func (c *Collector) getTopProcesses(limit int) ([]models.Process, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	var stats []models.Process
	newCache := make(map[int32]*process.Process)

	for _, pid := range pids {
		var p *process.Process
		if cached, ok := c.procCache[pid]; ok {
			p = cached
		} else {
			// New process
			p, err = process.NewProcess(pid)
			if err != nil {
				continue
			}
		}
		newCache[pid] = p

		// Get CPU
		cpu, _ := p.CPUPercent()
		if cpu > 0.1 { // Filter out very low usage
			name, _ := p.Name()
			username, _ := p.Username()
			memInfo, _ := p.MemoryInfo()
			status, _ := p.Status()
			memoryMB := uint64(0)
			if memInfo != nil {
				memoryMB = memInfo.RSS / 1024 / 1024 // Convert to MB
			}
			stats = append(stats, models.Process{
				PID:    pid,
				Name:   name,
				User:   username,
				CPU:    cpu,
				Memory: memoryMB,
				State:  normalizeProcessState(status),
			})
		}
	}

	// Replace cache with new one (implicit pruning of dead procs)
	c.procCache = newCache

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].CPU > stats[j].CPU
	})

	if len(stats) > limit {
		stats = stats[:limit]
	}
	return stats, nil
}

func formatTopProcesses(processes []models.Process) string {
	if len(processes) == 0 {
		return "None"
	}

	result := make([]string, 0, len(processes))
	for _, p := range processes {
		result = append(result, fmt.Sprintf("%s (%.1f%%, %dMB, %s)", p.Name, p.CPU, p.Memory, p.User))
	}
	return strings.Join(result, ", ")
}

func normalizeProcessState(status []string) string {
	for _, value := range status {
		switch strings.ToLower(value) {
		case "running", "run":
			return "Running"
		case "zombie":
			return "Zombie"
		case "stopped", "stop":
			return "Stopped"
		}
	}
	return "Sleeping"
}
