package collector

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"sys-sentient/internal/hostid"
	"sys-sentient/internal/models"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// defaultTopProcesses is used when the configured value is missing or invalid.
const defaultTopProcesses = 10

// maxFilesystems bounds the filesystem list. The payload ships on every
// WebSocket frame, and a container host can mount hundreds of overlay and bind
// mounts that carry no operational signal.
const maxFilesystems = 24

// pseudoFSTypes are kernel/virtual filesystems with no meaningful capacity.
// Reporting them buries the real disks under dozens of 0-byte rows.
var pseudoFSTypes = map[string]bool{
	"autofs": true, "bpf": true, "cgroup": true, "cgroup2": true,
	"configfs": true, "debugfs": true, "devpts": true, "devtmpfs": true,
	"efivarfs": true, "fuse.gvfsd-fuse": true, "fuse.portal": true,
	"fusectl": true, "hugetlbfs": true, "mqueue": true, "nsfs": true,
	"overlay": true, "proc": true, "pstore": true, "ramfs": true,
	"securityfs": true, "selinuxfs": true, "squashfs": true, "sysfs": true,
	"tracefs": true,
}

// procCPUSample is the previous CPU-time reading for one process, used to
// derive current utilisation from the delta between polls.
type procCPUSample struct {
	totalSeconds float64
	at           time.Time
}

type Collector struct {
	topProcesses    int
	hostIDOverride  string
	procCache       map[int32]*process.Process
	lastProcCPU     map[int32]procCPUSample
	lastCollectTime time.Time
	lastReadOps     uint64
	lastWriteOps    uint64
}

// NewCollector returns a Collector that records topProcesses processes per
// snapshot. Values below 1 fall back to defaultTopProcesses so a zero-value
// config can never silently disable the process list.
func NewCollector(topProcesses int) *Collector {
	return NewCollectorWithHostID(topProcesses, "")
}

// NewCollectorWithHostID allows overriding the derived machine identifier.
// An empty override falls back to the machine-id derivation.
func NewCollectorWithHostID(topProcesses int, hostIDOverride string) *Collector {
	if topProcesses < 1 {
		topProcesses = defaultTopProcesses
	}
	return &Collector{
		topProcesses:   topProcesses,
		hostIDOverride: strings.TrimSpace(hostIDOverride),
		procCache:      make(map[int32]*process.Process),
		lastProcCPU:    make(map[int32]procCPUSample),
	}
}

// Collect gathers current system metrics
func (c *Collector) Collect() (*models.SystemState, error) {
	now := time.Now()

	// 1. CPU Usage (Global + Per-Core)
	// Blocking call for 500ms to get a sample
	cpuPercent, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get cpu stats: %w", err)
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
		return nil, fmt.Errorf("failed to get memory stats: %w", err)
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
		return nil, fmt.Errorf("failed to get disk stats: %w", err)
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
			totalOps := counterDelta(readOps, c.lastReadOps) + counterDelta(writeOps, c.lastWriteOps)
			diskIOPS = float64(totalOps) / dt
		}
	}
	c.lastReadOps = readOps
	c.lastWriteOps = writeOps
	c.lastCollectTime = now

	// 4. Net Counters
	netCounters, err := net.IOCounters(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get net stats: %w", err)
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

	// 7. Host identity + uptime — cheap (single /proc read) and lets the dashboard show a
	// real number instead of a counter that resets on page refresh.
	uptimeSeconds, err := host.Uptime()
	if err != nil {
		uptimeSeconds = 0
	}

	hostID, hostname := hostid.Resolve()
	if c.hostIDOverride != "" {
		hostID = c.hostIDOverride
	}

	// 8. Top Processes
	processes, err := c.getTopProcesses(c.topProcesses, now)
	topProcs := formatTopProcesses(processes)
	if err != nil {
		topProcs = "Error collecting processes"
	}

	state := &models.SystemState{
		HostID:         hostID,
		Hostname:       hostname,
		Filesystems:    collectFilesystems(),
		Timestamp:      now,
		CPUUsage:       totalCPU,
		CPUPerCore:     cpuPerCore,
		MemoryUsed:     vMem.Used,
		MemoryCached:   vMem.Cached,
		MemoryBuffers:  vMem.Buffers,
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
		UptimeSeconds:  uptimeSeconds,
		TopProcesses:   topProcs,
		Processes:      processes,
	}

	return state, nil
}

