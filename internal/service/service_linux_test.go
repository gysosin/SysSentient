package service

import (
	"strings"
	"testing"
)

func TestUnderTmpDetectsHiddenPaths(t *testing.T) {
	// PrivateTmp gives the service its own /tmp, so a binary or config under
	// /tmp becomes invisible to it and the unit fails with a 203/EXEC "No such
	// file or directory" for a file that plainly exists. Observed live.
	cases := map[string]bool{
		"/tmp/build/sys-sentient":      true,
		"/var/tmp/sys-sentient":        true,
		"/usr/bin/sys-sentient":        false,
		"/home/op/.local/sys-sentient": false,
		"":                             false,
		// Not under /tmp despite the prefix.
		"/tmpfiles/sys-sentient": false,
	}
	for path, want := range cases {
		if got := underTmp(path); got != want {
			t.Errorf("underTmp(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestParseShowReadsSystemctlProperties(t *testing.T) {
	raw := "ActiveState=activating\nSubState=auto-restart\nResult=exit-code\nNRestarts=10"
	got := parseShow(raw)
	for key, want := range map[string]string{
		"ActiveState": "activating",
		"SubState":    "auto-restart",
		"Result":      "exit-code",
		"NRestarts":   "10",
	} {
		if got[key] != want {
			t.Errorf("parseShow()[%q] = %q, want %q", key, got[key], want)
		}
	}
}

func TestParseShowIgnoresMalformedLines(t *testing.T) {
	got := parseShow("ActiveState=active\n\ngarbage line\nSubState=running")
	if got["ActiveState"] != "active" || got["SubState"] != "running" {
		t.Errorf("parseShow dropped good values around a malformed line: %v", got)
	}
	if len(got) != 2 {
		t.Errorf("parseShow kept %d entries, want 2", len(got))
	}
}

func TestLogHintMatchesScope(t *testing.T) {
	if got := logHint(true); !strings.Contains(got, "--user") {
		t.Errorf("logHint(user) = %q, want a --user journalctl command", got)
	}
	if got := logHint(false); strings.Contains(got, "--user") {
		t.Errorf("logHint(system) = %q, want a system journalctl command", got)
	}
}

func TestUnitDirIsXDGForUserScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/op/.config")
	dir, err := unitDir(true)
	if err != nil {
		t.Fatalf("unitDir: %v", err)
	}
	if dir != "/home/op/.config/systemd/user" {
		t.Errorf("unitDir(user) = %q", dir)
	}

	if dir, err := unitDir(false); err != nil || dir != "/etc/systemd/system" {
		t.Errorf("unitDir(system) = %q, %v", dir, err)
	}
}
