package alerting

import (
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func stateWithCPU(cpu float64) models.SystemState {
	return models.SystemState{Hostname: "testhost", CPUUsage: cpu}
}

func cpuRule(forDuration time.Duration) Rule {
	return Rule{
		ID: "cpu", Name: "CPU high", Metric: MetricCPUUsage,
		Op: GreaterThan, Threshold: 80, For: forDuration,
		Severity: SeverityWarning, Enabled: true,
	}
}

func TestTransientSpikeDoesNotFire(t *testing.T) {
	// The whole point of `For`. The previous hardcoded check was single-sample,
	// so every `make -j16` spike triggered a paid Gemini call.
	ev := NewEvaluator([]Rule{cpuRule(2 * time.Minute)})
	base := time.Now()

	// Condition true, but only for 30s.
	if got := ev.Evaluate(stateWithCPU(95), base); len(got) != 0 {
		t.Fatalf("first sample produced %d transitions, want 0", len(got))
	}
	if got := ev.Evaluate(stateWithCPU(95), base.Add(30*time.Second)); len(got) != 0 {
		t.Fatalf("30s in produced %d transitions, want 0 (still pending)", len(got))
	}

	active := ev.Active()
	if len(active) != 1 || active[0].State != StatePending {
		t.Fatalf("expected exactly one pending alert, got %+v", active)
	}

	// Spike ends before the duration elapses: nothing should ever have fired.
	transitions := ev.Evaluate(stateWithCPU(10), base.Add(45*time.Second))
	for _, tr := range transitions {
		if tr.State == StateFiring {
			t.Fatalf("a transient spike fired: %+v", tr)
		}
	}
	if len(ev.Active()) != 0 {
		t.Fatalf("pending alert should have been discarded, got %+v", ev.Active())
	}
}

func TestSustainedBreachFires(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(2 * time.Minute)})
	base := time.Now()

	ev.Evaluate(stateWithCPU(95), base)
	ev.Evaluate(stateWithCPU(95), base.Add(time.Minute))

	transitions := ev.Evaluate(stateWithCPU(95), base.Add(2*time.Minute+time.Second))
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	fired := transitions[0]
	if fired.State != StateFiring {
		t.Fatalf("state = %q, want %q", fired.State, StateFiring)
	}
	if fired.Value != 95 {
		t.Fatalf("value = %v, want 95", fired.Value)
	}
	if fired.Hostname != "testhost" {
		t.Fatalf("hostname = %q, want testhost", fired.Hostname)
	}
	if fired.StartedAt.After(fired.FiredAt) {
		t.Fatal("StartedAt must precede FiredAt")
	}
}

func TestFiringIsNotRepeated(t *testing.T) {
	// An alert that re-notifies on every 2s poll is worse than no alerting.
	ev := NewEvaluator([]Rule{cpuRule(time.Minute)})
	base := time.Now()

	ev.Evaluate(stateWithCPU(95), base)
	ev.Evaluate(stateWithCPU(95), base.Add(61*time.Second))

	for i := 2; i < 10; i++ {
		got := ev.Evaluate(stateWithCPU(95), base.Add(time.Duration(60+i)*time.Second))
		if len(got) != 0 {
			t.Fatalf("poll %d produced a duplicate transition: %+v", i, got)
		}
	}
}

func TestRecoveryResolves(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(time.Minute)})
	base := time.Now()

	ev.Evaluate(stateWithCPU(95), base)
	ev.Evaluate(stateWithCPU(95), base.Add(61*time.Second))

	transitions := ev.Evaluate(stateWithCPU(5), base.Add(2*time.Minute))
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1 resolve", len(transitions))
	}
	if transitions[0].State != StateResolved {
		t.Fatalf("state = %q, want %q", transitions[0].State, StateResolved)
	}
	if transitions[0].ResolvedAt.IsZero() {
		t.Fatal("ResolvedAt not set on resolve")
	}
	if len(ev.Active()) != 0 {
		t.Fatalf("resolved alert still active: %+v", ev.Active())
	}
}

func TestDisabledRulesAreIgnored(t *testing.T) {
	rule := cpuRule(0)
	rule.Enabled = false
	ev := NewEvaluator([]Rule{rule})

	if got := ev.Evaluate(stateWithCPU(100), time.Now()); len(got) != 0 {
		t.Fatalf("disabled rule produced transitions: %+v", got)
	}
}