func counterDelta(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func (c *Collector) getTopProcesses(limit int, now time.Time) ([]models.Process, error) {
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

		// Current CPU, derived from the delta since the previous poll.
		// p.CPUPercent() is a lifetime average and mis-ranks processes badly.
		var totalSeconds float64
		if times, err := p.Times(); err == nil && times != nil {
			totalSeconds = times.User + times.System + times.Nice + times.Iowait +
				times.Irq + times.Softirq + times.Steal
		}
		cpu := c.processCPUPercent(pid, totalSeconds, now)

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
				Name:   compactProcessName(name),
				User:   username,
				CPU:    cpu,
				Memory: memoryMB,
				State:  normalizeProcessState(status),
			})
		}
	}

	// Replace cache with new one (implicit pruning of dead procs)
	c.procCache = newCache

	// Drop CPU baselines for processes that have exited, otherwise the map
	// grows without bound on a busy host and a recycled PID inherits a stale
	// baseline.
	for pid := range c.lastProcCPU {
		if _, alive := newCache[pid]; !alive {
			delete(c.lastProcCPU, pid)
		}
	}

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
		result = append(result, fmt.Sprintf("%s (%.1f%%, %dMB)", p.Name, p.CPU, p.Memory))
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

func compactProcessName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	name = strings.ReplaceAll(name, ",", " ")
	if len(name) <= 80 {
		return name
	}
	return name[:77] + "..."
}

// collectFilesystems reports capacity for each real mounted filesystem.
//
// disk.Usage() was never called anywhere in the original collector, so the
// product had no filesystem capacity metric at all — only cumulative IO byte
// counters. Pseudo filesystems are skipped and duplicate devices collapsed so
// the list stays operationally useful.
func collectFilesystems() []models.Filesystem {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil
	}

	filesystems := make([]models.Filesystem, 0, len(partitions))
	seenMount := make(map[string]bool, len(partitions))
	seenDevice := make(map[string]bool, len(partitions))

	for _, partition := range partitions {
		if len(filesystems) >= maxFilesystems {
			break
		}
		if pseudoFSTypes[strings.ToLower(partition.Fstype)] {
			continue
		}
		if seenMount[partition.Mountpoint] {
			continue
		}
		// Bind mounts expose the same device at several paths; one row each is
		// noise, and they all report identical capacity.
		if partition.Device != "" && seenDevice[partition.Device] {
			continue
		}

		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil || usage == nil || usage.Total == 0 {
			continue
		}

		seenMount[partition.Mountpoint] = true
		if partition.Device != "" {
			seenDevice[partition.Device] = true
		}

		filesystems = append(filesystems, models.Filesystem{
			Mountpoint:        partition.Mountpoint,
			Device:            partition.Device,
			FSType:            partition.Fstype,
			TotalBytes:        usage.Total,
			UsedBytes:         usage.Used,
			FreeBytes:         usage.Free,
			UsedPercent:       usage.UsedPercent,
			InodesUsedPercent: usage.InodesUsedPercent,
		})
	}

	// Fullest first: that is the row an operator needs to see.
	sort.Slice(filesystems, func(i, j int) bool {
		return filesystems[i].UsedPercent > filesystems[j].UsedPercent
	})

	return filesystems
}

// processCPUPercent returns a process's CPU utilisation since the previous
// poll, as a percentage of one core (matching what top reports).
//
// gopsutil's p.CPUPercent() divides total CPU time by process age, i.e. a
// lifetime average. On a long-lived process that is almost never the current
// value: a browser that pegged a core an hour ago outranks a compiler spiking
// right now. Both the process table and the AI prompt consumed that number.
//
// The first observation of a PID has no baseline and reports 0 rather than
// falling back to the lifetime average.
func (c *Collector) processCPUPercent(pid int32, totalSeconds float64, now time.Time) float64 {
	previous, seen := c.lastProcCPU[pid]
	c.lastProcCPU[pid] = procCPUSample{totalSeconds: totalSeconds, at: now}

	if !seen {
		return 0
	}

	elapsed := now.Sub(previous.at).Seconds()
	if elapsed <= 0 {
		return 0
	}

	delta := totalSeconds - previous.totalSeconds
	if delta <= 0 {
		// Counter reset (PID reuse) or no CPU consumed.
		return 0
	}

	return (delta / elapsed) * 100
}
