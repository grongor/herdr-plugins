//go:build unix

package tui

import (
	"os"
	"os/signal"
	"syscall"
)

// watchResize reports terminal resizes so the picker redraws without waiting for the next keystroke.
func watchResize() (<-chan struct{}, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)

	resized := make(chan struct{}, 1)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-signals:
				select {
				case resized <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()

	return resized, func() {
		signal.Stop(signals)
		close(done)
	}
}
