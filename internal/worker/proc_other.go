//go:build !windows && !linux

package worker

import "syscall"

// parentDeath: elsewhere a killed worker may leave an agent behind, which the next start finds as group_unproven.
func parentDeath(*syscall.SysProcAttr) {}

func startTicks(int) uint64 { return 0 }
