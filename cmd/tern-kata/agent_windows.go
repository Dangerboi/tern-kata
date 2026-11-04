//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// execAgent runs command as a child running as actor on the Issue ref with the inbox agentEnv takes, with stdio attached, and
// exits with its status.
func execAgent(actor, ref, inbox string, command, environ []string) error {
	return subcommandError{runChild(command[0], command[1:], agentEnv(environ, actor, ref, inbox, true))}
}

// runChild runs name with stdio attached and exits with its status; it returns only when the child could not be run.
func runChild(name string, args, env []string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Keep the parent alive on Ctrl+C/Break so it still exits with the child's status.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err == nil {
		os.Exit(0)
	}
	return err
}
