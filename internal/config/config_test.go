package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Gemini.ModelName != "gemini-2.5-flash-lite" {
		t.Errorf("Expected default model gemini-2.5-flash-lite, got %s", cfg.Gemini.ModelName)
	}
	if cfg.Database.MetricsRetentionHours != 24 {
		t.Errorf("Expected default metrics retention 24, got %d", cfg.Database.MetricsRetentionHours)
	}
	if cfg.Database.InsightsRetentionHours != 168 {
		t.Errorf("Expected default insights retention 168, got %d", cfg.Database.InsightsRetentionHours)
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	os.Setenv("SYS_SENTIENT_SERVER_PORT", "9090")
	defer os.Unsetenv("SYS_SENTIENT_SERVER_PORT")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Expected env override port 9090, got %d", cfg.Server.Port)
	}
}

func TestLoadConfig_AllowedOriginsEnvOverride(t *testing.T) {
	os.Setenv("SYS_SENTIENT_SERVER_ALLOWED_ORIGINS", "http://a.example,http://b.example")
	defer os.Unsetenv("SYS_SENTIENT_SERVER_ALLOWED_ORIGINS")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	expected := []string{"http://a.example", "http://b.example"}
	if len(cfg.Server.AllowedOrigins) != len(expected) {
		t.Fatalf("Expected %d origins, got %d: %#v", len(expected), len(cfg.Server.AllowedOrigins), cfg.Server.AllowedOrigins)
	}
	for i := range expected {
		if cfg.Server.AllowedOrigins[i] != expected[i] {
			t.Fatalf("Expected origin %d to be %q, got %q", i, expected[i], cfg.Server.AllowedOrigins[i])
		}
	}
}

func TestLoadConfig_AllowedOriginsEnvOverrideTrimsEmptyEntries(t *testing.T) {
	t.Setenv("SYS_SENTIENT_SERVER_ALLOWED_ORIGINS", " http://a.example , , http://b.example ")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	expected := []string{"http://a.example", "http://b.example"}
	if len(cfg.Server.AllowedOrigins) != len(expected) {
		t.Fatalf("Expected %d origins, got %d: %#v", len(expected), len(cfg.Server.AllowedOrigins), cfg.Server.AllowedOrigins)
	}
	for i := range expected {
		if cfg.Server.AllowedOrigins[i] != expected[i] {
			t.Fatalf("Expected origin %d to be %q, got %q", i, expected[i], cfg.Server.AllowedOrigins[i])
		}
	}
}

func TestLoadConfig_RetentionEnvOverride(t *testing.T) {
	os.Setenv("SYS_SENTIENT_DATABASE_METRICS_RETENTION_HOURS", "48")
	os.Setenv("SYS_SENTIENT_DATABASE_INSIGHTS_RETENTION_HOURS", "336")
	defer os.Unsetenv("SYS_SENTIENT_DATABASE_METRICS_RETENTION_HOURS")
	defer os.Unsetenv("SYS_SENTIENT_DATABASE_INSIGHTS_RETENTION_HOURS")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Database.MetricsRetentionHours != 48 {
		t.Fatalf("Expected metrics retention override 48, got %d", cfg.Database.MetricsRetentionHours)
	}
	if cfg.Database.InsightsRetentionHours != 336 {
		t.Fatalf("Expected insights retention override 336, got %d", cfg.Database.InsightsRetentionHours)
	}
}

func TestConfigValidateRejectsInvalidRetention(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Port: 8080},
		Database: DatabaseConfig{
			Path:                   "sys-sentient.db",
			MetricsRetentionHours:  24,
			InsightsRetentionHours: 168,
		},
		Collector: CollectorConfig{PollIntervalSeconds: 2},
	}

	cfg.Database.MetricsRetentionHours = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid metrics retention to fail")
	}

	cfg.Database.MetricsRetentionHours = 24
	cfg.Database.InsightsRetentionHours = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid insights retention to fail")
	}
}

func TestTopProcessesDefault(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Collector.TopProcesses != 10 {
		t.Fatalf("Collector.TopProcesses = %d, want 10", cfg.Collector.TopProcesses)
	}
}

func TestTopProcessesEnvOverride(t *testing.T) {
	t.Setenv("SYS_SENTIENT_COLLECTOR_TOP_PROCESSES", "25")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Collector.TopProcesses != 25 {
		t.Fatalf("Collector.TopProcesses = %d, want 25", cfg.Collector.TopProcesses)
	}
}

func TestTopProcessesValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{name: "zero is rejected", value: 0, wantErr: true},
		{name: "negative is rejected", value: -1, wantErr: true},
		{name: "one is allowed", value: 1, wantErr: false},
		{name: "typical value is allowed", value: 10, wantErr: false},
		{name: "upper bound is allowed", value: 200, wantErr: false},
		{name: "above upper bound is rejected", value: 201, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Collector.TopProcesses = tt.value

			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error for TopProcesses=%d", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil for TopProcesses=%d", err, tt.value)
			}
		})
	}
}

// validBaseConfig returns a Config that passes Validate, so each test can vary
// exactly one field.
func validBaseConfig() *Config {
	cfg := &Config{}
	cfg.Auth = AuthConfig{SessionIdleHours: 24, SessionMaxDays: 30, LoginRatePerMinute: 5}
	cfg.Server.Port = 8080
	cfg.Collector.PollIntervalSeconds = 2
	cfg.Collector.TopProcesses = 10
	cfg.Database.Path = "test.db"
	cfg.Database.MetricsRetentionHours = 24
	cfg.Database.InsightsRetentionHours = 168
	cfg.Database.MinuteRollupDays = 30
	cfg.Database.FiveMinuteRollupDays = 365
	cfg.Mode = ModeAllInOne
	cfg.Logging.Level = "info"
	cfg.Logging.Format = "text"
	return cfg
}

