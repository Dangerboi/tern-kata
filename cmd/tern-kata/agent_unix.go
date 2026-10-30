//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// execAgent replaces this process with command (looked up on PATH) running as actor, so its exit status is ours.
func execAgent(actor string, command, environ []string) error {
	path, err := exec.LookPath(command[0])
	if err != nil {
		return subcommandError{err}
	}
	if err := syscall.Exec(path, command, childEnv(environ, actor, false)); err != nil {
		return subcommandError{err}
	}
	return nil
}
