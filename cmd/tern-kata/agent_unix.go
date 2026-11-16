//go:build !windows

package main

import (
	"os/exec"
	"syscall"

	"github.com/Dangerboi/tern-kata/internal/envname"
)

func execAgent(plan LaunchPlan, environ []string) error {
	path, err := exec.LookPath(plan.Operands[0])
	if err != nil {
		return subcommandError{err}
	}
	if err := syscall.Exec(path, plan.Operands, agentEnv(closeTranscriptEnv(environ, envname.CaseInsensitive), plan, envname.CaseInsensitive)); err != nil {
		return subcommandError{err}
	}
	return nil
}
