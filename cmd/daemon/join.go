package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"sys-sentient/internal/hostid"
	"sys-sentient/internal/version"
)

// joinTimeout bounds the enrolment request. Enrolment is interactive — someone
// is watching a terminal — so it fails fast rather than hanging.
const joinTimeout = 30 * time.Second

type joinPayload struct {
	Token    string `json:"token"`
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
}

type joinResult struct {
	AgentID  string `json:"agent_id"`
	AgentKey string `json:"agent_key"`
	Label    string `json:"label"`
}

// runJoin implements `sys-sentient agent join`.
//
// It exchanges a single-use token for this machine's own credential and writes
// a config file, so enrolling a host is one pasted command rather than hand-
// editing YAML and inventing a shared key.
func runJoin(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("agent join", flag.ContinueOnError)
	fs.SetOutput(stdout)
	var (
		serverURL  = fs.String("server", "", "base URL of the sys-sentient server (required)")
		token      = fs.String("token", "", "single-use join token from the server's Devices screen (required)")
		out        = fs.String("config", defaultAgentConfigPath(), "path to write the agent config to")
		caCert     = fs.String("ca-cert", "", "trust a private CA for the server connection")
		skipVerify = fs.Bool("insecure-skip-verify", false, "disable TLS verification (last resort)")
		force      = fs.Bool("force", false, "overwrite an existing config file")
	)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stdout, "Usage: sys-sentient agent join --server <url> --token <token>\n\n"+
			"Enrols this machine with a sys-sentient server. Get a token from the\n"+
			"server's Settings -> Devices screen.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *serverURL == "" || *token == "" {
		fs.Usage()
		return errors.New("both --server and --token are required")
	}

	base := strings.TrimSuffix(strings.TrimSpace(*serverURL), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return fmt.Errorf("--server must start with http:// or https://, got %q", base)
	}
	if strings.HasPrefix(base, "http://") && !*skipVerify {
		// The token and the credential it returns both cross this connection
		// in the clear. Worth a word rather than a silent downgrade.
		_, _ = fmt.Fprintf(stdout, "warning: %s is unencrypted; the join token and agent key will be sent in plain text\n", base)
	}

	// Refuse to clobber a working install by default: re-running the command
	// after a successful join should not silently orphan the old credential.
	if !*force {
		if _, err := os.Stat(*out); err == nil {
			return fmt.Errorf("%s already exists; pass --force to overwrite", *out)
		}
	}

	client, err := joinClient(*caCert, *skipVerify)
	if err != nil {
		return err
	}

	id, hostname := hostid.Resolve()
	body, err := json.Marshal(joinPayload{
		Token: *token, HostID: id, Hostname: hostname, Version: version.Get().Version,
	})
	if err != nil {
		return fmt.Errorf("encode join request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), joinTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/agents/join", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build join request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("contact server at %s: %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server refused enrolment: %s", describeJoinFailure(resp))
	}

	var result joinResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&result); err != nil {
		return fmt.Errorf("decode server response: %w", err)
	}
	if result.AgentKey == "" {
		return errors.New("server returned an empty agent key")
	}

	if err := writeAgentConfig(*out, base, result.AgentKey, *caCert, *skipVerify); err != nil {
		return err
	}

	name := result.Label
	if name == "" {
		name = hostname
	}
	_, _ = fmt.Fprintf(stdout, "Enrolled %q with %s\n", name, base)
	_, _ = fmt.Fprintf(stdout, "Config written to %s\n", *out)
	_, _ = fmt.Fprintf(stdout, "\nStart reporting with:\n  sys-sentient --config %s\n", *out)
	return nil
}

// describeJoinFailure turns a failed response into something an operator can
// act on. A bare status code sends people to the server logs unnecessarily.
func describeJoinFailure(resp *http.Response) string {
	var payload struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return "the join token is invalid or has expired — generate a new one"
	case http.StatusNotFound:
		return "no enrolment endpoint at this address — check --server, and that the server is up to date"
	default:
		return resp.Status
	}
}

func joinClient(caCertPath string, skipVerify bool) (*http.Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if skipVerify {
		tlsConfig.InsecureSkipVerify = true
	}
	if caCertPath != "" {
		// #nosec G304 -- caCertPath is the operator's own --ca-cert flag;
		// reading the file they named is what the flag is for.
		pem, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", caCertPath)
		}
		tlsConfig.RootCAs = pool
	}
	return &http.Client{
		Timeout:   joinTimeout,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}

// writeAgentConfig persists the credential this machine was issued.
//
// Written through viper so the file is guaranteed to round-trip through the
// same loader the daemon uses at boot — a hand-rolled YAML writer can emit
// something the reader then rejects.
func writeAgentConfig(path, serverURL, key, caCert string, skipVerify bool) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		// 0750: the directory holds this machine's agent credential.
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}
	}

	v := viper.New()
	v.Set("mode", "agent")
	v.Set("agent.server_url", serverURL)
	v.Set("agent.key", key)
	v.Set("agent.spool_path", filepath.Join(filepath.Dir(path), "spool.jsonl"))
	if caCert != "" {
		v.Set("agent.ca_cert_path", caCert)
	}
	if skipVerify {
		v.Set("agent.insecure_skip_verify", true)
	}

	// Create the file with 0600 before viper writes to it: the file holds a
	// credential that can write to the fleet's data, and WriteConfigAs would
	// otherwise create it 0644.
	// #nosec G304 -- path is the operator's own --config flag.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}

	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	// WriteConfigAs recreates the file, so re-assert the mode.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure %s: %w", path, err)
	}
	return nil
}

// defaultAgentConfigPath picks the conventional location for the platform.
func defaultAgentConfigPath() string {
	if os.Geteuid() == 0 {
		return "/etc/sys-sentient/agent.yaml"
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "sys-sentient", "agent.yaml")
	}
	return "agent.yaml"
}
