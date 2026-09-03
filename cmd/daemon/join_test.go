package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// joinServer stands in for a real server's enrolment endpoint.
func joinServer(t *testing.T, status int, body any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agents/join" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestJoinWritesLoadableConfig(t *testing.T) {
	srv := joinServer(t, http.StatusCreated, joinResult{
		AgentID: "agent-1", AgentKey: "issued-key", Label: "web-01",
	})
	out := filepath.Join(t.TempDir(), "agent.yaml")

	var stdout bytes.Buffer
	err := runJoin([]string{"--server", srv.URL, "--token", "tok", "--config", out}, &stdout)
	if err != nil {
		t.Fatalf("runJoin: %v", err)
	}

	// The written file must load through the same reader the daemon uses.
	v := viper.New()
	v.SetConfigFile(out)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("written config does not parse: %v", err)
	}
	if got := v.GetString("mode"); got != "agent" {
		t.Errorf("mode = %q, want agent", got)
	}
	if got := v.GetString("agent.key"); got != "issued-key" {
		t.Errorf("agent.key = %q, want issued-key", got)
	}
	if got := v.GetString("agent.server_url"); got != srv.URL {
		t.Errorf("agent.server_url = %q, want %q", got, srv.URL)
	}

	if !strings.Contains(stdout.String(), "web-01") {
		t.Errorf("output does not name the enrolled device: %q", stdout.String())
	}
}

func TestJoinWritesCredentialFilePrivate(t *testing.T) {
	srv := joinServer(t, http.StatusCreated, joinResult{AgentID: "a", AgentKey: "k"})
	out := filepath.Join(t.TempDir(), "agent.yaml")

	if err := runJoin([]string{"--server", srv.URL, "--token", "t", "--config", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runJoin: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// The file holds a credential that can write to the fleet's data.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config mode = %04o, want 0600", perm)
	}
}

func TestJoinRefusesToClobberExistingConfig(t *testing.T) {
	srv := joinServer(t, http.StatusCreated, joinResult{AgentID: "a", AgentKey: "k"})
	out := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(out, []byte("mode: agent\n"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	err := runJoin([]string{"--server", srv.URL, "--token", "t", "--config", out}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runJoin overwrote an existing config without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error does not mention the escape hatch: %v", err)
	}

	// ...and --force gets through.
	if err := runJoin([]string{"--server", srv.URL, "--token", "t", "--config", out, "--force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runJoin --force: %v", err)
	}
}

func TestJoinReportsServerRefusalReadably(t *testing.T) {
	srv := joinServer(t, http.StatusUnauthorized,
		map[string]string{"error": "join token is invalid or has expired"})

	err := runJoin([]string{"--server", srv.URL, "--token", "stale",
		"--config", filepath.Join(t.TempDir(), "a.yaml")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runJoin succeeded against a 401")
	}
	// The operator needs to know to mint a new token, not read server logs.
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error is not actionable: %v", err)
	}
}

func TestJoinRequiresBothFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--server", "https://x"},
		{"--token", "t"},
		{},
	} {
		if err := runJoin(args, &bytes.Buffer{}); err == nil {
			t.Errorf("runJoin(%v) succeeded with a missing flag", args)
		}
	}
}

func TestJoinRejectsSchemelessServer(t *testing.T) {
	err := runJoin([]string{"--server", "monitor.example.com", "--token", "t",
		"--config", filepath.Join(t.TempDir(), "a.yaml")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runJoin accepted a URL with no scheme")
	}
}

func TestJoinWarnsOnPlaintextTransport(t *testing.T) {
	srv := joinServer(t, http.StatusCreated, joinResult{AgentID: "a", AgentKey: "k"})
	var stdout bytes.Buffer
	if err := runJoin([]string{"--server", srv.URL, "--token", "t",
		"--config", filepath.Join(t.TempDir(), "a.yaml")}, &stdout); err != nil {
		t.Fatalf("runJoin: %v", err)
	}
	// httptest.NewServer is http://, so the warning must appear.
	if !strings.Contains(stdout.String(), "unencrypted") {
		t.Errorf("no plaintext warning in output: %q", stdout.String())
	}
}

func TestJoinRejectsEmptyKeyFromServer(t *testing.T) {
	srv := joinServer(t, http.StatusCreated, joinResult{AgentID: "a", AgentKey: ""})
	out := filepath.Join(t.TempDir(), "agent.yaml")

	if err := runJoin([]string{"--server", srv.URL, "--token", "t", "--config", out}, &bytes.Buffer{}); err == nil {
		t.Fatal("runJoin accepted an empty agent key")
	}
	// Nothing should have been written on a failed enrolment.
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("a config was written despite enrolment failing")
	}
}
