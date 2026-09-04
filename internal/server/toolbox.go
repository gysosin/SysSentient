package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
)

// storeToolbox answers the assistant's questions from the database.
//
// Every result is scrubbed before it is returned: these strings go to a model
// running on somebody else's hardware, and the privacy boundary that governs
// the one-shot analysis has to govern the assistant too.
type storeToolbox struct {
	srv *Server
}

func (s *Server) toolbox() *storeToolbox { return &storeToolbox{srv: s} }

func (t *storeToolbox) QueryMetrics(_ context.Context, hostID string, from, to time.Time) (string, error) {
	if t.srv.store == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	window := storage.Range{From: from, To: to}

	samples, err := t.srv.store.QueryRange(hostID, window, 500)
	if err != nil {
		return "", err
	}
	if len(samples) == 0 {
		// Fall back to the aggregated tiers, the same way the API does, so a
		// question about last month is not answered "no data" when a rollup
		// holds it.
		points, rerr := t.srv.store.GetRollupsRange(storage.RollupMinute, hostID, window, 500)
		if rerr != nil || len(points) == 0 {
			points, rerr = t.srv.store.GetRollupsRange(storage.RollupFiveMinute, hostID, window, 500)
		}
		if rerr != nil {
			return "", rerr
		}
		return summariseRollups(points, window), nil
	}
	return summariseSamples(t.srv.scrubber.SanitizeState(samples[0]).Hostname, samples, window), nil
}

// summariseSamples renders a window as prose plus the few numbers that matter.
func summariseSamples(hostname string, samples []models.SystemState, window storage.Range) string {
	var (
		cpuSum, memSum float64
		cpuMax, memMax float64
		peakAt         time.Time
	)
	for _, s := range samples {
		cpuSum += s.CPUUsage
		if s.CPUUsage > cpuMax {
			cpuMax, peakAt = s.CPUUsage, s.Timestamp
		}
		memPct := 0.0
		if s.MemoryTotal > 0 {
			memPct = float64(s.MemoryUsed) / float64(s.MemoryTotal) * 100
		}
		memSum += memPct
		if memPct > memMax {
			memMax = memPct
		}
	}
	n := float64(len(samples))

	var b strings.Builder
	fmt.Fprintf(&b, "Window %s to %s on %s, %d samples.\n",
		window.From.UTC().Format(time.RFC3339), window.To.UTC().Format(time.RFC3339),
		orUnknown(hostname), len(samples))
	fmt.Fprintf(&b, "CPU: avg %.1f%%, peak %.1f%% at %s.\n",
		cpuSum/n, cpuMax, peakAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "Memory: avg %.1f%%, peak %.1f%%.\n", memSum/n, memMax)

	last := samples[len(samples)-1]
	fmt.Fprintf(&b, "At the end of the window: load %.2f, %d processes running, %.0f MB used of %.0f MB.\n",
		last.LoadAvg1, last.ProcessCount,
		float64(last.MemoryUsed)/(1<<20), float64(last.MemoryTotal)/(1<<20))
	return b.String()
}

func summariseRollups(points []storage.RollupPoint, window storage.Range) string {
	if len(points) == 0 {
		return ""
	}
	var cpuSum, cpuMax float64
	var peak time.Time
	for _, p := range points {
		cpuSum += p.CPUAvg
		if p.CPUMax > cpuMax {
			cpuMax, peak = p.CPUMax, p.Bucket
		}
	}
	return fmt.Sprintf(
		"Window %s to %s, %d aggregated buckets (averages, not individual samples).\n"+
			"CPU: avg %.1f%%, peak %.1f%% around %s.\n",
		window.From.UTC().Format(time.RFC3339), window.To.UTC().Format(time.RFC3339),
		len(points), cpuSum/float64(len(points)), cpuMax, peak.UTC().Format(time.RFC3339))
}

func (t *storeToolbox) TopProcesses(_ context.Context, hostID string, at time.Time) (string, error) {
	if t.srv.store == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	// A window around the instant, because samples land on their own cadence
	// and an exact-match lookup would usually find nothing.
	window := storage.Range{From: at.Add(-2 * time.Minute), To: at.Add(2 * time.Minute)}
	samples, err := t.srv.store.QueryRange(hostID, window, 50)
	if err != nil {
		return "", err
	}
	if len(samples) == 0 {
		return "", nil
	}

	// The sample nearest the requested moment.
	nearest := samples[0]
	for _, s := range samples {
		if absDuration(s.Timestamp.Sub(at)) < absDuration(nearest.Timestamp.Sub(at)) {
			nearest = s
		}
	}
	nearest = t.srv.scrubber.SanitizeState(nearest)

	procs := append([]models.Process(nil), nearest.Processes...)
	sort.Slice(procs, func(i, j int) bool { return procs[i].CPU > procs[j].CPU })

	var b strings.Builder
	fmt.Fprintf(&b, "Processes at %s on %s (%d running in total):\n",
		nearest.Timestamp.UTC().Format(time.RFC3339), orUnknown(nearest.Hostname), nearest.ProcessCount)
	for i, p := range procs {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&b, "  %-20s pid %-7d cpu %5.1f%% of machine (%.0f%% of one core)  mem %.0f MB\n",
			p.Name, p.PID, p.CPU, p.CPUCore, float64(p.MemoryBytes)/(1<<20))
	}
	return b.String(), nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func (t *storeToolbox) ListHosts(context.Context) (string, error) {
	if t.srv.store == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	hosts, err := t.srv.store.ListHosts()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d host(s) reporting:\n", len(hosts))
	for _, h := range hosts {
		fmt.Fprintf(&b, "  id %s  hostname %s  agent %s  last seen %s\n",
			h.HostID, orUnknown(h.Hostname), orUnknown(h.AgentVersion),
			h.LastSeen.UTC().Format(time.RFC3339))
	}
	return b.String(), nil
}

func (t *storeToolbox) RecentAlerts(_ context.Context, limit int) (string, error) {
	if t.srv.store == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	events, err := t.srv.store.GetRecentAlertEvents(limit)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d alert transition(s), newest first:\n", len(events))
	for _, e := range events {
		fmt.Fprintf(&b, "  %s  %-10s %-24s %s = %.1f (threshold %.0f) on %s\n",
			e.OccurredAt.UTC().Format(time.RFC3339), e.State, e.RuleName,
			e.Metric, e.Value, e.Threshold, orUnknown(e.Hostname))
	}
	return b.String(), nil
}

func (t *storeToolbox) RecentLogs(ctx context.Context) (string, error) {
	if t.srv.logReader == nil {
		return "", fmt.Errorf("log collection is not available on this host")
	}
	raw, err := t.srv.logReader.GetLogsContextWithTimeout(ctx, 5*time.Second)
	if err != nil {
		return "", err
	}
	// The same scrubbing the one-shot analysis applies: usernames, emails, IPs
	// and home paths must not reach the model.
	return t.srv.scrubber.SanitizeLog(raw), nil
}

func (t *storeToolbox) RecentInsights(_ context.Context, limit int) (string, error) {
	if t.srv.store == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	insights, err := t.srv.store.ListInsights(storage.InsightQuery{Limit: limit})
	if err != nil {
		return "", err
	}
	if len(insights) == 0 {
		return "", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d past analysis(es), newest first:\n", len(insights))
	for _, in := range insights {
		fmt.Fprintf(&b, "  %s  %s\n    %s\n",
			in.Timestamp.UTC().Format(time.RFC3339), orUnknown(in.Status),
			truncate(in.Content, 300))
	}
	return b.String(), nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
