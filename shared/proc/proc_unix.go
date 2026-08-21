//go:build unix

package proc

import (
	"os/exec"
	"syscall"
)

// Detach makes cmd start in its own session. A new process group is not enough: Herdr 0.7.5 sends SIGHUP, SIGHUP,
// SIGTERM to the plugin process (not the terminal) when its pane closes, and a child in its own group dies with it.
func Detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Setsid = true
}
