// Package service registers the daemon with the host's service manager, so an
// enrolled agent survives logout and reboot.
//
// `agent join` enrols a machine and prints a command to run; without this the
// operator has to write a unit file by hand, and an agent that only runs in
// somebody's shell is not monitoring anything the next morning.
package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Name is the service identifier on every platform.
const Name = "sys-sentient"

// ErrNotInstalled reports that no service is registered.
var ErrNotInstalled = errors.New("no sys-sentient service is installed")

// ErrExists reports that one already is.
var ErrExists = errors.New("a sys-sentient service is already installed")

// Config describes the service to register.
type Config struct {
	// ExecPath is the binary to run. Resolved to an absolute path, because a
	// service manager has no working directory to resolve a relative one
	// against and would fail at start time rather than at install time.
	ExecPath string
	// ConfigPath is passed as --config. Empty means the daemon's own default
	// discovery.
	ConfigPath string
	// User installs a per-user service instead of a system-wide one, which is
	// how an unprivileged operator persists an agent without root.
	User bool
	// Force replaces an existing definition.
	Force bool
}

// Status is what the service manager reports back.
type Status struct {
	Installed bool
	Running   bool
	// Detail carries the manager's own wording, which is more useful to an
	// operator than anything this package could paraphrase.
	Detail string
	// Path is where the definition lives, for an operator who wants to read it.
	Path string
}

// Resolve fills in defaults and validates the configuration.
func (c *Config) Resolve() error {
	if c.ExecPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate this binary: %w", err)
		}
		c.ExecPath = exe
	}

	abs, err := filepath.Abs(c.ExecPath)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", c.ExecPath, err)
	}
	// EvalSymlinks so a service installed from a symlinked path keeps working
	// after the symlink is repointed by a package upgrade.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return fmt.Errorf("%s is not an executable file", abs)
	}
	c.ExecPath = abs

	if c.ConfigPath != "" {
		configAbs, err := filepath.Abs(c.ConfigPath)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", c.ConfigPath, err)
		}
		if _, err := os.Stat(configAbs); err != nil {
			return fmt.Errorf("config file %s does not exist", configAbs)
		}
		c.ConfigPath = configAbs
	}
	return nil
}

// argsFor renders the command line the service manager should run.
func (c *Config) argsFor() []string {
	if c.ConfigPath == "" {
		return nil
	}
	return []string{"--config", c.ConfigPath}
}

// commandLine renders exec path and arguments as one string, for the platforms
// whose definitions take a single command rather than a list.
func (c *Config) commandLine() string {
	parts := append([]string{c.ExecPath}, c.argsFor()...)
	return strings.Join(parts, " ")
}
