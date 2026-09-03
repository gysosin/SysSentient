package pii

import (
	"strings"
	"testing"

	"sys-sentient/internal/models"
)

func TestScrubber_SanitizeLog(t *testing.T) {
	tests := []struct {
		name          string
		maskIPs       bool
		maskEmails    bool
		maskUsernames bool
		input         string
		expected      string
	}{
		{
			name:          "Mask All",
			maskIPs:       true,
			maskEmails:    true,
			maskUsernames: true,
			input:         "User xyfo at /home/xyfo logged in from 192.168.1.1 using test@example.com",
			expected:      "User xyfo at /home/[USER_REDACTED] logged in from [IP_REDACTED] using [EMAIL_REDACTED]",
		},
		{
			name:          "Mask None",
			maskIPs:       false,
			maskEmails:    false,
			maskUsernames: false,
			input:         "192.168.1.1 test@example.com",
			expected:      "192.168.1.1 test@example.com",
		},
		{
			name:          "Mask Only IP",
			maskIPs:       true,
			maskEmails:    false,
			maskUsernames: false,
			input:         "192.168.1.1 test@example.com",
			expected:      "[IP_REDACTED] test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScrubber(tt.maskIPs, tt.maskEmails, tt.maskUsernames)
			if got := s.SanitizeLog(tt.input); got != tt.expected {
				t.Errorf("Scrubber.SanitizeLog() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSanitizeLogMasksIPv6(t *testing.T) {
	// The dashboard-side scrubber carried a full IPv6 pattern while this one -
	// the only scrubber that actually gates what leaves the machine for Gemini -
	// had none. journald and dmesg are full of IPv6 on any modern Linux host.
	s := NewScrubber(true, true, true)

	tests := []struct {
		name  string
		input string
	}{
		{name: "full address", input: "peer 2001:0db8:85a3:0000:0000:8a2e:0370:7334 closed"},
		{name: "compressed", input: "connect to 2001:db8::8a2e:370:7334 failed"},
		{name: "loopback", input: "bound ::1 port 8080"},
		{name: "link-local", input: "iface fe80::1ff:fe23:4567:890a up"},
		{name: "unique local", input: "route via fd00::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.SanitizeLog(tt.input)
			if !strings.Contains(got, "[IP_REDACTED]") {
				t.Fatalf("SanitizeLog(%q) = %q, want an [IP_REDACTED] marker", tt.input, got)
			}
			if strings.Contains(got, "2001:") || strings.Contains(got, "fe80:") || strings.Contains(got, "fd00:") {
				t.Fatalf("SanitizeLog(%q) = %q, raw IPv6 survived", tt.input, got)
			}
		})
	}
}

func TestSanitizeLogIPv6RespectsToggle(t *testing.T) {
	s := NewScrubber(false, true, true)
	input := "peer 2001:db8::1 closed"

	if got := s.SanitizeLog(input); got != input {
		t.Fatalf("SanitizeLog(%q) = %q, want unchanged when maskIPs is off", input, got)
	}
}

func TestSanitizeLogPreservesNonAddressColons(t *testing.T) {
	// Timestamps and key:value pairs must not be mangled by the IPv6 pattern.
	s := NewScrubber(true, true, true)
	input := "Sep 02 00:25:21 fedora kernel: pipe A start=3090258 time 404 us"

	got := s.SanitizeLog(input)
	if got != input {
		t.Fatalf("SanitizeLog(%q) = %q, want unchanged (no addresses present)", input, got)
	}
}

func TestSanitizeStateScrubsTopProcesses(t *testing.T) {
	// TopProcesses is interpolated straight into the Gemini prompt
	// (internal/ai/gemini.go:83) and was never scrubbed - process names carry
	// paths, hostnames and user identity.
	s := NewScrubber(true, true, true)

	state := models.SystemState{
		TopProcesses: "/home/alice/bin/agent (12.5%, 100MB), mailer-bob@example.com (3%, 10MB)",
		Processes: []models.Process{
			{PID: 1, Name: "/home/alice/bin/agent", User: "alice", CPU: 12.5, Memory: 100},
		},
	}

	got := s.SanitizeState(state)

	if strings.Contains(got.TopProcesses, "alice") {
		t.Fatalf("SanitizeState left a username in TopProcesses: %q", got.TopProcesses)
	}
	if strings.Contains(got.TopProcesses, "bob@example.com") {
		t.Fatalf("SanitizeState left an email in TopProcesses: %q", got.TopProcesses)
	}
	if strings.Contains(got.Processes[0].Name, "alice") {
		t.Fatalf("SanitizeState left a username in Processes[0].Name: %q", got.Processes[0].Name)
	}
	if got.Processes[0].User != "[USER_REDACTED]" {
		t.Fatalf("Processes[0].User = %q, want [USER_REDACTED]", got.Processes[0].User)
	}
}

func TestSanitizeStateDoesNotMutateInput(t *testing.T) {
	s := NewScrubber(true, true, true)
	original := "/home/alice/bin/agent (1%, 1MB)"
	state := models.SystemState{
		TopProcesses: original,
		Processes:    []models.Process{{Name: "/home/alice/x", User: "alice"}},
	}

	_ = s.SanitizeState(state)

	if state.TopProcesses != original {
		t.Fatalf("SanitizeState mutated its input: %q", state.TopProcesses)
	}
	if state.Processes[0].User != "alice" {
		t.Fatalf("SanitizeState mutated the caller's Processes slice: %q", state.Processes[0].User)
	}
}
