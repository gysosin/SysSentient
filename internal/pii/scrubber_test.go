package pii

import (
	"testing"
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
