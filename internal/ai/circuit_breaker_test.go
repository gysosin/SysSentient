package ai

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)

	// Should allow requests in closed state
	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error in closed state, got %v", err)
	}

	if cb.GetState() != StateClosed {
		t.Errorf("Expected closed state, got %s", cb.GetStateName())
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)

	testErr := errors.New("test error")

	// Fail 3 times to open circuit
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return testErr
		})
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Expected open state after failures, got %s", cb.GetStateName())
	}

	// Next request should be rejected
	err := cb.Execute(func() error {
		t.Error("Function should not be called when circuit is open")
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	// Trigger open state
	testErr := errors.New("test error")
	cb.Execute(func() error { return testErr })
	cb.Execute(func() error { return testErr })

	if cb.GetState() != StateOpen {
		t.Error("Circuit should be open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Next request should transition to half-open
	executed := false
	cb.Execute(func() error {
		executed = true
		return nil
	})

	if !executed {
		t.Error("Function should execute in half-open state")
	}

	// Successful request should close circuit
	if cb.GetState() != StateClosed {
		t.Errorf("Circuit should be closed after success, got %s", cb.GetStateName())
	}
}

func TestCircuitBreaker_AllowsOnlyOneHalfOpenProbe(t *testing.T) {
	cb := NewCircuitBreaker(1, 10*time.Millisecond)
	cb.Execute(func() error { return errors.New("error") })

	time.Sleep(20 * time.Millisecond)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- cb.Execute(func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started
	err := cb.Execute(func() error {
		t.Fatal("second half-open request should not execute")
		return nil
	})
	if err != ErrTooManyRequests {
		t.Fatalf("expected ErrTooManyRequests, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("expected probe to succeed, got %v", err)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(1, 1*time.Minute)

	// Open the circuit
	cb.Execute(func() error { return errors.New("error") })

	if cb.GetState() != StateOpen {
		t.Error("Circuit should be open")
	}

	// Manual reset
	cb.Reset()

	if cb.GetState() != StateClosed {
		t.Error("Circuit should be closed after reset")
	}
}

func TestCircuitBreaker_FailureCounter(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)

	// One failure
	cb.Execute(func() error { return errors.New("error") })
	if cb.GetState() != StateClosed {
		t.Error("Circuit should still be closed after 1 failure")
	}

	// Two failures
	cb.Execute(func() error { return errors.New("error") })
	if cb.GetState() != StateClosed {
		t.Error("Circuit should still be closed after 2 failures")
	}

	// One success should reset counter
	cb.Execute(func() error { return nil })

	// Now need 3 more failures to open
	cb.Execute(func() error { return errors.New("error") })
	cb.Execute(func() error { return errors.New("error") })
	if cb.GetState() != StateClosed {
		t.Error("Circuit should still be closed")
	}

	cb.Execute(func() error { return errors.New("error") })
	if cb.GetState() != StateOpen {
		t.Error("Circuit should be open after 3 failures")
	}
}
