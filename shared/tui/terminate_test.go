//go:build unix

package tui

import (
	"os"
	"slices"
	"syscall"
	"testing"
	"time"
)

// Dying first would leave the host terminal in raw mode with mouse reporting on.
func TestWatchSignalsTearsDownBeforeDying(t *testing.T) {
	incoming := make(chan os.Signal, 1)
	died := make(chan struct{})

	var order []string
	stop := watchSignals(incoming,
		func() { order = append(order, "teardown") },
		func(sig os.Signal) {
			order = append(order, "die "+sig.String())
			close(died)
		},
	)
	defer stop()

	incoming <- syscall.SIGHUP

	select {
	case <-died:
	case <-time.After(2 * time.Second):
		t.Fatal("the signal was never acted on")
	}

	if want := []string{"teardown", "die hangup"}; !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestWatchSignalsStopsQuietly(t *testing.T) {
	incoming := make(chan os.Signal, 1)
	acted := make(chan struct{})

	stop := watchSignals(incoming, func() { close(acted) }, func(os.Signal) {})
	stop()
	// A deferred stop runs twice only by mistake, but closing a channel twice panics, so it has to be safe.
	stop()

	select {
	case <-acted:
		t.Fatal("teardown ran after the watcher was stopped")
	case <-time.After(50 * time.Millisecond):
	}
}
