package alerting

import (
	"sort"
	"sync"
	"time"

	"sys-sentient/internal/models"
)

// Evaluator applies rules to each collected sample and tracks alert lifecycle.
//
// The state machine is deliberately small:
//
//	condition true  + duration not yet met -> pending  (no notification)
//	condition true  + duration met         -> firing   (notify once)
//	condition false + was firing           -> resolved (notify once)
//	condition false + was pending          -> discarded silently
//
// Only transitions are returned, so a rule that stays breached for an hour
// notifies once rather than on every 2-second poll.
//
// State is tracked per (host, rule): with a fleet reporting into one server,
// keying on the rule alone would let one host's healthy sample resolve
// another's alert, and would suppress the same breach on a second machine.
//
// Safe for concurrent use: ingest evaluates while the API reads.
type Evaluator struct {
	mu     sync.RWMutex
	rules  []Rule
	active map[alertKey]*Alert
	// resolveAfter is how long a metric must stay clear before its alert
	// resolves. Without it a single non-breaching sample resolved the alert,
	// so a host sitting on the threshold fired and resolved on every poll.
	resolveAfter time.Duration
}

// DefaultResolveAfter is the settle time applied unless overridden.
const DefaultResolveAfter = 60 * time.Second

// alertKey identifies one rule's activation on one host.
type alertKey struct {
	hostID string
	ruleID string
}

func NewEvaluator(rules []Rule) *Evaluator {
	return &Evaluator{
		rules:        rules,
		active:       make(map[alertKey]*Alert),
		resolveAfter: DefaultResolveAfter,
	}
}

// SetResolveAfter changes the settle time. Zero restores the default; a
// negative value disables hysteresis, which is only useful in tests.
func (e *Evaluator) SetResolveAfter(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if d == 0 {
		d = DefaultResolveAfter
	}
	e.resolveAfter = d
}

// Rules returns a copy of the configured rules.
func (e *Evaluator) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]Rule, len(e.rules))
	copy(out, e.rules)
	return out
}

// ReplaceRules swaps the rule set, discarding alert state for rules that no
// longer exist so a deleted rule cannot leave an alert stuck firing forever.
func (e *Evaluator) ReplaceRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = rules

	known := make(map[string]bool, len(rules))
	for _, r := range rules {
		known[r.ID] = true
	}
	for key := range e.active {
		if !known[key.ruleID] {
			delete(e.active, key)
		}
	}
}

// Active returns the currently pending or firing alerts, most severe first.
func (e *Evaluator) Active() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]Alert, 0, len(e.active))
	for _, a := range e.active {
		out = append(out, *a)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			// critical before warning
			return out[i].Severity == SeverityCritical
		}
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		// Deterministic order when two hosts breach in the same instant.
		if out[i].HostID != out[j].HostID {
			return out[i].HostID < out[j].HostID
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

// Acknowledge silences notifications for one host's active alert without
// resolving it. Returns false if no such alert is active.
func (e *Evaluator) Acknowledge(hostID, ruleID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	alert, ok := e.active[alertKey{hostID: hostID, ruleID: ruleID}]
	if !ok {
		return false
	}
	alert.Acknowledged = true
	return true
}

// ActiveForHost returns active alerts for one host. An empty hostID returns
// every host's alerts.
func (e *Evaluator) ActiveForHost(hostID string) []Alert {
	all := e.Active()
	if hostID == "" {
		return all
	}

	filtered := make([]Alert, 0, len(all))
	for _, alert := range all {
		if alert.HostID == hostID {
			filtered = append(filtered, alert)
		}
	}
	return filtered
}

// ForgetHost drops all alert state for a host, so a decommissioned machine does
// not leave alerts firing forever.
func (e *Evaluator) ForgetHost(hostID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for key := range e.active {
		if key.hostID == hostID {
			delete(e.active, key)
		}
	}
}

// Evaluate applies every enabled rule to one sample and returns the state
// transitions that occurred. `now` is injected so tests can drive durations
// without sleeping.
func (e *Evaluator) Evaluate(state models.SystemState, now time.Time) []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	var transitions []Alert

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		value, available := MetricValue(state, rule.Metric)
		if !available {
			// A metric the host does not report must never satisfy a rule --
			// a missing sensor reads as 0 and would trip any `<` threshold.
			continue
		}

		key := alertKey{hostID: state.HostID, ruleID: rule.ID}
		existing, tracked := e.active[key]
		breached := rule.matches(value)

		switch {
		case breached && !tracked:
			alert := &Alert{
				RuleID:    rule.ID,
				RuleName:  rule.Name,
				Metric:    rule.Metric,
				Severity:  rule.Severity,
				Value:     value,
				Threshold: rule.Threshold,
				HostID:    state.HostID,
				Hostname:  state.Hostname,
				StartedAt: now,
				State:     StatePending,
			}
			// A zero duration means fire on first breach.
			if rule.For <= 0 {
				alert.State = StateFiring
				alert.FiredAt = now
				transitions = append(transitions, *alert)
			}
			e.active[key] = alert

		case breached && tracked:
			existing.Value = value
			existing.Hostname = state.Hostname
			// Breaching again cancels a resolve in progress.
			existing.ClearSince = time.Time{}
			if existing.State == StatePending && now.Sub(existing.StartedAt) >= rule.For {
				existing.State = StateFiring
				existing.FiredAt = now
				transitions = append(transitions, *existing)
			}

		case !breached && tracked:
			// Only a firing alert resolves. A pending one never notified
			// anybody, so it is dropped without a resolve event -- and without
			// waiting out the settle window below, which exists to stop
			// notifications flapping and has nothing to hold back here.
			if existing.State == StateFiring {
				// Hysteresis. A single non-breaching sample used to resolve
				// immediately, so a host oscillating either side of the
				// threshold flapped on every poll: firing and resolving, and
				// notifying each time. A firing alert must stay clear for
				// ResolveAfter before it is considered over.
				if existing.ClearSince.IsZero() {
					existing.ClearSince = now
				}
				if now.Sub(existing.ClearSince) < e.resolveAfter {
					existing.Value = value
					continue
				}

				resolved := *existing
				resolved.State = StateResolved
				resolved.Value = value
				resolved.ResolvedAt = now
				transitions = append(transitions, resolved)
			}
			delete(e.active, key)
		}
	}

	return transitions
}
