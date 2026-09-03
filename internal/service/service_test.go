package service

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sys-sentient")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

func TestResolveRejectsMissingBinary(t *testing.T) {
	cfg := Config{ExecPath: filepath.Join(t.TempDir(), "absent")}
	if err := cfg.Resolve(); err == nil {
		t.Fatal("Resolve accepted a path with no file")
	}
}

func TestResolveRejectsDirectory(t *testing.T) {
	cfg := Config{ExecPath: t.TempDir()}
	if err := cfg.Resolve(); err == nil {
		t.Fatal("Resolve accepted a directory as the executable")
	}
}

func TestResolveMakesPathsAbsolute(t *testing.T) {
	bin := fakeBinary(t)
	dir := filepath.Dir(bin)
	config := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(config, []byte("mode: agent\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// A service manager has no working directory to resolve a relative path
	// against, so this must fail at install time, not at start time.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := Config{ExecPath: "sys-sentient", ConfigPath: "agent.yaml"}
	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !filepath.IsAbs(cfg.ExecPath) {
		t.Errorf("ExecPath = %q, want an absolute path", cfg.ExecPath)
	}
	if !filepath.IsAbs(cfg.ConfigPath) {
		t.Errorf("ConfigPath = %q, want an absolute path", cfg.ConfigPath)
	}
}

func TestResolveRejectsMissingConfig(t *testing.T) {
	cfg := Config{ExecPath: fakeBinary(t), ConfigPath: filepath.Join(t.TempDir(), "nope.yaml")}
	if err := cfg.Resolve(); err == nil {
		t.Fatal("Resolve accepted a config path that does not exist")
	}
}

func TestCommandLineIncludesConfig(t *testing.T) {
	cfg := Config{ExecPath: "/usr/bin/sys-sentient", ConfigPath: "/etc/sys-sentient/agent.yaml"}
	got := cfg.commandLine()
	want := "/usr/bin/sys-sentient --config /etc/sys-sentient/agent.yaml"
	if got != want {
		t.Errorf("commandLine() = %q, want %q", got, want)
	}
}

func TestCommandLineOmitsEmptyConfig(t *testing.T) {
	cfg := Config{ExecPath: "/usr/bin/sys-sentient"}
	if got := cfg.commandLine(); got != "/usr/bin/sys-sentient" {
		t.Errorf("commandLine() = %q, want no --config flag", got)
	}
}
