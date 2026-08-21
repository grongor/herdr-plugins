//go:build unix

package tui

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// terminating are the signals that end the process without running deferred functions, which would skip the teardown
// and leave the host terminal in raw mode with mouse reporting on. SIGINT rarely arrives: raw mode clears ISIG, so
// ctrl-c comes through as a byte; it is listed for a signal sent from outside.
var terminating = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// onTerminate runs teardown when one of those arrives, then re-raises the signal with its default disposition so the
// process still dies of it and reports the exit status the sender asked for.
func onTerminate(teardown func()) (stop func()) {
	incoming := make(chan os.Signal, 1)
	signal.Notify(incoming, terminating...)

	stopWatch := watchSignals(incoming, teardown, reraise)

	return func() {
		signal.Stop(incoming)
		stopWatch()
	}
}

// watchSignals is the half with no operating system in it, so the order — tear down first, die second — can be tested
// without killing the test.
func watchSignals(incoming <-chan os.Signal, teardown func(), die func(os.Signal)) (stop func()) {
	done := make(chan struct{})

	go func() {
		select {
		case sig := <-incoming:
			teardown()
			die(sig)
		case <-done:
		}
	}()

	return sync.OnceFunc(func() { close(done) })
}

func reraise(sig os.Signal) {
	// Reset first, or the re-raised signal is delivered back to this handler.
	signal.Reset(sig)

	if raw, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(syscall.Getpid(), raw)
	}
}
