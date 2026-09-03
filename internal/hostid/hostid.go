// Package hostid derives a stable identifier for the machine a sample came
// from.
//
// Hostnames alone are not usable as a fleet key: they collide (a dozen hosts
// called "localhost"), they change when a machine is renamed, and DHCP or
// cloud-init can reassign them. A stable ID lets a renamed host keep its
// history instead of appearing as a new machine.
package hostid

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

// Sources are read in order; the first usable one wins.
//
//	/etc/machine-id      — systemd, stable across reboots and renames
//	/var/lib/dbus/machine-id — older systems without systemd
//	/proc/sys/kernel/random/boot_id — last resort, changes per boot
var sources = []string{
	"/etc/machine-id",
	"/var/lib/dbus/machine-id",
}

// Resolve returns a stable host identifier and the hostname.
//
// The machine-id is hashed rather than returned verbatim: on systemd hosts it
// is also used to derive other secrets, and D-Bus documentation explicitly
// warns against exposing it directly. Hashing keeps it stable and unique while
// making it useless to anyone who intercepts a metrics payload.
func Resolve() (id string, hostname string) {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}

	for _, path := range sources {
		// `sources` is a fixed package-level allowlist, not caller input.
		raw, err := os.ReadFile(path) // #nosec G304 -- fixed allowlist above
		if err != nil {
			continue
		}
		machineID := strings.TrimSpace(string(raw))
		if machineID == "" {
			continue
		}
		return derive(machineID), hostname
	}

	// No machine-id available (some containers, non-systemd BSDs). Fall back to
	// the hostname so the value is at least stable for this host, and accept
	// that renaming it starts a new history.
	return derive("hostname:" + hostname), hostname
}

// derive hashes a raw identity into a fixed-width, non-reversible ID.
func derive(raw string) string {
	sum := sha256.Sum256([]byte("sys-sentient-host-v1:" + raw))
	return hex.EncodeToString(sum[:])[:32]
}
