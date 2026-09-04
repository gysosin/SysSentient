package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Gemini    GeminiConfig    `mapstructure:"gemini"`
	Privacy   PrivacyConfig   `mapstructure:"privacy"`
	Collector CollectorConfig `mapstructure:"collector"`
	Alerting  AlertingConfig  `mapstructure:"alerting"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Mode      Mode            `mapstructure:"mode"`
	Agent     AgentConfig     `mapstructure:"agent"`
	Auth      AuthConfig      `mapstructure:"auth"`
}

type ServerConfig struct {
	Port   int    `mapstructure:"port"`
	APIKey string `mapstructure:"api_key"`
	// AgentKey authenticates agents pushing to /api/ingest. Deliberately
	// separate from APIKey: the dashboard key is inlined into the published
	// JavaScript bundle and readable by anyone who can load the page, so it
	// must never also grant write access to the fleet's data.
	AgentKey string `mapstructure:"agent_key"`
	// PublicURL is the address agents should call back on, used to build the
	// enrolment command shown in the UI. Behind a reverse proxy the server
	// cannot infer this, because the request's Host is the proxy's own.
	PublicURL      string   `mapstructure:"public_url"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	// Insecure disables authentication entirely. It exists for throwaway
	// local runs and is warned about on every start; never set it on a
	// network-reachable install.
	Insecure bool `mapstructure:"insecure"`
}

type AuthConfig struct {
	// SessionIdleHours is how long a session survives without activity.
	SessionIdleHours int `mapstructure:"session_idle_hours"`
	// SessionMaxDays caps a session's total life regardless of activity.
	SessionMaxDays int `mapstructure:"session_max_days"`
	// LoginRatePerMinute bounds password attempts per client IP.
	LoginRatePerMinute int `mapstructure:"login_rate_per_minute"`
}

type DatabaseConfig struct {
	Path                  string `mapstructure:"path"`
	MetricsRetentionHours int    `mapstructure:"metrics_retention_hours"`
	// Tiered retention. Raw samples are expensive — roughly 4.3 KB a row at a
	// two-second interval — so full resolution is kept briefly and rolled up
	// rather than discarded. These control how long each aggregate survives.
	MinuteRollupDays       int `mapstructure:"minute_rollup_days"`
	FiveMinuteRollupDays   int `mapstructure:"five_minute_rollup_days"`
	InsightsRetentionHours int `mapstructure:"insights_retention_hours"`
	// ArchivePath sends aged-out rollups to compressed files instead of
	// deleting them. Empty disables archiving, which is the historical
	// behaviour: retention alone. Retention decides what the dashboard can
	// query; archiving decides whether anything older still exists.
	ArchivePath string `mapstructure:"archive_path"`
}

type GeminiConfig struct {
	APIKey       string  `mapstructure:"api_key"`
	ModelName    string  `mapstructure:"model_name"`
	MaxDailyCost float64 `mapstructure:"max_daily_cost"`
}

type PrivacyConfig struct {
	MaskIPs       bool `mapstructure:"mask_ips"`
	MaskEmails    bool `mapstructure:"mask_emails"`
	MaskUsernames bool `mapstructure:"mask_usernames"`
}

// Mode selects what the process does.
//
// A single binary with modes rather than separate sys-agent/sys-server
// binaries: the SQLite driver is cgo-based, so every extra binary doubles an
// already-awkward build matrix. The separation is logical, not physical, and
// an agent-only build can later drop storage behind a build tag.
type Mode string

const (
	// ModeAllInOne collects locally and serves the dashboard. The historical
	// behaviour, and still the default so existing installs are unaffected.
	ModeAllInOne Mode = "all-in-one"
	// ModeServer receives samples from agents and serves the dashboard. It
	// does not collect from the machine it runs on.
	ModeServer Mode = "server"
	// ModeAgent collects locally and pushes to a server. No dashboard, no
	// local API.
	ModeAgent Mode = "agent"
)

