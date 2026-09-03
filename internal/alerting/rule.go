// Package alerting evaluates threshold rules against collected metrics and
// tracks alert lifecycle.
//
// Before this package the daemon had no alerting at all: two hardcoded numbers
// in main.go (CPU > 80, memory > 90) triggered an AI analysis and nothing else.
// They were single-sample, so any transient spike fired; nobody was ever
// notified; and there was no history, no acknowledgement and no way to change a
// threshold without recompiling.
package alerting

import (
	"fmt"
	"strings"
	"time"

	"sys-sentient/internal/models"
)

// Metric names a scalar series a rule can be written against.
type Metric string

const (
	MetricCPUUsage      Metric = "cpu_usage"
	MetricMemoryPercent Metric = "memory_percent"
	MetricSwapPercent   Metric = "swap_percent"
	MetricDiskPercent   Metric = "disk_percent"
	MetricLoadAvg1      Metric = "load_avg_1"
	MetricLoadAvg5      Metric = "load_avg_5"
	MetricLoadAvg15     Metric = "load_avg_15"
	MetricTemperature   Metric = "temperature"
)

// SupportedMetrics is the closed set a rule may reference.
var SupportedMetrics = []Metric{
	MetricCPUUsage, MetricMemoryPercent, MetricSwapPercent, MetricDiskPercent,
	MetricLoadAvg1, MetricLoadAvg5, MetricLoadAvg15, MetricTemperature,
}

// Comparator is the test applied between the observed value and the threshold.
type Comparator string

const (
	GreaterThan      Comparator = ">"
	GreaterThanEqual Comparator = ">="
	LessThan         Comparator = "<"
	LessThanEqual    Comparator = "<="
)

// Severity ranks a firing alert.
type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// State is where an alert sits in its lifecycle.
type State string

const (
	// StatePending means the condition is true but has not yet held for the
	// rule's duration. This is what suppresses transient spikes.
	StatePending  State = "pending"
	StateFiring   State = "firing"
	StateResolved State = "resolved"
)

// Rule is a single threshold condition.
type Rule struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Metric    Metric     `json:"metric"`
	Op        Comparator `json:"op"`
	Threshold float64    `json:"threshold"`
	// For is how long the condition must hold before the alert fires. Zero
	// means fire immediately, which is almost never what an operator wants.
	For      time.Duration `json:"for"`
	Severity Severity      `json:"severity"`
	Enabled  bool          `json:"enabled"`
}

// Validate reports whether the rule is usable.
func (r Rule) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("rule id cannot be empty")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("rule %q: name cannot be empty", r.ID)
	}

	var known bool
	for _, m := range SupportedMetrics {
		if r.Metric == m {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("rule %q: unsupported metric %q", r.ID, r.Metric)
	}

	switch r.Op {
	case GreaterThan, GreaterThanEqual, LessThan, LessThanEqual:
	default:
		return fmt.Errorf("rule %q: unsupported comparator %q", r.ID, r.Op)
	}

	switch r.Severity {
	case SeverityWarning, SeverityCritical:
	default:
		return fmt.Errorf("rule %q: unsupported severity %q", r.ID, r.Severity)
	}

	if r.For < 0 {
		return fmt.Errorf("rule %q: for duration cannot be negative", r.ID)
	}
	return nil
}

// matches reports whether the observed value satisfies the rule's condition.
func (r Rule) matches(value float64) bool {
	switch r.Op {
	case GreaterThan:
		return value > r.Threshold
	case GreaterThanEqual:
		return value >= r.Threshold
	case LessThan:
		return value < r.Threshold
	case LessThanEqual:
		return value <= r.Threshold
	default:
		return false
	}
}

