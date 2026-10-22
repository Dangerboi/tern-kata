//go:build linux

package worker

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// parentDeath makes the kernel kill the child when the worker dies, so a killed worker leaves no running agent.
func parentDeath(attr *syscall.SysProcAttr) { attr.Pdeathsig = syscall.SIGKILL }

// startTicks is the process's kernel start time (field 22 of /proc/<pid>/stat), which guards against a reused pid.
func startTicks(pid int) uint64 {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	// The command name may hold spaces and parentheses; the fields that follow it start after the last ')'.
	text := string(raw)
	fields := strings.Fields(text[strings.LastIndexByte(text, ')')+1:])
	if len(fields) < 20 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[19], 10, 64)
	return n
}
