//go:build darwin

package logs

import (
	"context"
	"fmt"
)

// platformLogSources is the macOS set.
//
// The unified log replaced syslog in macOS 10.12; /var/log/system.log still
// exists on many systems but carries almost nothing. `log show` is the
// supported interface and needs no elevation for the default predicate.
var platformLogSources = []logSource{
	{heading: "MACOS UNIFIED LOG (Errors/Faults)", fetch: (*LogReader).getUnifiedLog},
}

// getUnifiedLog reads recent error and fault entries.
//
// `--last 10m` bounds the scan: the unified log is enormous and an unbounded
// `log show` can take tens of seconds and a great deal of memory, which is not
// something a monitoring agent may do to the machine it is monitoring.
func (r *LogReader) getUnifiedLog(ctx context.Context) (string, error) {
	output, err := r.runLogCommand(ctx, "log", "show",
		"--last", "10m",
		"--predicate", `messageType == "Error" OR messageType == "Fault"`,
		"--style", "compact",
		"--no-pager",
	)
	if err != nil {
		return "", fmt.Errorf("log show failed: %w", err)
	}
	return r.filterRelevantLines(r.getTailLines(string(output), r.maxLines)), nil
}
