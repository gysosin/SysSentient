package ai

import (
	"testing"
	"time"
)

func TestCollapseLogs(t *testing.T) {
	input := `Error A
Error A
Error A
Error B
Error A
Error A`
	expected := `Error A [x3]
Error B
Error A [x2]`

	// Note: My implementation might be slightly different on the exact format, let's just check loosely or match exact impl.
	// Current Impl:
	// Error A [x3]
	// Error B
	// Error A [REPEATED]

	got := CollapseLogs(input)
	if got != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, got)
	}
}

func TestRAGStore(t *testing.T) {
	store := NewRAGStore()

	logs := "Test Log Sequence"
	insight := "It looks fine."

	_, found := store.GetCachedInsight(logs)
	if found {
		t.Error("Should not find insight initially")
	}

	store.SaveInsight(logs, insight)

	got, found := store.GetCachedInsight(logs)
	if !found {
		t.Error("Should find insight after save")
	}
	if got != insight {
		t.Errorf("Expected %s, got %s", insight, got)
	}

	// Different logs
	_, found = store.GetCachedInsight("Different logs")
	if found {
		t.Error("Should not find insight for different logs")
	}
}

func TestRAGStore_Expiry(t *testing.T) {
	store := NewRAGStore()
	logs := "Expired Log"
	insight := "Old news"

	store.SaveInsight(logs, insight)

	// Manually expire
	store.mu.Lock()
	entry := store.cache[GenerateLogSignature(logs)]
	entry.Timestamp = time.Now().Add(-25 * time.Hour)
	store.cache[GenerateLogSignature(logs)] = entry
	store.mu.Unlock()

	_, found := store.GetCachedInsight(logs)
	if found {
		t.Error("Should not return expired insight")
	}
}
