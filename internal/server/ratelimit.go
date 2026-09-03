package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a fixed-capacity token bucket keyed by client.
//
// POST /api/analyze triggers a paid Gemini call and had no limit of any kind:
// a loop against it is a direct financial denial-of-service. /api/logs is also
// worth bounding because each call shells out to journalctl and dmesg.
//
// Deliberately simple — no Redis, no distributed state. A single-node daemon
// only needs to stop a runaway client, and an in-process bucket does that
// without adding a dependency.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	// refill is tokens added per second.
	refill float64
	// lastSweep bounds map growth from one-off clients.
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// newRateLimiter allows `burst` requests immediately, then one per
// `perRequest` interval.
func newRateLimiter(burst int, perRequest time.Duration) *rateLimiter {
	if burst < 1 {
		burst = 1
	}
	if perRequest <= 0 {
		perRequest = time.Second
	}
	return &rateLimiter{
		buckets:   make(map[string]*bucket),
		capacity:  float64(burst),
		refill:    1 / perRequest.Seconds(),
		lastSweep: time.Now(),
	}
}

// allow reports whether the client may proceed, consuming a token if so.
func (r *rateLimiter) allow(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sweepLocked(now)

	b, ok := r.buckets[key]
	if !ok {
		r.buckets[key] = &bucket{tokens: r.capacity - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * r.refill
		if b.tokens > r.capacity {
			b.tokens = r.capacity
		}
		b.last = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepLocked drops buckets that have been idle long enough to be full again,
// so a scanner hitting thousands of source addresses cannot grow the map
// without bound.
func (r *rateLimiter) sweepLocked(now time.Time) {
	if now.Sub(r.lastSweep) < time.Minute {
		return
	}
	r.lastSweep = now

	idleFullAfter := time.Duration(r.capacity/r.refill) * time.Second
	for key, b := range r.buckets {
		if now.Sub(b.last) > idleFullAfter {
			delete(r.buckets, key)
		}
	}
}

// clientKey identifies the caller for rate-limiting purposes.
//
// RemoteAddr is used rather than X-Forwarded-For: that header is client
// controlled, so trusting it would let anyone bypass the limit by varying it.
// Behind a reverse proxy every request shares the proxy's address, which is the
// safe failure mode (over-restrictive, not bypassable).
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimit wraps a handler, rejecting callers over their budget with 429.
func rateLimit(limiter *rateLimiter, retryAfter string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientKey(r), time.Now()) {
			w.Header().Set("Retry-After", retryAfter)
			writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}
