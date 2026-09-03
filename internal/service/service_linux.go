package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// unitTemplate is deliberately close to the packaged unit, minus the hardening
// that assumes a dedicated system account. A user unit cannot use User=/Group=
// and most of the Protect* directives are either meaningless or actively break
// a service running as an ordinary login account.
const unitTemplate = `[Unit]
Description=SysSentient monitoring agent
Documentation=https://github.com/gysosin/SysSentient
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5
# An agent that cannot reach its server restarts on a backoff rather than
# hammering it; the on-disk spool already holds the samples meanwhile.
NoNewPrivileges=true
%s%s
[Install]
WantedBy=%s
`

// systemHardening is applied only to system units, where the service runs as
// its own account and these directives are both meaningful and safe.
const systemHardening = `ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%s
LockPersonality=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native
UMask=0077
`

func unitDir(user bool) (string, error) {
	if !user {
		return "/etc/systemd/system", nil
	}
	// XDG location, so it works without root and is picked up by the user's
	// own systemd instance.
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user"), nil
}

func unitPath(user bool) (string, error) {
	dir, err := unitDir(user)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, Name+".service"), nil
}

// systemctl runs systemctl against the right instance.
func systemctl(user bool, args ...string) (string, error) {
	full := args
	if user {
		full = append([]string{"--user"}, args...)
	}
	out, err := exec.Command("systemctl", full...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Install writes the unit and enables it.
func Install(cfg Config) (string, error) {
	if err := cfg.Resolve(); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "", errors.New("systemd is not available on this machine; install a service manually")
	}

	path, err := unitPath(cfg.User)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil && !cfg.Force {
		return "", fmt.Errorf("%w at %s; pass --force to replace it", ErrExists, path)
	}

	// PrivateTmp gives the service its own /tmp, which hides any path under
	// /tmp from the service itself. The failure is a 203/EXEC "No such file or
	// directory" for a file that plainly exists -- opaque enough to be worth
	// avoiding rather than documenting.
	privateTmp := "PrivateTmp=true\n"
	if underTmp(cfg.ExecPath) || underTmp(cfg.ConfigPath) {
		privateTmp = "# PrivateTmp omitted: a path used by this service lives under /tmp,\n" +
			"# which a private /tmp would hide from the service itself.\n"
	}

	hardening := ""
	if !cfg.User {
		// The daemon needs to write its database; without this ProtectSystem
		// makes the whole filesystem read-only and the service dies on first
		// write with a message that does not obviously point here.
		writable := "/var/lib/sys-sentient"
		if cfg.ConfigPath != "" {
			writable = filepath.Dir(cfg.ConfigPath)
		}
		hardening = fmt.Sprintf(systemHardening, writable)
	}

	target := "multi-user.target"
	if cfg.User {
		target = "default.target"
	}

	unit := fmt.Sprintf(unitTemplate, cfg.commandLine(), privateTmp, hardening, target)

	// A user unit is read by that user's own systemd instance and by nobody
	// else, so it is kept private. A system unit follows the convention every
	// distribution and this project's own package use: world-readable, because
	// operators expect `systemctl cat` to work unprivileged and the file holds
	// no secret -- only a path.
	dirMode, fileMode := os.FileMode(0o750), os.FileMode(0o644)
	if cfg.User {
		dirMode, fileMode = 0o700, 0o600
	}

	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return "", fmt.Errorf("create unit directory: %w", err)
	}
	// #nosec G306 -- see the comment above: a system unit is deliberately
	// world-readable, matching every distribution and the packaged unit.
	if err := os.WriteFile(path, []byte(unit), fileMode); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	if out, err := systemctl(cfg.User, "daemon-reload"); err != nil {
		return path, fmt.Errorf("systemctl daemon-reload: %w (%s)", err, out)
	}
	if out, err := systemctl(cfg.User, "enable", Name); err != nil {
		return path, fmt.Errorf("systemctl enable: %w (%s)", err, out)
	}
	return path, nil
}

// Start launches the service now.
func Start(cfg Config) error {
	if out, err := systemctl(cfg.User, "start", Name); err != nil {
		return fmt.Errorf("systemctl start: %w (%s)", err, out)
	}
	return nil
}

// Uninstall stops, disables and removes the unit.
func Uninstall(cfg Config) error {
	path, err := unitPath(cfg.User)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return ErrNotInstalled
	}

	// Best effort: a unit that is already stopped or was never enabled must not
	// stop the removal, or a half-installed service can never be cleaned up.
	_, _ = systemctl(cfg.User, "stop", Name)
	_, _ = systemctl(cfg.User, "disable", Name)

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if out, err := systemctl(cfg.User, "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w (%s)", err, out)
	}
	return nil
}

// Query reports what the service manager knows.
func Query(cfg Config) (Status, error) {
	path, err := unitPath(cfg.User)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Status{Installed: false, Path: path}, nil
	}

	// ActiveState alone is misleading: a service crash-looping under
	// Restart=on-failure sits in "activating" indefinitely, which reads as
	// "starting up" when it is actually broken. SubState and NRestarts are
	// what distinguish the two.
	raw, _ := systemctl(cfg.User, "show", Name,
		"-p", "ActiveState", "-p", "SubState", "-p", "Result", "-p", "NRestarts")
	props := parseShow(raw)

	active := props["ActiveState"]
	sub := props["SubState"]
	// "active (running)" is the healthy case and needs no elaboration; the
	// substate only earns a mention when it differs from the state.
	detail := active
	if sub != "" && sub != active && (active != "active" || sub != "running") {
		detail = active + " (" + sub + ")"
	}

	restarts := props["NRestarts"]
	if active == "failed" || props["Result"] == "exit-code" {
		detail = "failed: " + props["Result"]
		if restarts != "" && restarts != "0" {
			detail += ", " + restarts + " restarts"
		}
		detail += " -- check the logs: " + logHint(cfg.User)
	} else if active == "activating" && restarts != "" && restarts != "0" {
		// Restarting repeatedly is a crash loop, not a slow start.
		detail = "restarting repeatedly (" + restarts + " restarts) -- check the logs: " + logHint(cfg.User)
	}

	return Status{
		Installed: true,
		Running:   active == "active",
		Detail:    detail,
		Path:      path,
	}, nil
}

func logHint(user bool) string {
	if user {
		return "journalctl --user -u " + Name + " -n 20"
	}
	return "journalctl -u " + Name + " -n 20"
}

// parseShow reads systemctl's Key=Value output.
func parseShow(raw string) map[string]string {
	props := make(map[string]string, 4)
	for _, line := range strings.Split(raw, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			props[key] = value
		}
	}
	return props
}

// underTmp reports whether a path lives in a directory a private /tmp replaces.
func underTmp(path string) bool {
	if path == "" {
		return false
	}
	return strings.HasPrefix(path, "/tmp/") || strings.HasPrefix(path, "/var/tmp/")
}
