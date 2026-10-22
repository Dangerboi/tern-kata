//go:build !windows

package worker

import (
	"errors"
	"os/exec"
	"syscall"
)

// isolate starts cmd in its own process group so the whole tree can be signalled and proven gone (ADR-0006).
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	parentDeath(cmd.SysProcAttr)
}

// adopt records the started process's group: the process leads it, and its kernel start time guards against a
// reused pid.
func adopt(cmd *exec.Cmd) (*LaunchRecord, error) {
	pid := cmd.Process.Pid
	return &LaunchRecord{PID: pid, PGID: pid, StartTicks: startTicks(pid)}, nil
}

// signalGroup sends SIGTERM, or SIGKILL with kill, to every process of the group.
func signalGroup(l *LaunchRecord, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-l.PGID, sig)
}

// groupAlive reports whether any process of the group is still there or cannot be checked. A leader pid that now
// belongs to a process started later was reused, so the group recorded is gone.
func groupAlive(l *LaunchRecord) bool {
	if err := syscall.Kill(-l.PGID, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if l.StartTicks != 0 {
		if now := startTicks(l.PID); now != 0 && now != l.StartTicks {
			return false
		}
	}
	return true
}

// exitSignal names the signal that ended the process, empty for a normal exit.
func exitSignal(state interface{ Sys() any }) string {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return ws.Signal().String()
	}
	return ""
}
