package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		given string
		want  slog.Level
	}{
		{given: "debug", want: slog.LevelDebug},
		{given: "DEBUG", want: slog.LevelDebug},
		{given: "info", want: slog.LevelInfo},
		{given: "warn", want: slog.LevelWarn},
		{given: "warning", want: slog.LevelWarn},
		{given: "error", want: slog.LevelError},
		{given: "  Error  ", want: slog.LevelError},
		{given: "nonsense", want: slog.LevelInfo},
		{given: "", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.given, func(t *testing.T) {
			if got := ParseLevel(tt.given); got != tt.want {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tt.given, got, tt.want)
			}
		})
	}
}

func TestJSONFormatEmitsParseableRecords(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{Level: "info", Format: "json"})

	logger.Info("collector tick", "cpu", 42.5, "host", "web-01")

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("output is not valid JSON: %v (%s)", err, buf.String())
	}
	if record["msg"] != "collector tick" {
		t.Fatalf("msg = %v, want %q", record["msg"], "collector tick")
	}
	if record["host"] != "web-01" {
		t.Fatalf("host = %v, want web-01", record["host"])
	}
	if record["level"] != "INFO" {
		t.Fatalf("level = %v, want INFO", record["level"])
	}
}

func TestLevelFiltersLowerSeverity(t *testing.T) {
	// The per-tick metrics line is logged at debug; at the default level it
	// must not appear, otherwise the daemon writes 43,200 lines a day.
	var buf bytes.Buffer
	logger := New(&buf, Options{Level: "info", Format: "text"})

	logger.Debug("per-tick metrics")
	if buf.Len() != 0 {
		t.Fatalf("debug record emitted at info level: %s", buf.String())
	}

	logger.Warn("something worth seeing")
	if !strings.Contains(buf.String(), "something worth seeing") {
		t.Fatalf("warn record was filtered out: %q", buf.String())
	}
}

func TestDebugLevelIncludesDebugRecords(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{Level: "debug", Format: "text"})

	logger.Debug("per-tick metrics", "cpu", 1.0)
	if !strings.Contains(buf.String(), "per-tick metrics") {
		t.Fatalf("debug record missing at debug level: %q", buf.String())
	}
}

func TestTextIsTheDefaultFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{Level: "info", Format: "unrecognised"}).Info("hello")

	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("unrecognised format produced JSON, want text: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("message missing: %q", buf.String())
	}
}
