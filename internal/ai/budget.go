package ai

import (
	"errors"
	"sync"
	"time"
)

// ErrDailyBudgetExhausted is returned when the configured spend cap for the
// current UTC day has been reached.
var ErrDailyBudgetExhausted = errors.New("daily AI cost budget exhausted")

// CostBudget enforces gemini.max_daily_cost.
//
// That setting has existed since the first release: it is defined, defaulted to
// 1.0 and validated — and was then read by nothing. README and QUICK_START both
// told operators it caps spend. It did not. For a product where the customer
// pays per Gemini call, an advertised cost control that silently does nothing
// is a financial-exposure bug, not a missing feature.
//
// Accounting is approximate: token counts come from the API response when it
// reports them, and fall back to a character-based estimate when it does not.
// The goal is a safety rail against runaway spend, not billing-grade accuracy.
type CostBudget struct {
	mu sync.Mutex
	// maxDaily is the cap in USD. Zero disables enforcement.
	maxDaily float64
	spent    float64
	day      string

	inputPer1M  float64
	outputPer1M float64
	now         func() time.Time
}

// Pricing defaults approximate gemini-2.5-flash-lite (USD per million tokens).
// These are a conservative estimate; the point is a bound, not an invoice.
const (
	defaultInputCostPer1M  = 0.10
	defaultOutputCostPer1M = 0.40
)

func NewCostBudget(maxDaily float64) *CostBudget {
	return &CostBudget{
		maxDaily:    maxDaily,
		inputPer1M:  defaultInputCostPer1M,
		outputPer1M: defaultOutputCostPer1M,
		now:         time.Now,
	}
}

// Enabled reports whether a cap is in force.
func (b *CostBudget) Enabled() bool {
	return b != nil && b.maxDaily > 0
}

// rollover resets the ledger when the UTC day changes. Caller holds the lock.
func (b *CostBudget) rolloverLocked() {
	today := b.now().UTC().Format("2006-01-02")
	if b.day != today {
		b.day = today
		b.spent = 0
	}
}

// Check reports whether another call is permitted under today's budget.
func (b *CostBudget) Check() error {
	if !b.Enabled() {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.rolloverLocked()

	if b.spent >= b.maxDaily {
		return ErrDailyBudgetExhausted
	}
	return nil
}

// Record adds the cost of one completed call.
func (b *CostBudget) Record(inputTokens, outputTokens int64) float64 {
	cost := (float64(inputTokens)/1_000_000)*b.inputPer1M +
		(float64(outputTokens)/1_000_000)*b.outputPer1M

	if !b.Enabled() {
		return cost
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.rolloverLocked()
	b.spent += cost
	return cost
}

// Spent returns today's accumulated cost and the configured cap.
func (b *CostBudget) Spent() (spent, limit float64) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rolloverLocked()
	return b.spent, b.maxDaily
}

// EstimateTokens approximates a token count from text length, used only when
// the API response omits usage metadata. Roughly 4 characters per token.
func EstimateTokens(text string) int64 {
	if text == "" {
		return 0
	}
	return int64(len(text)/4) + 1
}
