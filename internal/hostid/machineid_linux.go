//go:build linux

package hostid

import (
	"os"
	"strings"
)

// sources are read in order; the first usable one wins.
//
//	/etc/machine-id          — systemd, stable across reboots and renames
//	/var/lib/dbus/machine-id — older systems without systemd
var sources = []string{
	"/etc/machine-id",
	"/var/lib/dbus/machine-id",
}

func machineID() string {
	for _, path := range sources {
		// `sources` is a fixed package-level allowlist, not caller input.
		raw, err := os.ReadFile(path) // #nosec G304 -- fixed allowlist above
		if err != nil {
			continue
		}
		if id := strings.TrimSpace(string(raw)); id != "" {
			return id
		}
	}
	return ""
}
