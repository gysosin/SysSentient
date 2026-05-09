package ai

import (
	"fmt"
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
	entry.Timestamp = time.Now().Add(-ragCacheTTL - time.Hour)
	store.cache[GenerateLogSignature(logs)] = entry
	store.mu.Unlock()

	_, found := store.GetCachedInsight(logs)
	if found {
		t.Error("Should not return expired insight")
	}
}

func TestRAGStorePrunesExpiredEntriesOnSave(t *testing.T) {
	store := NewRAGStore()
	store.SaveInsight("expired", "old")

	store.mu.Lock()
	entry := store.cache[GenerateLogSignature("expired")]
	entry.Timestamp = time.Now().Add(-ragCacheTTL - time.Hour)
	store.cache[GenerateLogSignature("expired")] = entry
	store.mu.Unlock()

	store.SaveInsight("fresh", "new")

	store.mu.RLock()
	defer store.mu.RUnlock()
	if _, found := store.cache[GenerateLogSignature("expired")]; found {
		t.Fatal("expected expired cache entry to be pruned")
	}
	if _, found := store.cache[GenerateLogSignature("fresh")]; !found {
		t.Fatal("expected fresh cache entry to remain")
	}
}

func TestRAGStoreCapsCacheSize(t *testing.T) {
	store := NewRAGStore()
	for i := 0; i < maxRAGCacheEntries+10; i++ {
		store.SaveInsight(fmt.Sprintf("logs-%d", i), "insight")
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.cache) > maxRAGCacheEntries {
		t.Fatalf("expected cache size <= %d, got %d", maxRAGCacheEntries, len(store.cache))
	}
}
