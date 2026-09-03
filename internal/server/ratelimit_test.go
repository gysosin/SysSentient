package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsBurstThenBlocks(t *testing.T) {
	limiter := newRateLimiter(3, time.Second)
	now := time.Now()

	for i := 0; i < 3; i++ {
		if !limiter.allow("client-a", now) {
			t.Fatalf("request %d in the burst was rejected", i+1)
		}
	}

	if limiter.allow("client-a", now) {
		t.Fatal("4th request in the burst was allowed, want rejection")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	limiter := newRateLimiter(2, time.Second)
	now := time.Now()

	limiter.allow("c", now)
	limiter.allow("c", now)
	if limiter.allow("c", now) {
		t.Fatal("burst not exhausted")
	}

	// One token per second.
	if !limiter.allow("c", now.Add(1100*time.Millisecond)) {
		t.Fatal("token did not refill after a second")
	}
}

func TestRateLimiterIsPerClient(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	now := time.Now()

	if !limiter.allow("client-a", now) {
		t.Fatal("client-a first request rejected")
	}
	if limiter.allow("client-a", now) {
		t.Fatal("client-a second request allowed")
	}
	// A different client must have its own budget.
	if !limiter.allow("client-b", now) {
		t.Fatal("client-b was blocked by client-a's usage")
	}
}

func TestRateLimiterDoesNotExceedCapacityAfterLongIdle(t *testing.T) {
	limiter := newRateLimiter(2, time.Second)
	now := time.Now()

	limiter.allow("c", now)

	// Idle for an hour: the bucket must cap at capacity, not accumulate
	// thousands of tokens.
	later := now.Add(time.Hour)
	allowed := 0
	for i := 0; i < 10; i++ {
		if limiter.allow("c", later) {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("allowed %d requests after idle, want exactly the capacity (2)", allowed)
	}
}

func TestRateLimitMiddlewareReturns429WithRetryAfter(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	handler := rateLimit(limiter, "60", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	handler(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	handler(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", second.Code)
	}
	if got := second.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
}

func TestClientKeyIgnoresForwardedForHeader(t *testing.T) {
	// X-Forwarded-For is attacker-controlled. Keying on it would let a caller
	// bypass the limit entirely by varying the header.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := clientKey(req); got != "203.0.113.9" {
		t.Fatalf("clientKey() = %q, want the socket address 203.0.113.9", got)
	}
}
