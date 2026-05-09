package ai

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for AI service
type CircuitBreaker struct {
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenMaxCalls int

	mu            sync.RWMutex
	state         CircuitState
	failures      int
	lastFailTime  time.Time
	halfOpenCalls int
}

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:      maxFailures,
		resetTimeout:     resetTimeout,
		halfOpenMaxCalls: 1,
		state:            StateClosed,
	}
}

// Execute runs a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()
	cb.afterRequest(err)
	return err
}

// beforeRequest checks if request should be allowed
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.halfOpenCalls = 1
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		if cb.halfOpenCalls >= cb.halfOpenMaxCalls {
			return ErrTooManyRequests
		}
		cb.halfOpenCalls++
		return nil

	default: // StateClosed
		return nil
	}
}

// afterRequest updates circuit breaker state based on result
func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()

		if cb.state == StateHalfOpen {
			// Failed in half-open, go back to open
			cb.state = StateOpen
			cb.halfOpenCalls = 0
		} else if cb.failures >= cb.maxFailures {
			// Too many failures, open the circuit
			cb.state = StateOpen
		}
	} else {
		// Success
		if cb.state == StateHalfOpen {
			// Success in half-open, close the circuit
			cb.state = StateClosed
			cb.failures = 0
			cb.halfOpenCalls = 0
		} else {
			// Reset failure counter on success
			cb.failures = 0
		}
	}
}

// GetState returns current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStateName returns human-readable state name
func (cb *CircuitBreaker) GetStateName() string {
	state := cb.GetState()
	switch state {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.halfOpenCalls = 0
}
