package server

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"

	"sys-sentient/internal/alerting"
	"sys-sentient/internal/models"
	"sys-sentient/internal/version"
)

// handlePrometheusMetrics exposes the latest sample in Prometheus text format.
//
// A monitoring product that cannot itself be scraped is a hard sell: operators
// already run Prometheus and want this host's numbers in the same place as
// everything else. Written by hand rather than pulling in the client library —
// the metric set is small and fixed, and the exposition format is stable.
func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	build := version.Get()
	writeMetric(&b, "sys_sentient_build_info", "Build identity of the running daemon.", "gauge",
		[]sample{{labels: map[string]string{
			"version": build.Version, "commit": build.Commit, "go_version": build.GoVersion,
		}, value: 1}})

	// Daemon self-metrics: previously there was no visibility at all into the
	// process doing the monitoring.
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeMetric(&b, "sys_sentient_goroutines", "Number of goroutines.", "gauge",
		[]sample{{value: float64(runtime.NumGoroutine())}})
	writeMetric(&b, "sys_sentient_heap_alloc_bytes", "Bytes of allocated heap objects.", "gauge",
		[]sample{{value: float64(mem.HeapAlloc)}})
	writeMetric(&b, "sys_sentient_gc_cycles_total", "Completed GC cycles.", "counter",
		[]sample{{value: float64(mem.NumGC)}})

	if s.Hub != nil {
		writeMetric(&b, "sys_sentient_websocket_clients", "Connected dashboard clients.", "gauge",
			[]sample{{value: float64(s.Hub.ClientCount())}})
	}

	if s.evaluator != nil {
		firing, pending := 0, 0
		for _, alert := range s.evaluator.Active() {
			if alert.State == alerting.StateFiring {
				firing++
			} else {
				pending++
			}
		}
		writeMetric(&b, "sys_sentient_alerts_active", "Active alerts by state.", "gauge", []sample{
			{labels: map[string]string{"state": "firing"}, value: float64(firing)},
			{labels: map[string]string{"state": "pending"}, value: float64(pending)},
		})
	}

	if s.store != nil {
		if recent, err := s.store.GetRecent(1); err == nil && len(recent) > 0 {
			writeHostMetrics(&b, recent[0])
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}

func writeHostMetrics(b *strings.Builder, state models.SystemState) {
	host := map[string]string{"host": state.Hostname}

	writeMetric(b, "sys_sentient_cpu_usage_percent", "Total CPU utilisation.", "gauge",
		[]sample{{labels: host, value: state.CPUUsage}})
	writeMetric(b, "sys_sentient_memory_used_bytes", "Memory in use.", "gauge",
		[]sample{{labels: host, value: float64(state.MemoryUsed)}})
	writeMetric(b, "sys_sentient_memory_total_bytes", "Total memory.", "gauge",
		[]sample{{labels: host, value: float64(state.MemoryTotal)}})
	writeMetric(b, "sys_sentient_swap_used_bytes", "Swap in use.", "gauge",
		[]sample{{labels: host, value: float64(state.SwapUsed)}})
	writeMetric(b, "sys_sentient_swap_total_bytes", "Total swap.", "gauge",
		[]sample{{labels: host, value: float64(state.SwapTotal)}})
	writeMetric(b, "sys_sentient_load1", "1-minute load average.", "gauge",
		[]sample{{labels: host, value: state.LoadAvg1}})
	writeMetric(b, "sys_sentient_load5", "5-minute load average.", "gauge",
		[]sample{{labels: host, value: state.LoadAvg5}})
	writeMetric(b, "sys_sentient_load15", "15-minute load average.", "gauge",
		[]sample{{labels: host, value: state.LoadAvg15}})
	writeMetric(b, "sys_sentient_uptime_seconds", "Host uptime.", "gauge",
		[]sample{{labels: host, value: float64(state.UptimeSeconds)}})

	if state.Temperature > 0 {
		writeMetric(b, "sys_sentient_temperature_celsius", "Hottest reported sensor.", "gauge",
			[]sample{{labels: host, value: state.Temperature}})
	}

	if len(state.Filesystems) > 0 {
		used := make([]sample, 0, len(state.Filesystems))
		free := make([]sample, 0, len(state.Filesystems))
		for _, fs := range state.Filesystems {
			labels := map[string]string{"host": state.Hostname, "mountpoint": fs.Mountpoint, "fstype": fs.FSType}
			used = append(used, sample{labels: labels, value: fs.UsedPercent})
			free = append(free, sample{labels: labels, value: float64(fs.FreeBytes)})
		}
		writeMetric(b, "sys_sentient_filesystem_used_percent", "Filesystem utilisation.", "gauge", used)
		writeMetric(b, "sys_sentient_filesystem_free_bytes", "Filesystem free space.", "gauge", free)
	}
}

type sample struct {
	labels map[string]string
	value  float64
}

func writeMetric(b *strings.Builder, name, help, metricType string, samples []sample) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, metricType)
	for _, s := range samples {
		fmt.Fprintf(b, "%s%s %g\n", name, formatLabels(s.labels), s.value)
	}
}

// formatLabels renders label pairs in a stable order so scrape output does not
// churn between requests.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k, v := range labels {
		if v == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, escapeLabelValue(labels[k])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// escapeLabelValue escapes per the Prometheus exposition format. Mountpoints
// and hostnames are not guaranteed to be free of backslashes or quotes.
func escapeLabelValue(v string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return replacer.Replace(v)
}
