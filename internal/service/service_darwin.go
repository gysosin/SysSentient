package service

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const label = "io.github.gysosin." + Name

func plistPath(user bool) (string, error) {
	if !user {
		return filepath.Join("/Library/LaunchDaemons", label+".plist"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

// renderPlist builds the property list.
//
// Every string is XML-escaped: a path containing & or < would otherwise
// produce a plist launchd silently refuses to load.
func renderPlist(cfg Config, logDir string) (string, error) {
	args := append([]string{cfg.ExecPath}, cfg.argsFor()...)

	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString("  <key>Label</key>\n  <string>" + escapeXML(label) + "</string>\n")
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	for _, a := range args {
		b.WriteString("    <string>" + escapeXML(a) + "</string>\n")
	}
	b.WriteString("  </array>\n")
	b.WriteString("  <key>RunAtLoad</key>\n  <true/>\n")
	b.WriteString("  <key>KeepAlive</key>\n  <true/>\n")
	b.WriteString("  <key>StandardOutPath</key>\n  <string>" + escapeXML(filepath.Join(logDir, Name+".log")) + "</string>\n")
	b.WriteString("  <key>StandardErrorPath</key>\n  <string>" + escapeXML(filepath.Join(logDir, Name+".err.log")) + "</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String(), nil
}

func escapeXML(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}

func logDirFor(user bool) (string, error) {
	if !user {
		return "/var/log", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Logs"), nil
}

func Install(cfg Config) (string, error) {
	if err := cfg.Resolve(); err != nil {
		return "", err
	}
	path, err := plistPath(cfg.User)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil && !cfg.Force {
		return "", fmt.Errorf("%w at %s; pass --force to replace it", ErrExists, path)
	}

	logDir, err := logDirFor(cfg.User)
	if err != nil {
		return "", err
	}
	plist, err := renderPlist(cfg, logDir)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	// bootstrap replaced `load` in Big Sur; fall back so this keeps working on
	// older systems rather than failing with an opaque launchctl error.
	if out, err := exec.Command("launchctl", "bootstrap", domain(cfg.User), path).CombinedOutput(); err != nil {
		if out2, err2 := exec.Command("launchctl", "load", "-w", path).CombinedOutput(); err2 != nil {
			return path, fmt.Errorf("launchctl: %w (%s / %s)", err, strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return path, nil
}

func domain(user bool) string {
	if user {
		return fmt.Sprintf("gui/%d", os.Getuid())
	}
	return "system"
}

func Start(cfg Config) error {
	if out, err := exec.Command("launchctl", "kickstart", "-k", domain(cfg.User)+"/"+label).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Uninstall(cfg Config) error {
	path, err := plistPath(cfg.User)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return ErrNotInstalled
	}

	// Best effort, so a service that is already unloaded can still be removed.
	_, _ = exec.Command("launchctl", "bootout", domain(cfg.User)+"/"+label).CombinedOutput()
	_, _ = exec.Command("launchctl", "unload", "-w", path).CombinedOutput()

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func Query(cfg Config) (Status, error) {
	path, err := plistPath(cfg.User)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Status{Installed: false, Path: path}, nil
	}

	out, err := exec.Command("launchctl", "print", domain(cfg.User)+"/"+label).CombinedOutput()
	running := err == nil && strings.Contains(string(out), "state = running")
	detail := "loaded"
	if err != nil {
		detail = "not loaded"
	} else if running {
		detail = "running"
	}
	return Status{Installed: true, Running: running, Detail: detail, Path: path}, nil
}
