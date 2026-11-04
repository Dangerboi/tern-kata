//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// execAgent replaces this process with plan's command (looked up on PATH) in the environment agentEnv makes, so its exit status is
// ours.
func execAgent(plan LaunchPlan, environ []string) error {
	path, err := exec.LookPath(plan.Operands[0])
	if err != nil {
		return subcommandError{err}
	}
	if err := syscall.Exec(path, plan.Operands, agentEnv(environ, plan, false)); err != nil {
		return subcommandError{err}
	}
	return nil
}
