//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// shutdownContext cancels on an interrupt or SIGTERM.
func shutdownContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// serviceShutdownComplete is a no-op off Windows. Only the SCM needs to be
// told that shutdown finished.
func serviceShutdownComplete() {}
