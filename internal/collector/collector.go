package collector

import (
	"cmp"
	"fmt"
	"math"
	"slices"
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
	// statReader keeps a scratch buffer alive across polls.
	statReader      *statReader
	topProcesses    int
	hostIDOverride  string
	procCache       map[int32]*process.Process
	lastProcCPU     map[int32]procCPUSample
	lastCollectTime time.Time
	lastReadOps     uint64
	lastWriteOps    uint64
	// cores caches the logical CPU count, used to convert per-core process
	// CPU into whole-machine percent.
	cores int
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
		statReader:     newStatReader(),
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
	// interval 0 measures against the previous call rather than sleeping.
	//
	// The blocking form held the daemon's main select loop for 500ms of every
	// two-second tick — a quarter of the cycle — and Go tickers drop ticks
	// rather than queue them, so any overrun silently lost samples. Polling
	// every two seconds already provides the interval to diff against, so the
	// sleep bought nothing.
	cpuPercent, err := cpu.Percent(0, false)
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
	processes, processCount, err := c.getTopProcesses(c.topProcesses, now)
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
		ProcessCount:   processCount,
	}

	return state, nil
}

func counterDelta(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

// getTopProcesses returns the `limit` busiest processes by current CPU.
//
// Two passes, deliberately. The first is cheap: one Times() read per PID to
// derive CPU from the delta since the last poll. Only the survivors of that
// ranking pay for Name, Username, MemoryInfo and Status.
//
// The previous version fetched all four for every process above a 0.1%
// threshold and then threw all but `limit` of them away in the sort — several
// hundred processes' worth of syscalls on a busy host to display ten rows.
// This runs on every tick for the life of the daemon, so it is the single
// biggest cost in the collector.
// getTopProcesses returns the busiest processes and how many are running.
//
// Ranked by CPU *and* by memory, and the two lists are merged. Ranking by CPU
// alone -- which this did, with a `cpu > 0.1` gate applied before memory was
// ever read -- meant an idle process holding 8 GB was dropped before anything
// looked at its memory, while the dashboard still offered a Memory column and
// a memory sort. Those ranked ten CPU-active processes by memory and called it
// the top memory consumers.
func (c *Collector) getTopProcesses(limit int, now time.Time) ([]models.Process, int, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, 0, err
	}

	type candidate struct {
		pid    int32
		proc   *process.Process
		cpu    float64
		memory uint64
	}

	candidates := make([]candidate, 0, len(pids))
	newCache := make(map[int32]*process.Process, len(pids))

	for _, pid := range pids {
		p, ok := c.procCache[pid]
		if !ok {
			p, err = process.NewProcess(pid)
			if err != nil {
				continue
			}
		}
		newCache[pid] = p

		// Current CPU, derived from the delta since the previous poll.
		// p.CPUPercent() is a lifetime average and mis-ranks processes badly:
		// something that pegged a core an hour ago outranks one busy now.
		// One read yields both: CPU time and resident memory live in the same
		// file, so memory for every process costs nothing extra.
		totalSeconds, memory, ok := c.statReader.procStat(pid)
		if !ok {
			continue
		}
		cpu := c.processCPUPercent(pid, totalSeconds, now)

		// Nothing is filtered out here. A process that is idle now still
		// belongs in the memory ranking, and one holding no memory still
		// belongs in the CPU ranking.
		candidates = append(candidates, candidate{pid: pid, proc: p, cpu: cpu, memory: memory})
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

	// Union of the two rankings, so both questions are answerable from the
	// same payload without storing every process.
	selected := make(map[int32]candidate, limit*2)

	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Compare(b.cpu, a.cpu)
	})
	for _, cand := range candidates[:min(limit, len(candidates))] {
		if cand.cpu > 0 {
			selected[cand.pid] = cand
		}
	}

	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Compare(b.memory, a.memory)
	})
	for _, cand := range candidates[:min(limit, len(candidates))] {
		if cand.memory > 0 {
			selected[cand.pid] = cand
		}
	}

	// Only now, for the handful that will actually be displayed, pay for the
	// metadata.
	cores := float64(c.coreCount())
	stats := make([]models.Process, 0, len(selected))
	for _, cand := range selected {
		name, err := cand.proc.Name()
		if err != nil || name == "" {
			// The process exited between the scan and here. Reporting it with
			// an empty name, no user and a default state rendered a dead
			// process as "Sleeping, 0 MB" beside a real CPU number.
			continue
		}
		username, _ := cand.proc.Username()
		status, _ := cand.proc.Status()

		machineCPU := cand.cpu
		if cores > 0 {
			machineCPU = cand.cpu / cores
		}

		stats = append(stats, models.Process{
			PID:  cand.pid,
			Name: compactProcessName(name),
			User: username,
			// Rounded: the raw value carried seventeen significant digits into
			// the database and the API for no one's benefit.
			CPU:         math.Round(machineCPU*100) / 100,
			CPUCore:     math.Round(cand.cpu*100) / 100,
			Memory:      cand.memory / (1024 * 1024),
			MemoryBytes: cand.memory,
			State:       normalizeProcessState(status),
		})
	}

	// Deterministic order for the transport: busiest first.
	slices.SortFunc(stats, func(a, b models.Process) int {
		if n := cmp.Compare(b.CPU, a.CPU); n != 0 {
			return n
		}
		return cmp.Compare(b.MemoryBytes, a.MemoryBytes)
	})

	return stats, len(pids), nil
}

// coreCount reports the number of logical CPUs, cached after the first call.
//
// Used to convert per-core process CPU into whole-machine percent so the
// number is comparable with the system gauge beside it.
func (c *Collector) coreCount() int {
	if c.cores == 0 {
		if n, err := cpu.Counts(true); err == nil && n > 0 {
			c.cores = n
		} else {
			c.cores = 1
		}
	}
	return c.cores
}

func formatTopProcesses(processes []models.Process) string {
	return models.FormatTopProcesses(processes)
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
	slices.SortFunc(filesystems, func(a, b models.Filesystem) int {
		switch {
		case a.UsedPercent > b.UsedPercent:
			return -1
		case a.UsedPercent < b.UsedPercent:
			return 1
		default:
			return 0
		}
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
