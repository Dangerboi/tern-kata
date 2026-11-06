//go:build darwin

package worker

import "golang.org/x/sys/unix"

// startTicks is the process's start time in microseconds since the epoch (kern.proc.pid), which guards against a reused pid.
func startTicks(pid int) uint64 {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0
	}
	t := kp.Proc.P_starttime
	return uint64(t.Sec)*1_000_000 + uint64(t.Usec)
}