// Alert is one rule's current or historical activation.
type Alert struct {
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	Metric     Metric    `json:"metric"`
	State      State     `json:"state"`
	Severity   Severity  `json:"severity"`
	Value      float64   `json:"value"`
	Threshold  float64   `json:"threshold"`
	HostID     string    `json:"host_id"`
	Hostname   string    `json:"hostname"`
	StartedAt  time.Time `json:"started_at"`
	FiredAt    time.Time `json:"fired_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
	// Acknowledged silences notifications for this activation without
	// resolving it.
	Acknowledged bool `json:"acknowledged"`
}

// Summary renders a human-readable one-liner for notifications.
func (a Alert) Summary() string {
	host := a.Hostname
	if host == "" {
		host = a.HostID
	}
	if host == "" {
		host = "unknown host"
	}
	return fmt.Sprintf("%s: %s is %.1f (threshold %s %.1f) on %s",
		strings.ToUpper(string(a.Severity)), a.RuleName, a.Value,
		string(a.Metric), a.Threshold, host)
}

// DefaultRules is a starting set so a fresh install alerts on the conditions
// that actually cause outages. Deliberately conservative durations: the old
// hardcoded checks were single-sample and fired on every build spike.
func DefaultRules() []Rule {
	return []Rule{
		{ID: "cpu-high", Name: "CPU sustained high", Metric: MetricCPUUsage, Op: GreaterThan, Threshold: 90, For: 5 * time.Minute, Severity: SeverityWarning, Enabled: true},
		{ID: "memory-high", Name: "Memory nearly exhausted", Metric: MetricMemoryPercent, Op: GreaterThan, Threshold: 90, For: 2 * time.Minute, Severity: SeverityCritical, Enabled: true},
		{ID: "swap-high", Name: "Swap heavily used", Metric: MetricSwapPercent, Op: GreaterThan, Threshold: 75, For: 5 * time.Minute, Severity: SeverityWarning, Enabled: true},
		{ID: "disk-full", Name: "Filesystem nearly full", Metric: MetricDiskPercent, Op: GreaterThan, Threshold: 90, For: time.Minute, Severity: SeverityCritical, Enabled: true},
		{ID: "load-high", Name: "Load average high", Metric: MetricLoadAvg5, Op: GreaterThan, Threshold: 8, For: 5 * time.Minute, Severity: SeverityWarning, Enabled: true},
		{ID: "temp-high", Name: "CPU temperature high", Metric: MetricTemperature, Op: GreaterThan, Threshold: 90, For: 2 * time.Minute, Severity: SeverityWarning, Enabled: true},
	}
}

// MetricValue extracts the scalar a rule is written against.
//
// Percentages are derived here rather than stored, so a rule always compares
// against the same unit regardless of the host's absolute capacity.
func MetricValue(state models.SystemState, metric Metric) (float64, bool) {
	pct := func(used, total uint64) (float64, bool) {
		if total == 0 {
			return 0, false
		}
		return float64(used) / float64(total) * 100, true
	}

	switch metric {
	case MetricCPUUsage:
		return state.CPUUsage, true
	case MetricMemoryPercent:
		return pct(state.MemoryUsed, state.MemoryTotal)
	case MetricSwapPercent:
		return pct(state.SwapUsed, state.SwapTotal)
	case MetricDiskPercent:
		// The fullest filesystem: one full disk is an incident even if the
		// others are empty, so an average would hide it.
		var worst float64
		var found bool
		for _, fs := range state.Filesystems {
			if fs.TotalBytes == 0 {
				continue
			}
			found = true
			if fs.UsedPercent > worst {
				worst = fs.UsedPercent
			}
		}
		return worst, found
	case MetricLoadAvg1:
		return state.LoadAvg1, true
	case MetricLoadAvg5:
		return state.LoadAvg5, true
	case MetricLoadAvg15:
		return state.LoadAvg15, true
	case MetricTemperature:
		// A host with no sensor reports 0; that must not read as "very cool"
		// and satisfy a `<` rule.
		return state.Temperature, state.Temperature > 0
	default:
		return 0, false
	}
}
