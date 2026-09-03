package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"sys-sentient/internal/service"
)

// runService implements `sys-sentient service <install|uninstall|status>`.
//
// Enrolment writes a config; this makes the daemon survive logout and reboot.
// It is separate from `agent join` because registering a service writes to
// /etc/systemd/system or the Windows service manager and usually needs root,
// which enrolment itself does not.
func runService(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		serviceUsage(stdout)
		return errors.New("expected install, uninstall or status")
	}

	action := args[0]
	fs := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	fs.SetOutput(stdout)
	var (
		configPath = fs.String("config", "", "config file the service should run with")
		userScope  = fs.Bool("user", false, "install for the current user only, which needs no root")
		force      = fs.Bool("force", false, "replace an existing service definition")
		start      = fs.Bool("start", true, "start the service once it is installed")
	)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg := service.Config{ConfigPath: *configPath, User: *userScope, Force: *force}

	switch action {
	case "install":
		path, err := service.Install(cfg)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Service installed: %s\n", path)

		if *start {
			if err := service.Start(cfg); err != nil {
				// The definition is written; failing to start is worth
				// reporting but does not mean the install failed.
				_, _ = fmt.Fprintf(stdout, "warning: could not start it: %v\n", err)
			} else {
				_, _ = fmt.Fprintln(stdout, "Started.")
			}
		}
		_, _ = fmt.Fprintf(stdout, "\nCheck it with:\n  sys-sentient service status%s\n", scopeFlag(*userScope))
		return nil

	case "uninstall":
		if err := service.Uninstall(cfg); err != nil {
			if errors.Is(err, service.ErrNotInstalled) {
				_, _ = fmt.Fprintln(stdout, "No service is installed; nothing to remove.")
				return nil
			}
			return err
		}
		_, _ = fmt.Fprintln(stdout, "Service removed.")
		return nil

	case "status":
		status, err := service.Query(cfg)
		if err != nil {
			return err
		}
		if !status.Installed {
			_, _ = fmt.Fprintf(stdout, "Not installed.\n\nInstall it with:\n  sys-sentient service install%s\n",
				scopeFlag(*userScope))
			return nil
		}
		state := "stopped"
		if status.Running {
			state = "running"
		}
		_, _ = fmt.Fprintf(stdout, "Installed: %s\nState:     %s (%s)\n", status.Path, state, status.Detail)
		return nil

	default:
		serviceUsage(stdout)
		return fmt.Errorf("unknown action %q", action)
	}
}

func scopeFlag(user bool) string {
	if user {
		return " --user"
	}
	return ""
}

func serviceUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: sys-sentient service <install|uninstall|status> [flags]

Registers the daemon with this machine's service manager so it survives
logout and reboot: systemd on Linux, launchd on macOS, the service manager
on Windows.

  install      write the service definition and start it
  uninstall    stop and remove it
  status       report whether it is installed and running

Flags:
  --config <path>   config file the service should run with
  --user            install for the current user only, which needs no root
  --force           replace an existing definition
  --start=false     install without starting

Examples:
  sudo sys-sentient service install --config /etc/sys-sentient/agent.yaml
  sys-sentient service install --user --config ~/.config/sys-sentient/agent.yaml
`)
}
