//go:build linux

package logs

import (
	"context"
	"fmt"
)

// platformLogSources is the Linux set.
//
// dmesg is listed even though the shipped systemd unit cannot read it —
// ProtectKernelLogs=true plus an empty CapabilityBoundingSet make that
// impossible by design. It still works when the daemon is run directly, and
// failing soft costs nothing.
var platformLogSources = []logSource{
	{heading: "SYSTEMD JOURNAL (Errors/Warnings)", fetch: (*LogReader).getJournalErrors},
	{heading: "KERNEL MESSAGES (Errors/Warnings)", fetch: (*LogReader).getDmesgErrors},
	{heading: "SYSLOG (Recent Errors)", fetch: (*LogReader).getSyslogErrors},
}

// getJournalErrors gets recent error/warning entries from journalctl
func (r *LogReader) getJournalErrors(ctx context.Context) (string, error) {
	// Get logs from last 10 minutes with priority error or warning
	output, err := r.runLogCommand(ctx, "journalctl", "--since", "10 minutes ago", "-p", "warning", "--no-pager", "-n", fmt.Sprintf("%d", r.maxLines))
	if err != nil {
		// journalctl might not be available or user lacks permissions
		return "", fmt.Errorf("journalctl failed: %w", err)
	}

	return r.filterRelevantLines(string(output)), nil
}

// getDmesgErrors gets recent kernel errors from dmesg
func (r *LogReader) getDmesgErrors(ctx context.Context) (string, error) {
	output, err := r.runLogCommand(ctx, "dmesg", "-l", "err,warn", "-T")
	if err != nil {
		// dmesg might require root or not available
		return "", fmt.Errorf("dmesg failed: %w", err)
	}

	lines := r.getTailLines(string(output), r.maxLines)
	return r.filterRelevantLines(lines), nil
}

// getSyslogErrors reads recent errors from /var/log/syslog
func (r *LogReader) getSyslogErrors(ctx context.Context) (string, error) {
	// Try to read syslog with tail
	output, err := r.runLogCommand(ctx, "tail", "-n", fmt.Sprintf("%d", r.maxLines), "/var/log/syslog")
	if err != nil {
		// syslog might not exist or permission denied
		return "", fmt.Errorf("syslog read failed: %w", err)
	}

	// Filter for error/warning keywords
	return r.filterErrorLines(string(output)), nil
}
