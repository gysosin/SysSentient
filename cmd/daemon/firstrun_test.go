package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAnnounceFirstRunStaysQuietWhenNotATerminal(t *testing.T) {
	var buf bytes.Buffer
	announceFirstRun(&buf, "http://localhost:8080/setup", "tok")
	// In a journal or a container log the box drawing is noise, and the
	// structured log line already carries the token.
	if buf.Len() != 0 {
		t.Errorf("wrote %q to a non-terminal", buf.String())
	}
}

func TestRenderFirstRunCarriesBothTheURLAndTheToken(t *testing.T) {
	out := renderFirstRun("http://localhost:8080/setup", "tok-123")

	// The whole point is that the one required step is impossible to miss.
	for _, want := range []string{"http://localhost:8080/setup", "tok-123", "One step left"} {
		if !strings.Contains(out, want) {
			t.Errorf("notice is missing %q:\n%s", want, out)
		}
	}
}

func TestRenderFirstRunKeepsItsBoxAligned(t *testing.T) {
	out := renderFirstRun("http://localhost:8080/setup", "tok-123")

	var widths []int
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		widths = append(widths, len([]rune(line)))
	}
	for i, w := range widths {
		// A ragged box reads as corrupted output, which undermines the one
		// message the operator most needs to trust.
		if w != widths[0] {
			t.Fatalf("line %d is %d runes wide, first line is %d:\n%s", i, w, widths[0], out)
		}
	}
}

func TestRenderFirstRunDoesNotPutTheTokenInTheURL(t *testing.T) {
	out := renderFirstRun("http://localhost:8080/setup", "tok-123")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Open:") && strings.Contains(line, "tok-123") {
			// A secret in a URL ends up in browser history; the operator can
			// paste it from the terminal instead.
			t.Errorf("the token is embedded in the URL: %q", line)
		}
	}
}
