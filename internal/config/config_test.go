package config

import (
	"os"
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
