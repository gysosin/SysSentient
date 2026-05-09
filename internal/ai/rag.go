package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	ragCacheTTL        = 24 * time.Hour
	maxRAGCacheEntries = 128
)

// Simple in-memory vector store for deduplication
// In a real production system, this would be a persistent vector DB.
type RAGStore struct {
	cache map[string]CachedInsight
	mu    sync.RWMutex
}

type CachedInsight struct {
	Insight   string
	Timestamp time.Time
}

func NewRAGStore() *RAGStore {
	return &RAGStore{
		cache: make(map[string]CachedInsight),
	}
}

// GenerateLogSignature creates a hash of the log content to detect duplicates
// We can use this as a simple "embedding" for exact matching.
// For semantic matching, we'd use a real embedding model.
func GenerateLogSignature(logs string) string {
	// Normalize: remove timestamps, pointers, etc. (Basic heuristic)
	// For now, just hashing the raw string for exact deduplication of repeated blocks
	hash := sha256.Sum256([]byte(strings.TrimSpace(logs)))
	return hex.EncodeToString(hash[:])
}

// GetCachedInsight checks if we've analyzed this exact log pattern recently
func (r *RAGStore) GetCachedInsight(logs string) (string, bool) {
	sig := GenerateLogSignature(logs)

	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, found := r.cache[sig]
	if !found {
		return "", false
	}

	if time.Since(entry.Timestamp) > ragCacheTTL {
		return "", false
	}

	return entry.Insight, true
}

func (r *RAGStore) SaveInsight(logs, insight string) {
	sig := GenerateLogSignature(logs)

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.pruneExpiredLocked(now)
	if _, exists := r.cache[sig]; !exists && len(r.cache) >= maxRAGCacheEntries {
		r.evictOldestLocked()
	}

	r.cache[sig] = CachedInsight{
		Insight:   insight,
		Timestamp: now,
	}
}

func (r *RAGStore) pruneExpiredLocked(now time.Time) {
	for sig, entry := range r.cache {
		if now.Sub(entry.Timestamp) > ragCacheTTL {
			delete(r.cache, sig)
		}
	}
}

func (r *RAGStore) evictOldestLocked() {
	var oldestSig string
	var oldestTime time.Time
	for sig, entry := range r.cache {
		if oldestSig == "" || entry.Timestamp.Before(oldestTime) {
			oldestSig = sig
			oldestTime = entry.Timestamp
		}
	}
	if oldestSig != "" {
		delete(r.cache, oldestSig)
	}
}

// CollapseLogs reduces repetitive lines like:
// "Error connecting to DB"
// "Error connecting to DB"
// ...
// into: "[x2] Error connecting to DB"
func CollapseLogs(logs string) string {
	lines := strings.Split(logs, "\n")
	if len(lines) == 0 {
		return ""
	}

	var collapsed []string
	lastLine := ""
	count := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == lastLine {
			count++
		} else {
			if lastLine != "" {
				if count > 1 {
					collapsed = append(collapsed, fmt.Sprintf("%s [x%d]", lastLine, count))
				} else {
					collapsed = append(collapsed, lastLine)
				}
			}
			lastLine = line
			count = 1
		}
	}
	// Flush last
	if lastLine != "" {
		if count > 1 {
			collapsed = append(collapsed, fmt.Sprintf("%s [x%d]", lastLine, count))
		} else {
			collapsed = append(collapsed, lastLine)
		}
	}

	return strings.Join(collapsed, "\n")
}