func TestRollupRetentionValidation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "minute tier must be at least a day",
			mutate:  func(c *Config) { c.Database.MinuteRollupDays = 0 },
			wantErr: "minute rollup retention",
		},
		{
			// Deleting the coarse tier before the fine one it was derived from
			// is backwards, and would silently shorten total history.
			name:    "five-minute tier cannot be shorter than the minute tier",
			mutate:  func(c *Config) { c.Database.FiveMinuteRollupDays = 7; c.Database.MinuteRollupDays = 30 },
			wantErr: "must be at least the minute retention",
		},
		{
			name:   "equal tiers are allowed",
			mutate: func(c *Config) { c.Database.FiveMinuteRollupDays = 30; c.Database.MinuteRollupDays = 30 },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := validBaseConfig()
			tt.mutate(c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoggingDefaultsAndValidation(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Logging.Level != "info" || cfg.Logging.Format != "text" {
		t.Fatalf("logging defaults = %+v, want level=info format=text", cfg.Logging)
	}

	tests := []struct {
		name    string
		level   string
		format  string
		wantErr bool
	}{
		{name: "debug text", level: "debug", format: "text"},
		{name: "json output", level: "info", format: "json"},
		{name: "case insensitive", level: "WARN", format: "JSON"},
		{name: "bad level", level: "verbose", format: "text", wantErr: true},
		{name: "bad format", level: "info", format: "xml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validBaseConfig()
			c.Logging.Level = tt.level
			c.Logging.Format = tt.format

			err := c.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want an error for level=%q format=%q", tt.level, tt.format)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestModeDefaultsAndValidation(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	// Existing single-node installs must be unaffected by the new modes.
	if cfg.Mode != ModeAllInOne {
		t.Fatalf("default mode = %q, want %q", cfg.Mode, ModeAllInOne)
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "all-in-one is valid", mutate: func(c *Config) { c.Mode = ModeAllInOne }},
		{name: "server is valid", mutate: func(c *Config) { c.Mode = ModeServer }},
		{name: "unknown mode rejected", mutate: func(c *Config) { c.Mode = "cluster" }, wantErr: "invalid mode"},
		{
			name: "agent requires a server url",
			mutate: func(c *Config) {
				c.Mode = ModeAgent
				c.Agent.Key = "k"
			},
			wantErr: "agent.server_url",
		},
		{
			name: "agent requires a scheme on the server url",
			mutate: func(c *Config) {
				c.Mode = ModeAgent
				c.Agent.ServerURL = "monitor.example.com"
				c.Agent.Key = "k"
			},
			wantErr: "http:// or https://",
		},
		{
			name: "agent requires a key",
			mutate: func(c *Config) {
				c.Mode = ModeAgent
				c.Agent.ServerURL = "https://monitor.example.com"
			},
			wantErr: "agent.key",
		},
		{
			name: "valid agent config",
			mutate: func(c *Config) {
				c.Mode = ModeAgent
				c.Agent.ServerURL = "https://monitor.example.com"
				c.Agent.Key = "secret"
				c.Agent.BatchSize = 60
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validBaseConfig()
			tt.mutate(c)

			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestAgentKeyDefaultsEmptyAndIsSeparateFromAPIKey(t *testing.T) {
	// The dashboard key is inlined into the published JS bundle, so it must not
	// be the same secret that grants write access to the fleet's data.
	t.Setenv("SYS_SENTIENT_SERVER_API_KEY", "dashboard-key")
	t.Setenv("SYS_SENTIENT_SERVER_AGENT_KEY", "agent-key")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Server.APIKey != "dashboard-key" {
		t.Fatalf("APIKey = %q", cfg.Server.APIKey)
	}
	if cfg.Server.AgentKey != "agent-key" {
		t.Fatalf("AgentKey = %q, want it independently configurable", cfg.Server.AgentKey)
	}
}

func TestAuthDefaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Insecure {
		t.Fatal("server.insecure must default to false")
	}
	if cfg.Auth.SessionIdleHours != 24 || cfg.Auth.SessionMaxDays != 30 || cfg.Auth.LoginRatePerMinute != 5 {
		t.Fatalf("unexpected auth defaults: %+v", cfg.Auth)
	}
}

func TestAuthEnvOverrides(t *testing.T) {
	t.Setenv("SYS_SENTIENT_SERVER_INSECURE", "true")
	t.Setenv("SYS_SENTIENT_AUTH_SESSION_IDLE_HOURS", "2")
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.Insecure || cfg.Auth.SessionIdleHours != 2 {
		t.Fatalf("env override not applied: insecure=%v idle=%d", cfg.Server.Insecure, cfg.Auth.SessionIdleHours)
	}
}

func TestAuthValidationRejectsZeroes(t *testing.T) {
	for _, env := range []string{"SYS_SENTIENT_AUTH_SESSION_IDLE_HOURS", "SYS_SENTIENT_AUTH_SESSION_MAX_DAYS", "SYS_SENTIENT_AUTH_LOGIN_RATE_PER_MINUTE"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "0")
			if _, err := LoadConfig(""); err == nil {
				t.Fatalf("%s=0 should fail validation", env)
			}
		})
	}
}

// The default origin allowlist must name the port this project's Vite config
// actually uses (web/vite.config.ts sets 3000). It previously listed only
// Vite's default 5173, so every `npm run dev` session got a 403 on the
// WebSocket upgrade.
func TestDefaultAllowedOriginsCoverTheDevServerPort(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://localhost:3000"
	for _, origin := range cfg.Server.AllowedOrigins {
		if origin == want {
			return
		}
	}
	t.Fatalf("allowed_origins = %v, missing %q", cfg.Server.AllowedOrigins, want)
}
