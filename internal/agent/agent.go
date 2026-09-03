package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"sys-sentient/internal/models"
)

// Client pushes collected samples to a sys-sentient server.
type Client struct {
	serverURL    string
	key          string
	agentVersion string
	http         *http.Client
	spool        *Spool
	batchSize    int
	logger       *slog.Logger
}

// Options configures the push client.
type Options struct {
	ServerURL    string
	Key          string
	AgentVersion string
	SpoolPath    string
	BatchSize    int
	// CACertPath trusts a private CA. This is the correct way to run against
	// an internal certificate — prefer it over InsecureSkipVerify.
	CACertPath string
	// InsecureSkipVerify disables certificate verification entirely, which
	// makes the connection trivially interceptable. Last resort only.
	InsecureSkipVerify bool
	Logger             *slog.Logger
}

const (
	pushTimeout   = 30 * time.Second
	spoolCapacity = 5000
)

// New builds a push client with a durable spool.
func New(opts Options) (*Client, error) {
	if strings.TrimSpace(opts.ServerURL) == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if opts.BatchSize < 1 {
		opts.BatchSize = 60
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	spool, err := NewSpool(opts.SpoolPath, spoolCapacity)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Preferred path for internal certificates: trust a private CA rather than
	// disabling verification.
	if path := strings.TrimSpace(opts.CACertPath); path != "" {
		// Path comes from the operator's own config file, the same trust level
		// as the server URL and agent key alongside it.
		pem, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
		if err != nil {
			return nil, fmt.Errorf("read CA certificate %q: %w", path, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %q", path)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		opts.Logger.Info("trusting private CA for server connections", "path", path)
	}

	if opts.InsecureSkipVerify {
		// Explicit operator opt-in. Warned about on every start because it
		// removes the only protection against an intercepted agent key.
		opts.Logger.Warn("TLS certificate verification is DISABLED for the server connection; " +
			"the agent key and all metrics can be intercepted. " +
			"Set agent.ca_cert_path to trust a private CA instead.")
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{} // #nosec G402 -- operator opt-in, warned above
		}
		transport.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- operator opt-in, warned above
	}

	return &Client{
		serverURL:    strings.TrimRight(opts.ServerURL, "/"),
		key:          opts.Key,
		agentVersion: opts.AgentVersion,
		http:         &http.Client{Timeout: pushTimeout, Transport: transport},
		spool:        spool,
		batchSize:    opts.BatchSize,
		logger:       opts.Logger,
	}, nil
}

// Enqueue buffers a sample for delivery.
func (c *Client) Enqueue(sample models.SystemState) error {
	return c.spool.Append(sample)
}

// Flush attempts to deliver buffered samples.
//
// Samples are removed from the spool only after the server acknowledges them,
// so a failed or partial send leaves the buffer intact and the next attempt
// retries the same batch.
func (c *Client) Flush(ctx context.Context) error {
	batch, err := c.spool.Peek(c.batchSize)
	if err != nil {
		return fmt.Errorf("read spool: %w", err)
	}
	if len(batch) == 0 {
		return nil
	}

	accepted, err := c.push(ctx, batch)
	if err != nil {
		return err
	}

	// Rejected samples (bad clock, missing host id) would otherwise be retried
	// forever and block everything behind them, so the whole batch is retired.
	if err := c.spool.Commit(len(batch)); err != nil {
		return fmt.Errorf("commit spool: %w", err)
	}

	c.logger.Debug("pushed samples", "sent", len(batch), "accepted", accepted)
	return nil
}

// Pending reports how many samples are waiting.
func (c *Client) Pending() int {
	n, err := c.spool.Len()
	if err != nil {
		return 0
	}
	return n
}

func (c *Client) push(ctx context.Context, samples []models.SystemState) (int, error) {
	payload := struct {
		AgentVersion string               `json:"agent_version"`
		Samples      []models.SystemState `json:"samples"`
	}{AgentVersion: c.agentVersion, Samples: samples}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/api/ingest", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("X-API-Key", c.key)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("push: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return 0, fmt.Errorf("server rejected the agent key (status %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Accepted int `json:"accepted"`
		Rejected int `json:"rejected"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// A 2xx with an unreadable body still means the server took it.
		return len(samples), nil
	}
	if result.Rejected > 0 {
		c.logger.Warn("server rejected some samples", "rejected", result.Rejected, "accepted", result.Accepted)
	}
	return result.Accepted, nil
}
