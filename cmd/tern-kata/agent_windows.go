//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// execAgent runs command as a child running as actor, with stdio attached, and exits with its status.
func execAgent(actor string, command, environ []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = childEnv(environ, actor, true)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Keep the parent alive on Ctrl+C/Break so it still exits with the child's status.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return subcommandError{err}
	}
	os.Exit(0)
	return nil
}
