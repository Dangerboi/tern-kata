//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// execKata runs `kata tui` as a child with stdio attached and exits with its status.
func execKata(kata string, args, env []string) error {
	cmd := exec.Command(kata, append([]string{"tui"}, args...)...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Keep the parent alive on Ctrl+C/Break so it still exits with the child's status.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func editorCommand(editor, file string) *exec.Cmd {
	cmd := exec.Command("cmd")
	// cmd /c strips the outer quotes, so $EDITOR keeps its own arguments and the path stays quoted.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /c "` + editor + ` "` + file + `""`}
	return cmd
}
