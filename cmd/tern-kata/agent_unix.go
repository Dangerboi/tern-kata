//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func execAgent(plan LaunchPlan, environ []string) error {
	path, err := exec.LookPath(plan.Operands[0])
	if err != nil {
		return subcommandError{err}
	}
	if err := syscall.Exec(path, plan.Operands, agentEnv(closeTranscriptEnv(environ, false), plan, false)); err != nil {
		return subcommandError{err}
	}
	return nil
}
