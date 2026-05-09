package collector

import (
	"strings"
	"testing"
)

func TestCompactProcessNameNormalizesAndTruncates(t *testing.T) {
	raw := "chrome --type=renderer,\n--some-long-flag=" + strings.Repeat("x", 120)

	got := compactProcessName(raw)
	if strings.Contains(got, ",") || strings.Contains(got, "\n") {
		t.Fatalf("expected compact name to remove commas and newlines, got %q", got)
	}
	if len(got) != 80 {
		t.Fatalf("expected compact name to be 80 chars, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected compact name to end with ellipsis, got %q", got)
	}
}
