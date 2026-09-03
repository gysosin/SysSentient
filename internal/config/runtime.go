package config

import (
	"fmt"
	"sync"
	"time"
)

// Runtime holds the settings an operator can change without restarting.
//
// Everything else stays boot-time. A port, a database path or a privacy
// setting cannot be changed under a running process without reopening sockets
// or re-deriving state, and pretending otherwise in the UI would be worse than
// saying "restart required". These three can genuinely take effect live.
type Runtime struct {
	mu sync.RWMutex

	pollInterval     time.Duration
	retentionHours   int
	minuteDays       int
	fiveMinuteDays   int
	logLevel         string
	onPollChange     func(time.Duration)
	onLogLevelChange func(string)
}

// RuntimeValues is the readable snapshot, for the API and the UI.
type RuntimeValues struct {
	PollIntervalSeconds   int    `json:"poll_interval_seconds"`
	MetricsRetentionHours int    `json:"metrics_retention_hours"`
	MinuteRollupDays      int    `json:"minute_rollup_days"`
	FiveMinuteRollupDays  int    `json:"five_minute_rollup_days"`
	LogLevel              string `json:"log_level"`
}

// NewRuntime seeds the live settings from the loaded configuration.
func NewRuntime(cfg *Config) *Runtime {
	return &Runtime{
		pollInterval:   time.Duration(cfg.Collector.PollIntervalSeconds) * time.Second,
		retentionHours: cfg.Database.MetricsRetentionHours,
		minuteDays:     cfg.Database.MinuteRollupDays,
		fiveMinuteDays: cfg.Database.FiveMinuteRollupDays,
		logLevel:       cfg.Logging.Level,
	}
}

// OnPollIntervalChange registers the callback that retimes the collector.
// Without it a changed interval would be stored and never applied, which is
// the failure mode this whole feature exists to avoid.
func (r *Runtime) OnPollIntervalChange(fn func(time.Duration)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onPollChange = fn
}

// OnLogLevelChange registers the callback that reconfigures the logger.
func (r *Runtime) OnLogLevelChange(fn func(string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onLogLevelChange = fn
}

// Values returns the current settings.
func (r *Runtime) Values() RuntimeValues {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RuntimeValues{
		PollIntervalSeconds:   int(r.pollInterval / time.Second),
		MetricsRetentionHours: r.retentionHours,
		MinuteRollupDays:      r.minuteDays,
		FiveMinuteRollupDays:  r.fiveMinuteDays,
		LogLevel:              r.logLevel,
	}
}

// PollInterval is read by the collector loop on every tick.
func (r *Runtime) PollInterval() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pollInterval
}

// Retention returns the current tier policy.
func (r *Runtime) Retention() (rawHours, minuteDays, fiveMinuteDays int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.retentionHours, r.minuteDays, r.fiveMinuteDays
}

// RuntimeUpdate is a partial change. Nil fields are left alone, so a caller
// changing one setting cannot accidentally reset the others to zero.
type RuntimeUpdate struct {
	PollIntervalSeconds   *int    `json:"poll_interval_seconds,omitempty"`
	MetricsRetentionHours *int    `json:"metrics_retention_hours,omitempty"`
	MinuteRollupDays      *int    `json:"minute_rollup_days,omitempty"`
	FiveMinuteRollupDays  *int    `json:"five_minute_rollup_days,omitempty"`
	LogLevel              *string `json:"log_level,omitempty"`
}

// Apply validates and applies an update atomically.
//
// Validation happens against the *resulting* configuration rather than each
// field alone, so a pair of individually-valid values that contradict each
// other — a five-minute tier shorter than the minute tier it is derived from —
// is rejected. Nothing is applied unless everything validates.
func (r *Runtime) Apply(u RuntimeUpdate) (RuntimeValues, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	next := RuntimeValues{
		PollIntervalSeconds:   int(r.pollInterval / time.Second),
		MetricsRetentionHours: r.retentionHours,
		MinuteRollupDays:      r.minuteDays,
		FiveMinuteRollupDays:  r.fiveMinuteDays,
		LogLevel:              r.logLevel,
	}

	if u.PollIntervalSeconds != nil {
		next.PollIntervalSeconds = *u.PollIntervalSeconds
	}
	if u.MetricsRetentionHours != nil {
		next.MetricsRetentionHours = *u.MetricsRetentionHours
	}
	if u.MinuteRollupDays != nil {
		next.MinuteRollupDays = *u.MinuteRollupDays
	}
	if u.FiveMinuteRollupDays != nil {
		next.FiveMinuteRollupDays = *u.FiveMinuteRollupDays
	}
	if u.LogLevel != nil {
		next.LogLevel = *u.LogLevel
	}

	if err := validateRuntime(next); err != nil {
		return RuntimeValues{}, err
	}

	pollChanged := next.PollIntervalSeconds != int(r.pollInterval/time.Second)
	levelChanged := next.LogLevel != r.logLevel

	r.pollInterval = time.Duration(next.PollIntervalSeconds) * time.Second
	r.retentionHours = next.MetricsRetentionHours
	r.minuteDays = next.MinuteRollupDays
	r.fiveMinuteDays = next.FiveMinuteRollupDays
	r.logLevel = next.LogLevel

	// Callbacks run while the lock is held so a concurrent Apply cannot
	// interleave and leave the collector timed to a value that was already
	// superseded.
	if pollChanged && r.onPollChange != nil {
		r.onPollChange(r.pollInterval)
	}
	if levelChanged && r.onLogLevelChange != nil {
		r.onLogLevelChange(r.logLevel)
	}

	return next, nil
}

func validateRuntime(v RuntimeValues) error {
	// A sub-second interval would have the collector overlapping itself: one
	// collection costs roughly 20ms, but the process scan scales with how busy
	// the host is, and a monitoring agent must not become the load.
	if v.PollIntervalSeconds < 1 || v.PollIntervalSeconds > 3600 {
		return fmt.Errorf("poll interval must be between 1 and 3600 seconds, got %d", v.PollIntervalSeconds)
	}
	if v.MetricsRetentionHours < 1 {
		return fmt.Errorf("metrics retention must be at least 1 hour, got %d", v.MetricsRetentionHours)
	}
	if v.MinuteRollupDays < 1 {
		return fmt.Errorf("minute rollup retention must be at least 1 day, got %d", v.MinuteRollupDays)
	}
	if v.FiveMinuteRollupDays < v.MinuteRollupDays {
		return fmt.Errorf("five-minute rollup retention (%d days) must be at least the minute retention (%d days)",
			v.FiveMinuteRollupDays, v.MinuteRollupDays)
	}
	switch v.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log level must be debug, info, warn or error, got %q", v.LogLevel)
	}
	return nil
}
