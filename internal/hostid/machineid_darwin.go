//go:build darwin

package hostid

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// machineID reads the hardware IOPlatformUUID.
//
// Stable across reboots and OS reinstalls, and the value Apple's own tooling
// treats as the machine's identity. There is no file to read: it lives in the
// IORegistry, and ioreg is the supported way to reach it without cgo.
//
// Bounded by a timeout because this runs during start-up, and a wedged ioreg
// must not stop the daemon from booting.
func machineID() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Fixed command and arguments; nothing here is caller-controlled.
	out, err := exec.CommandContext(ctx, "ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output() // #nosec G204
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.Contains(line, "IOPlatformUUID") {
			continue
		}
		// Line shape: `    "IOPlatformUUID" = "ABCD-..."`
		_, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if id := strings.Trim(strings.TrimSpace(value), `"`); id != "" {
			return id
		}
	}
	return ""
}
