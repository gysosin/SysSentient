//go:build windows

package logs

import (
	"context"
	"fmt"
)

// platformLogSources is the Windows set.
//
// wevtutil ships with every supported Windows version, which PowerShell's
// Get-WinEvent effectively does too but at a much higher start-up cost — a
// PowerShell process per poll is not acceptable on a machine this is supposed
// to be monitoring unobtrusively.
//
// The System and Application logs are the two that carry the failures an
// operator cares about; Security is deliberately excluded, since it is high
// volume, usually requires additional privileges, and its contents are exactly
// what the PII scrubber exists to keep out of an AI prompt.
var platformLogSources = []logSource{
	{heading: "WINDOWS SYSTEM LOG (Errors/Warnings)", fetch: (*LogReader).getWindowsSystemLog},
	{heading: "WINDOWS APPLICATION LOG (Errors/Warnings)", fetch: (*LogReader).getWindowsApplicationLog},
}

func (r *LogReader) getWindowsSystemLog(ctx context.Context) (string, error) {
	return r.queryEventLog(ctx, "System")
}

func (r *LogReader) getWindowsApplicationLog(ctx context.Context) (string, error) {
	return r.queryEventLog(ctx, "Application")
}

// queryEventLog reads recent Error (Level=2) and Warning (Level=3) events.
//
// The XPath filter is applied by the event service rather than by us, so a
// noisy machine does not stream its whole log through this process just to
// have most of it discarded.
func (r *LogReader) queryEventLog(ctx context.Context, channel string) (string, error) {
	query := "*[System[(Level=2 or Level=3) and TimeCreated[timediff(@SystemTime) <= 600000]]]"
	output, err := r.runLogCommand(ctx, "wevtutil", "qe", channel,
		"/q:"+query,
		"/f:text",
		"/rd:true", // newest first
		fmt.Sprintf("/c:%d", r.maxLines),
	)
	if err != nil {
		return "", fmt.Errorf("wevtutil %s failed: %w", channel, err)
	}
	return r.filterRelevantLines(string(output)), nil
}
