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
