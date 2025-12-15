package pii

import (
	"regexp"
)

type Scrubber struct {
	maskIPs       bool
	maskEmails    bool
	maskUsernames bool
}

func NewScrubber(maskIPs, maskEmails, maskUsernames bool) *Scrubber {
	return &Scrubber{
		maskIPs:       maskIPs,
		maskEmails:    maskEmails,
		maskUsernames: maskUsernames,
	}
}

var (
	// Basic IPv4 regex
	ipRegex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// Basic Email regex
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	// Simple user home path regex: /home/xyz -> /home/[USER_REDACTED]
	// Careful not to match /home/ itself without a user, but typically paths are /home/user/...
	userPathRegex = regexp.MustCompile(`(/home/)([^/\s]+)`)
)

func (s *Scrubber) SanitizeLog(input string) string {
	if s.maskIPs {
		input = ipRegex.ReplaceAllString(input, "[IP_REDACTED]")
	}
	if s.maskEmails {
		input = emailRegex.ReplaceAllString(input, "[EMAIL_REDACTED]")
	}
	if s.maskUsernames {
		input = userPathRegex.ReplaceAllString(input, "${1}[USER_REDACTED]")
	}
	return input
}
