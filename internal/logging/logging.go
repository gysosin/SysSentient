// Package logging configures the daemon's structured logger.
//
// Before this, output was split across two streams in two formats: fmt.Printf
// went to stdout (startup banners, a metrics line every 2 seconds forever) and
// log.Printf went to stderr (errors), with no levels, no JSON option and no way
// to silence the per-tick line. That is 43,200 lines a day of prose no log
// aggregator can parse.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// Options configures the logger.
type Options struct {
	Level  string // debug|info|warn|error
	Format string // text|json
}

// ParseLevel maps a config string to a slog level, defaulting to info.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New builds a logger writing to w.
//
// Everything goes to a single stream: splitting informational output across
// stdout and stderr made the daemon's own logs impossible to follow.
func New(w io.Writer, opts Options) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{Level: ParseLevel(opts.Level)}

	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(opts.Format), "json") {
		handler = slog.NewJSONHandler(w, handlerOpts)
	} else {
		handler = slog.NewTextHandler(w, handlerOpts)
	}

	return slog.New(handler)
}
