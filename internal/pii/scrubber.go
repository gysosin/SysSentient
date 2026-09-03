package pii

import (
	"regexp"

	"sys-sentient/internal/models"
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

const redactedUser = "[USER_REDACTED]"

var (
	// Basic IPv4 regex
	ipRegex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// IPv6, including compressed (::) and link-local forms. Anchored on a
	// hex-or-colon boundary so ordinary "key: value" text and timestamps such
	// as "00:25:21" are left alone — those have no hex group adjacent to a
	// double colon and never reach three colon-separated groups.
	ipv6Regex = regexp.MustCompile(
		`(?i)\b(?:[0-9a-f]{1,4}:){7}[0-9a-f]{1,4}\b` + // full 8-group form
			`|(?:[0-9a-f]{1,4}:){1,7}:(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?` + // compressed
			`|::(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,7})?`, // leading ::
	)
	// Basic Email regex
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	// Simple user home path regex: /home/xyz -> /home/[USER_REDACTED]
	// Careful not to match /home/ itself without a user, but typically paths are /home/user/...
	// /Users/ and /root are covered too: macOS agents and root-owned services
	// leak identity through the same shape.
	userPathRegex = regexp.MustCompile(`(/home/|/Users/|/var/home/)([^/\s]+)`)
)

func (s *Scrubber) SanitizeLog(input string) string {
	if s.maskIPs {
		// IPv6 first: the IPv4 pattern would otherwise chew the trailing dotted
		// quad out of an IPv4-mapped address such as ::ffff:192.0.2.1 and leave
		// the hex prefix behind.
		input = ipv6Regex.ReplaceAllString(input, "[IP_REDACTED]")
		input = ipRegex.ReplaceAllString(input, "[IP_REDACTED]")
	}
	if s.maskEmails {
		input = emailRegex.ReplaceAllString(input, "[EMAIL_REDACTED]")
	}
	if s.maskUsernames {
		input = userPathRegex.ReplaceAllString(input, "${1}"+redactedUser)
	}
	return input
}

// SanitizeState returns a copy of state with the operator-visible strings
// scrubbed. Metric numbers are untouched.
//
// This matters because TopProcesses and process names are interpolated directly
// into the Gemini prompt. Only logs used to be scrubbed, so process names —
// which routinely carry home paths, hostnames and argv-derived secrets — were
// sent to Google verbatim.
//
// The returned value never aliases the caller's Processes slice.
func (s *Scrubber) SanitizeState(state models.SystemState) models.SystemState {
	sanitized := state
	sanitized.TopProcesses = s.SanitizeLog(state.TopProcesses)

	if state.Processes == nil {
		return sanitized
	}

	processes := make([]models.Process, len(state.Processes))
	copy(processes, state.Processes)
	for i := range processes {
		processes[i].Name = s.SanitizeLog(processes[i].Name)
		if s.maskUsernames {
			processes[i].User = redactedUser
		}
	}
	sanitized.Processes = processes

	return sanitized
}