func TestZeroDurationFiresImmediately(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(0)})

	transitions := ev.Evaluate(stateWithCPU(95), time.Now())
	if len(transitions) != 1 || transitions[0].State != StateFiring {
		t.Fatalf("got %+v, want a single firing transition", transitions)
	}
}

func TestUnavailableMetricDoesNotFire(t *testing.T) {
	// A host with no temperature sensor reports 0. That must not satisfy a
	// "temperature < 5" rule, nor should a missing filesystem list fire a
	// disk rule.
	rules := []Rule{
		{ID: "cold", Name: "Too cold", Metric: MetricTemperature, Op: LessThan, Threshold: 5, Severity: SeverityWarning, Enabled: true},
		{ID: "disk", Name: "Disk full", Metric: MetricDiskPercent, Op: GreaterThan, Threshold: 90, Severity: SeverityCritical, Enabled: true},
	}
	ev := NewEvaluator(rules)

	if got := ev.Evaluate(models.SystemState{Hostname: "h"}, time.Now()); len(got) != 0 {
		t.Fatalf("unavailable metrics produced transitions: %+v", got)
	}
}

func TestDiskRuleUsesFullestFilesystem(t *testing.T) {
	// Averaging would hide one full disk among several empty ones.
	ev := NewEvaluator([]Rule{
		{ID: "disk", Name: "Disk full", Metric: MetricDiskPercent, Op: GreaterThan, Threshold: 90, Severity: SeverityCritical, Enabled: true},
	})

	state := models.SystemState{
		Hostname: "h",
		Filesystems: []models.Filesystem{
			{Mountpoint: "/", TotalBytes: 100, UsedPercent: 10},
			{Mountpoint: "/var", TotalBytes: 100, UsedPercent: 95},
			{Mountpoint: "/boot", TotalBytes: 100, UsedPercent: 20},
		},
	}

	transitions := ev.Evaluate(state, time.Now())
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	if transitions[0].Value != 95 {
		t.Fatalf("value = %v, want 95 (the fullest filesystem)", transitions[0].Value)
	}
}

func TestAcknowledgeSuppressesWithoutResolving(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(0)})
	base := time.Now()
	ev.Evaluate(stateWithCPU(95), base)

	if !ev.Acknowledge("", "cpu") {
		t.Fatal("Acknowledge() returned false for a firing alert")
	}

	active := ev.Active()
	if len(active) != 1 {
		t.Fatalf("acknowledging removed the alert: %+v", active)
	}
	if !active[0].Acknowledged {
		t.Fatal("alert not marked acknowledged")
	}
	if active[0].State != StateFiring {
		t.Fatalf("state = %q, want still firing", active[0].State)
	}

	if ev.Acknowledge("", "nonexistent") {
		t.Fatal("Acknowledge() returned true for an unknown rule")
	}
}

func TestReplaceRulesDropsStateForRemovedRules(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(0)})
	ev.Evaluate(stateWithCPU(95), time.Now())
	if len(ev.Active()) != 1 {
		t.Fatal("setup: expected one active alert")
	}

	ev.ReplaceRules(nil)

	if len(ev.Active()) != 0 {
		t.Fatalf("removing a rule left its alert active: %+v", ev.Active())
	}
}

func TestValidateRejectsBadRules(t *testing.T) {
	tests := []struct {
		name string
		rule Rule
	}{
		{name: "empty id", rule: Rule{Name: "x", Metric: MetricCPUUsage, Op: GreaterThan, Severity: SeverityWarning}},
		{name: "empty name", rule: Rule{ID: "a", Metric: MetricCPUUsage, Op: GreaterThan, Severity: SeverityWarning}},
		{name: "unknown metric", rule: Rule{ID: "a", Name: "x", Metric: "nope", Op: GreaterThan, Severity: SeverityWarning}},
		{name: "unknown comparator", rule: Rule{ID: "a", Name: "x", Metric: MetricCPUUsage, Op: "~", Severity: SeverityWarning}},
		{name: "unknown severity", rule: Rule{ID: "a", Name: "x", Metric: MetricCPUUsage, Op: GreaterThan, Severity: "meh"}},
		{name: "negative duration", rule: Rule{ID: "a", Name: "x", Metric: MetricCPUUsage, Op: GreaterThan, Severity: SeverityWarning, For: -time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rule.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want an error for %+v", tt.rule)
			}
		})
	}

	for _, rule := range DefaultRules() {
		if err := rule.Validate(); err != nil {
			t.Fatalf("DefaultRules() contains an invalid rule %q: %v", rule.ID, err)
		}
	}
}

