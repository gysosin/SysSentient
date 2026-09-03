package auth

import (
	"crypto/subtle"
	"sync"
)

// SetupToken gates first-run admin creation. It lives only in memory, is
// printed once to the daemon log, and is consumed by the first successful
// use — so a default password never exists, not even briefly.
type SetupToken struct {
	mu       sync.Mutex
	value    string
	consumed bool
}

func NewSetupToken() (*SetupToken, error) {
	v, err := NewToken()
	if err != nil {
		return nil, err
	}
	return &SetupToken{value: v}, nil
}

// String returns the token for the one log line that announces it.
func (t *SetupToken) String() string {
	return t.value
}

// Consume reports whether candidate matches, and if so retires the token.
func (t *SetupToken) Consume(candidate string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumed {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(t.value)) != 1 {
		return false
	}
	t.consumed = true
	return true
}
