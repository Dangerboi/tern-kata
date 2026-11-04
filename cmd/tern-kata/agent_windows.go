//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// execAgent runs plan's command as a child in the environment agentEnv makes, with stdio attached, and exits with its status.
func execAgent(plan LaunchPlan, environ []string) error {
	return subcommandError{runChild(plan.Operands[0], plan.Operands[1:], agentEnv(environ, plan, true))}
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
