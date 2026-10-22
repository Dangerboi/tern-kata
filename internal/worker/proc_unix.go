//go:build !windows

package worker

import (
	"errors"
	"os/exec"
	"syscall"
)

// unsupported is empty where the worker can prove a process group gone.
const unsupported = ""

// isolate starts cmd in its own process group so the whole tree can be signalled and proven gone (ADR-0006).
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	parentDeath(cmd.SysProcAttr)
}

// signalGroup sends sig (SIGTERM or SIGKILL by name) to every process of the group.
func signalGroup(pgid int, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pgid, sig)
}

// groupAlive reports whether any process of the group is still there or cannot be checked.
func groupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// exitSignal names the signal that ended the process, empty for a normal exit.
func exitSignal(state interface{ Sys() any }) string {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return ws.Signal().String()
	}
	return ""
}
