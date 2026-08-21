//go:build !unix

package tui

// watchResize is a no-op where SIGWINCH does not exist; the picker still redraws on the next keystroke.
func watchResize() (<-chan struct{}, func()) {
	return make(chan struct{}), func() {}
}
