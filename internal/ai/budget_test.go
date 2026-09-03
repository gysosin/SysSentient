package ai

import (
	"errors"
	"testing"
	"time"
)

func TestBudgetDisabledWhenZero(t *testing.T) {
	// A zero cap means "no limit", matching the config default semantics for
	// operators who do not want a ceiling.
	b := NewCostBudget(0)

	if b.Enabled() {
		t.Fatal("Enabled() = true for a zero cap")
	}
	if err := b.Check(); err != nil {
		t.Fatalf("Check() = %v, want nil when disabled", err)
	}
	// Recording must not panic or accumulate.
	b.Record(1_000_000_000, 1_000_000_000)
	if err := b.Check(); err != nil {
		t.Fatalf("Check() = %v after recording with the cap disabled, want nil", err)
	}
}

func TestBudgetBlocksOnceCapReached(t *testing.T) {
	b := NewCostBudget(0.01)

	if err := b.Check(); err != nil {
		t.Fatalf("first Check() = %v, want nil", err)
	}

	// 1M output tokens at $0.40/1M is well past a $0.01 cap.
	b.Record(0, 1_000_000)

	err := b.Check()
	if !errors.Is(err, ErrDailyBudgetExhausted) {
		t.Fatalf("Check() = %v, want ErrDailyBudgetExhausted", err)
	}
}

func TestBudgetResetsOnUTCDayChange(t *testing.T) {
	b := NewCostBudget(0.01)

	day1 := time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return day1 }

	b.Record(0, 1_000_000)
	if !errors.Is(b.Check(), ErrDailyBudgetExhausted) {
		t.Fatal("budget should be exhausted on day 1")
	}

	// Next UTC day: the ledger resets.
	b.now = func() time.Time { return day1.Add(2 * time.Hour) }
	if err := b.Check(); err != nil {
		t.Fatalf("Check() = %v after the day rolled over, want nil", err)
	}

	spent, limit := b.Spent()
	if spent != 0 {
		t.Fatalf("spent = %v after rollover, want 0", spent)
	}
	if limit != 0.01 {
		t.Fatalf("limit = %v, want 0.01", limit)
	}
}

func TestRecordReturnsCostAndAccumulates(t *testing.T) {
	b := NewCostBudget(10)

	// 1M input at $0.10 + 1M output at $0.40 = $0.50
	cost := b.Record(1_000_000, 1_000_000)
	if cost < 0.49 || cost > 0.51 {
		t.Fatalf("Record() = %v, want ~0.50", cost)
	}

	b.Record(1_000_000, 1_000_000)
	spent, _ := b.Spent()
	if spent < 0.99 || spent > 1.01 {
		t.Fatalf("spent = %v after two calls, want ~1.00", spent)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		given string
		want  int64
	}{
		{name: "empty", given: "", want: 0},
		{name: "short", given: "abcd", want: 2},
		{name: "longer", given: string(make([]byte, 400)), want: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EstimateTokens(tt.given); got != tt.want {
				t.Fatalf("EstimateTokens(len=%d) = %d, want %d", len(tt.given), got, tt.want)
			}
		})
	}
}

func TestBudgetIsConcurrencySafe(t *testing.T) {
	b := NewCostBudget(1000)

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				b.Record(1000, 1000)
				_ = b.Check()
				_, _ = b.Spent()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}

	spent, _ := b.Spent()
	if spent <= 0 {
		t.Fatalf("spent = %v after concurrent recording, want > 0", spent)
	}
}
