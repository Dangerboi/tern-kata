//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// execKata runs `kata tui` as a child with stdio attached and exits with its status.
func execKata(kata string, args, environ []string, actor string) error {
	return runChild(kata, append([]string{"tui"}, args...), childEnv(environ, actor, true))
}

func editorCommand(editor, file string) *exec.Cmd {
	cmd := exec.Command("cmd")
	// cmd /c strips the outer quotes, so $EDITOR keeps its own arguments and the path stays quoted.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /c "` + editor + ` "` + file + `""`}
	return cmd
}
