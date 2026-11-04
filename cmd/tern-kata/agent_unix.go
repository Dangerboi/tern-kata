//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// execAgent replaces this process with command (looked up on PATH) running as actor on the Issue ref with the inbox agentEnv
// takes, so its exit status is ours.
func execAgent(actor, ref, inbox string, command, environ []string) error {
	path, err := exec.LookPath(command[0])
	if err != nil {
		return subcommandError{err}
	}
	if err := syscall.Exec(path, command, agentEnv(environ, actor, ref, inbox, false)); err != nil {
		return subcommandError{err}
	}
	return nil
}
