package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/windows/svc"
)

// stopped is closed once the daemon has finished shutting down, so the service
// handler can report Stopped to the SCM rather than letting the process vanish
// mid-transition.
var (
	stopped     = make(chan struct{})
	stoppedOnce sync.Once
)

// serviceShutdownComplete tells the SCM handler that shutdown finished.
func serviceShutdownComplete() {
	stoppedOnce.Do(func() { close(stopped) })
}

// shutdownContext cancels on an interrupt, on SIGTERM, or on a Stop request
// from the Windows service manager.
//
// Without the SCM half, `sc start sys-sentient` launches the process and then
// marks the service failed: Windows expects a service to connect back to the
// dispatcher within its timeout and answer control requests, and a plain
// console binary never does.
func shutdownContext() (context.Context, context.CancelFunc) {
	base, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		// Running from a console: nothing more to do.
		return base, cancel
	}

	ctx, cancelService := context.WithCancel(base)
	go func() {
		// Errors here are reported through the service's own event log; there
		// is no console to print to.
		_ = svc.Run(serviceName, &handler{cancel: cancelService})
	}()
	return ctx, func() {
		cancelService()
		cancel()
	}
}

const serviceName = "sys-sentient"

type handler struct {
	cancel context.CancelFunc
}

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}
	s <- svc.Status{State: svc.Running, Accepts: accepted}

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			s <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			h.cancel()
			// Wait for the daemon to finish rather than returning immediately:
			// the SCM treats a service that reports Stopped while still
			// writing to its database as a clean stop, and the next start can
			// then race the old process.
			<-stopped
			return false, 0
		default:
			// Unknown control requests are ignored, which is what the SCM
			// expects for anything not in Accepts.
		}
	}
	return false, 0
}
