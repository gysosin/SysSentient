package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Notifier delivers one alert transition to an external destination.
type Notifier interface {
	Name() string
	Notify(ctx context.Context, alert Alert) error
}

const notifyTimeout = 10 * time.Second

// Dispatcher fans a transition out to every configured channel.
//
// Delivery is best-effort and never blocks the collector loop: a wedged Slack
// endpoint must not stop metrics collection on the host it is monitoring.
type Dispatcher struct {
	notifiers []Notifier
	logger    *slog.Logger
}

func NewDispatcher(logger *slog.Logger, notifiers ...Notifier) *Dispatcher {
	return &Dispatcher{notifiers: notifiers, logger: logger}
}

// Enabled reports whether any channel is configured. Used to warn operators
// that alerts are being evaluated but will reach nobody.
func (d *Dispatcher) Enabled() bool {
	return d != nil && len(d.notifiers) > 0
}

// Dispatch delivers the given transitions. Acknowledged alerts are skipped:
// acknowledging is how an operator says "I know, stop paging me".
func (d *Dispatcher) Dispatch(ctx context.Context, alerts []Alert) {
	if d == nil || len(d.notifiers) == 0 {
		return
	}

	for _, alert := range alerts {
		if alert.Acknowledged {
			continue
		}
		for _, n := range d.notifiers {
			ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
			err := n.Notify(ctx, alert)
			cancel()
			if err != nil {
				d.logger.Error("alert notification failed",
					"channel", n.Name(), "rule", alert.RuleID, "error", err)
				continue
			}
			d.logger.Info("alert notification sent",
				"channel", n.Name(), "rule", alert.RuleID, "state", string(alert.State))
		}
	}
}

// WebhookNotifier POSTs the alert as JSON.
type WebhookNotifier struct {
	URL    string
	Client *http.Client
}

func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{URL: url, Client: &http.Client{Timeout: notifyTimeout}}
}

func (w *WebhookNotifier) Name() string { return "webhook" }

func (w *WebhookNotifier) Notify(ctx context.Context, alert Alert) error {
	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshal alert: %w", err)
	}
	return post(ctx, w.Client, w.URL, body)
}

// SlackNotifier posts to a Slack incoming-webhook URL.
type SlackNotifier struct {
	URL    string
	Client *http.Client
}

func NewSlackNotifier(url string) *SlackNotifier {
	return &SlackNotifier{URL: url, Client: &http.Client{Timeout: notifyTimeout}}
}

func (s *SlackNotifier) Name() string { return "slack" }

func (s *SlackNotifier) Notify(ctx context.Context, alert Alert) error {
	icon := ":rotating_light:"
	verb := "FIRING"
	if alert.State == StateResolved {
		icon = ":white_check_mark:"
		verb = "RESOLVED"
	}

	payload := map[string]string{
		"text": fmt.Sprintf("%s *%s* — %s", icon, verb, alert.Summary()),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}
	return post(ctx, s.Client, s.URL, body)
}

func post(ctx context.Context, client *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	// Body is drained and closed so the connection can be reused; the
	// close error is not actionable on a fire-and-forget notification.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// BuildNotifiers constructs the configured channels.
//
// The URLs come from the operator's own config. They are not validated against
// an allowlist, which means a misconfigured or hostile config file can make the
// daemon issue outbound requests to arbitrary hosts — the same trust model as
// every other monitoring tool's webhook support, but worth stating.
func BuildNotifiers(webhookURL, slackURL string) []Notifier {
	var notifiers []Notifier
	if u := strings.TrimSpace(webhookURL); u != "" {
		notifiers = append(notifiers, NewWebhookNotifier(u))
	}
	if u := strings.TrimSpace(slackURL); u != "" {
		notifiers = append(notifiers, NewSlackNotifier(u))
	}
	return notifiers
}