func stateFor(hostID string, cpu float64) models.SystemState {
	return models.SystemState{HostID: hostID, Hostname: hostID, CPUUsage: cpu}
}

func TestAlertStateIsPerHost(t *testing.T) {
	// With alert state keyed only by rule, a second host reporting a healthy
	// value would resolve the first host's alert, and a breach on one host
	// would suppress the same breach on another.
	ev := NewEvaluator([]Rule{cpuRule(0)})
	now := time.Now()

	firedA := ev.Evaluate(stateFor("host-a", 95), now)
	if len(firedA) != 1 || firedA[0].HostID != "host-a" {
		t.Fatalf("host-a transitions = %+v, want one firing for host-a", firedA)
	}

	// host-b breaching must fire independently, not be swallowed as a duplicate.
	firedB := ev.Evaluate(stateFor("host-b", 96), now)
	if len(firedB) != 1 || firedB[0].HostID != "host-b" {
		t.Fatalf("host-b transitions = %+v, want one firing for host-b", firedB)
	}

	if got := len(ev.Active()); got != 2 {
		t.Fatalf("Active() = %d alerts, want 2 (one per host)", got)
	}

	// host-b recovering must not resolve host-a.
	resolved := ev.Evaluate(stateFor("host-b", 5), now.Add(time.Minute))
	if len(resolved) != 1 || resolved[0].HostID != "host-b" {
		t.Fatalf("resolve transitions = %+v, want one for host-b only", resolved)
	}

	active := ev.Active()
	if len(active) != 1 {
		t.Fatalf("Active() = %d, want 1 (host-a still firing)", len(active))
	}
	if active[0].HostID != "host-a" {
		t.Fatalf("remaining alert is for %q, want host-a", active[0].HostID)
	}
}

func TestAcknowledgeIsScopedToOneHost(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(0)})
	now := time.Now()
	ev.Evaluate(stateFor("host-a", 95), now)
	ev.Evaluate(stateFor("host-b", 95), now)

	if !ev.Acknowledge("host-a", "cpu") {
		t.Fatal("Acknowledge(host-a) returned false")
	}

	for _, alert := range ev.Active() {
		switch alert.HostID {
		case "host-a":
			if !alert.Acknowledged {
				t.Error("host-a alert not acknowledged")
			}
		case "host-b":
			if alert.Acknowledged {
				t.Error("acknowledging host-a also silenced host-b")
			}
		}
	}

	if ev.Acknowledge("host-c", "cpu") {
		t.Fatal("Acknowledge() returned true for a host with no active alert")
	}
}

func TestActiveForHostFilters(t *testing.T) {
	ev := NewEvaluator([]Rule{cpuRule(0)})
	now := time.Now()
	ev.Evaluate(stateFor("host-a", 95), now)
	ev.Evaluate(stateFor("host-b", 95), now)

	onlyA := ev.ActiveForHost("host-a")
	if len(onlyA) != 1 || onlyA[0].HostID != "host-a" {
		t.Fatalf("ActiveForHost(host-a) = %+v, want a single host-a alert", onlyA)
	}
	if got := len(ev.ActiveForHost("")); got != 2 {
		t.Fatalf("ActiveForHost(\"\") = %d, want all 2", got)
	}
}

func TestForgetHostDropsItsAlerts(t *testing.T) {
	// A decommissioned host must not leave alerts firing forever.
	ev := NewEvaluator([]Rule{cpuRule(0)})
	ev.Evaluate(stateFor("host-a", 95), time.Now())
	ev.Evaluate(stateFor("host-b", 95), time.Now())

	ev.ForgetHost("host-a")

	active := ev.Active()
	if len(active) != 1 || active[0].HostID != "host-b" {
		t.Fatalf("after ForgetHost(host-a), Active() = %+v, want only host-b", active)
	}
}
