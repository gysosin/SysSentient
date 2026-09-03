//go:build windows

package hostid

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// machineID reads the installation's MachineGuid.
//
// Written by Windows at install time and stable across reboots, renames and
// hardware changes — the closest equivalent to /etc/machine-id. Read via the
// registry API rather than by shelling out to reg.exe: a monitoring agent
// should not spawn a process to answer a question asked once per start-up.
//
// Opened with the 64-bit view explicitly, because a 32-bit build would
// otherwise be redirected to Wow6432Node, where the value does not exist.
func machineID() string {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	)
	if err != nil {
		return ""
	}
	defer func() { _ = key.Close() }()

	guid, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(guid)
}
