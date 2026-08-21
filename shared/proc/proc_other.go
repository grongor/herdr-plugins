//go:build !unix

package proc

import "os/exec"

// Detach is a no-op: the signals it dodges are POSIX ones.
func Detach(*exec.Cmd) {}
