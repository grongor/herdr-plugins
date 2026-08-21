//go:build !unix

package tui

// onTerminate is a no-op where the signals that skip deferred functions do not exist.
func onTerminate(func()) (stop func()) { return func() {} }
