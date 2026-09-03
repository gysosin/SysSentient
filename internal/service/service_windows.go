package service

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// displayName is what appears in services.msc.
const displayName = "SysSentient Monitoring Agent"

func connect() (*mgr.Mgr, error) {
	m, err := mgr.Connect()
	if err != nil {
		// The SCM refuses non-elevated connections, and the raw error does not
		// say so in a way an operator would recognise.
		return nil, fmt.Errorf("connect to the service manager (run this from an elevated prompt): %w", err)
	}
	return m, nil
}

func Install(cfg Config) (string, error) {
	if err := cfg.Resolve(); err != nil {
		return "", err
	}
	if cfg.User {
		// Windows has no per-user service equivalent worth emulating here.
		return "", errors.New("per-user services are not supported on Windows; install from an elevated prompt")
	}

	m, err := connect()
	if err != nil {
		return "", err
	}
	defer func() { _ = m.Disconnect() }()

	if existing, err := m.OpenService(Name); err == nil {
		defer func() { _ = existing.Close() }()
		if !cfg.Force {
			return "", fmt.Errorf("%w; pass --force to replace it", ErrExists)
		}
		if err := removeService(existing); err != nil {
			return "", err
		}
		// The SCM keeps a deleted service until every handle closes, so a
		// recreate immediately afterwards fails with "marked for deletion".
		_ = existing.Close()
		time.Sleep(time.Second)
	}

	s, err := m.CreateService(Name, cfg.ExecPath, mgr.Config{
		DisplayName:  displayName,
		Description:  "Collects system metrics and reports them to a SysSentient server.",
		StartType:    mgr.StartAutomatic,
		Dependencies: []string{"Tcpip"},
	}, cfg.argsFor()...)
	if err != nil {
		return "", fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = s.Close() }()

	return Name, nil
}

func removeService(s *mgr.Service) error {
	status, err := s.Query()
	if err == nil && status.State != svc.Stopped {
		if _, err := s.Control(svc.Stop); err != nil {
			return fmt.Errorf("stop service: %w", err)
		}
		// Give the SCM a moment; a delete against a running service fails.
		for range 20 {
			time.Sleep(250 * time.Millisecond)
			if st, err := s.Query(); err == nil && st.State == svc.Stopped {
				break
			}
		}
	}
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

func Start(cfg Config) error {
	m, err := connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(Name)
	if err != nil {
		return ErrNotInstalled
	}
	defer func() { _ = s.Close() }()

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

func Uninstall(cfg Config) error {
	m, err := connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(Name)
	if err != nil {
		return ErrNotInstalled
	}
	defer func() { _ = s.Close() }()

	return removeService(s)
}

func Query(cfg Config) (Status, error) {
	m, err := connect()
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(Name)
	if err != nil {
		return Status{Installed: false, Path: Name}, nil
	}
	defer func() { _ = s.Close() }()

	status, err := s.Query()
	if err != nil {
		return Status{Installed: true, Path: Name, Detail: "unknown"}, nil
	}

	detail := "stopped"
	if status.State == svc.Running {
		detail = "running"
	}
	return Status{
		Installed: true,
		Running:   status.State == svc.Running,
		Detail:    detail,
		Path:      Name,
	}, nil
}
