package hostid

import (
	"regexp"
	"testing"
)

func TestResolveReturnsStableID(t *testing.T) {
	id1, host1 := Resolve()
	id2, host2 := Resolve()

	if id1 == "" {
		t.Fatal("Resolve() returned an empty id")
	}
	if id1 != id2 {
		t.Fatalf("Resolve() is not stable: %q then %q", id1, id2)
	}
	if host1 != host2 {
		t.Fatalf("hostname is not stable: %q then %q", host1, host2)
	}
	if host1 == "" {
		t.Fatal("Resolve() returned an empty hostname")
	}
}

func TestResolveIDShape(t *testing.T) {
	id, _ := Resolve()

	if len(id) != 32 {
		t.Fatalf("id length = %d, want 32", len(id))
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("id = %q, want 32 lowercase hex characters", id)
	}
}

func TestDeriveDoesNotLeakTheRawIdentity(t *testing.T) {
	// The systemd machine-id is treated as confidential: D-Bus documentation
	// warns it must not be exposed, since other secrets are derived from it.
	const secret = "b8f3c1a29d4e4f0f8b7a6c5d4e3f2a10"

	derived := derive(secret)
	if derived == secret {
		t.Fatal("derive() returned the raw identity")
	}
	if len(derived) != 32 {
		t.Fatalf("derived length = %d, want 32", len(derived))
	}
}

func TestDeriveIsDeterministicAndDistinct(t *testing.T) {
	a := derive("machine-a")
	b := derive("machine-b")

	if a != derive("machine-a") {
		t.Fatal("derive() is not deterministic")
	}
	if a == b {
		t.Fatal("different inputs produced the same id")
	}
}