// AgentConfig configures push-mode operation.
type AgentConfig struct {
	// ServerURL is the base URL of the sys-sentient server, e.g.
	// https://monitor.example.com
	ServerURL string `mapstructure:"server_url"`
	// Key authenticates this agent to the server's ingest endpoint.
	Key string `mapstructure:"key"`
	// SpoolPath buffers samples while the server is unreachable so a network
	// partition loses no data.
	SpoolPath string `mapstructure:"spool_path"`
	// BatchSize caps how many spooled samples are sent per request.
	BatchSize int `mapstructure:"batch_size"`
	// CACertPath trusts a private CA for the server connection. This is the
	// correct way to run against an internal certificate.
	CACertPath string `mapstructure:"ca_cert_path"`
	// InsecureSkipVerify disables TLS verification entirely. Last resort; it
	// makes the connection trivially interceptable and is warned about on
	// every start.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// LoggingConfig controls the daemon's own log output.
type LoggingConfig struct {
	// Level is debug|info|warn|error.
	Level string `mapstructure:"level"`
	// Format is text|json. JSON is what log aggregators want.
	Format string `mapstructure:"format"`
}

// AlertingConfig controls threshold evaluation and notification delivery.
type AlertingConfig struct {
	Enabled bool `mapstructure:"enabled"`
	// WebhookURL receives the raw alert JSON; SlackWebhookURL receives a
	// formatted message. Both optional — with neither set, alerts are still
	// evaluated and visible in the UI but nobody is notified.
	WebhookURL      string `mapstructure:"webhook_url"`
	SlackWebhookURL string `mapstructure:"slack_webhook_url"`
}

type CollectorConfig struct {
	PollIntervalSeconds int `mapstructure:"poll_interval_seconds"`
	// HostID overrides the derived machine identifier. Containers built from
	// one image share /etc/machine-id, which would make a whole fleet report
	// as a single host; this lets the operator distinguish them.
	HostID string `mapstructure:"host_id"`
	// TopProcesses caps how many processes each snapshot records. Every
	// additional process costs several /proc reads per poll, so this trades
	// dashboard detail against collector overhead.
	TopProcesses int `mapstructure:"top_processes"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.api_key", "")
	// 8080 is the daemon serving its own dashboard; 3000 is this repo's Vite
	// dev server (web/vite.config.ts), and 5173 is Vite's default for anyone
	// who has not overridden it. Omitting 3000 made every `npm run dev`
	// session fail the WebSocket upgrade with a 403.
	v.SetDefault("server.allowed_origins", []string{
		"http://localhost:8080",
		"http://localhost:3000",
		"http://localhost:5173",
	})
	v.SetDefault("database.path", "sys-sentient.db")
	v.SetDefault("database.metrics_retention_hours", 24)
	v.SetDefault("database.minute_rollup_days", 30)
	v.SetDefault("database.five_minute_rollup_days", 365)
	v.SetDefault("database.archive_path", "")
	v.SetDefault("database.insights_retention_hours", 7*24)
	v.SetDefault("gemini.api_key", "") // Ensure env var is picked up
	v.SetDefault("gemini.model_name", "gemini-2.5-flash-lite")
	v.SetDefault("gemini.max_daily_cost", 1.0)
	v.SetDefault("privacy.mask_ips", true)
	v.SetDefault("privacy.mask_emails", true)
	v.SetDefault("privacy.mask_usernames", true)
	v.SetDefault("collector.poll_interval_seconds", 2)
	v.SetDefault("collector.top_processes", 10)
	v.SetDefault("collector.host_id", "")
	v.SetDefault("mode", string(ModeAllInOne))
	v.SetDefault("server.agent_key", "")
	v.SetDefault("agent.server_url", "")
	v.SetDefault("agent.key", "")
	v.SetDefault("agent.spool_path", "sys-sentient-spool.db")
	v.SetDefault("agent.batch_size", 60)
	v.SetDefault("agent.ca_cert_path", "")
	v.SetDefault("agent.insecure_skip_verify", false)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
	v.SetDefault("alerting.enabled", true)
	v.SetDefault("alerting.webhook_url", "")
	v.SetDefault("alerting.slack_webhook_url", "")
	v.SetDefault("server.insecure", false)
	v.SetDefault("auth.session_idle_hours", 24)
	v.SetDefault("auth.session_max_days", 30)
	v.SetDefault("auth.login_rate_per_minute", 5)

	// Environment variables
	v.SetEnvPrefix("SYS_SENTIENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/sys-sentient/")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is fine, we use defaults/env
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}
	cfg.Server.AllowedOrigins = normalizeStringList(cfg.Server.AllowedOrigins)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	if c.Collector.PollIntervalSeconds < 1 {
		return fmt.Errorf("poll interval must be at least 1 second, got %d", c.Collector.PollIntervalSeconds)
	}

	if c.Collector.PollIntervalSeconds > 3600 {
		return fmt.Errorf("poll interval too large: %d seconds (max 3600)", c.Collector.PollIntervalSeconds)
	}

	switch c.Mode {
	case ModeAllInOne, ModeServer, ModeAgent:
	default:
		return fmt.Errorf("invalid mode %q (want all-in-one, server or agent)", c.Mode)
	}

	if c.Mode == ModeAgent {
		if strings.TrimSpace(c.Agent.ServerURL) == "" {
			return fmt.Errorf("agent mode requires agent.server_url")
		}
		if !strings.HasPrefix(c.Agent.ServerURL, "http://") && !strings.HasPrefix(c.Agent.ServerURL, "https://") {
			return fmt.Errorf("agent.server_url must start with http:// or https://, got %q", c.Agent.ServerURL)
		}
		if strings.TrimSpace(c.Agent.Key) == "" {
			return fmt.Errorf("agent mode requires agent.key to authenticate to the server")
		}
		if c.Agent.BatchSize < 1 {
			return fmt.Errorf("agent.batch_size must be at least 1, got %d", c.Agent.BatchSize)
		}
	}

	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q (want debug, info, warn or error)", c.Logging.Level)
	}

	switch strings.ToLower(c.Logging.Format) {
	case "text", "json":
	default:
		return fmt.Errorf("invalid log format %q (want text or json)", c.Logging.Format)
	}

	if c.Collector.TopProcesses < 1 {
		return fmt.Errorf("top processes must be at least 1, got %d", c.Collector.TopProcesses)
	}

	if c.Collector.TopProcesses > 200 {
		return fmt.Errorf("top processes too large: %d (max 200)", c.Collector.TopProcesses)
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	if c.Database.MetricsRetentionHours < 1 {
		return fmt.Errorf("metrics retention must be at least 1 hour, got %d", c.Database.MetricsRetentionHours)
	}

	if c.Database.MinuteRollupDays < 1 {
		return fmt.Errorf("minute rollup retention must be at least 1 day, got %d", c.Database.MinuteRollupDays)
	}
	// A five-minute tier shorter than the minute tier would delete the coarse
	// data before the fine data it was derived from, which is backwards.
	if c.Database.FiveMinuteRollupDays < c.Database.MinuteRollupDays {
		return fmt.Errorf("five-minute rollup retention (%d days) must be at least the minute retention (%d days)",
			c.Database.FiveMinuteRollupDays, c.Database.MinuteRollupDays)
	}

	if c.Database.InsightsRetentionHours < 1 {
		return fmt.Errorf("insights retention must be at least 1 hour, got %d", c.Database.InsightsRetentionHours)
	}

	if c.Gemini.MaxDailyCost < 0 {
		return fmt.Errorf("max daily cost cannot be negative: %.2f", c.Gemini.MaxDailyCost)
	}

	if c.Auth.SessionIdleHours < 1 {
		return fmt.Errorf("auth.session_idle_hours must be at least 1, got %d", c.Auth.SessionIdleHours)
	}
	if c.Auth.SessionMaxDays < 1 {
		return fmt.Errorf("auth.session_max_days must be at least 1, got %d", c.Auth.SessionMaxDays)
	}
	if c.Auth.LoginRatePerMinute < 1 {
		return fmt.Errorf("auth.login_rate_per_minute must be at least 1, got %d", c.Auth.LoginRatePerMinute)
	}

	return nil
}

func normalizeStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
