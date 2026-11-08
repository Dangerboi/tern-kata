//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func execKata(kata string, args, environ []string, actor string) error {
	return runChild(kata, append([]string{"tui"}, args...), childEnv(environ, actor, true))
}

// editorFileEnv carries the temp file's path to cmd, which expands %VAR% once and does not expand the value again, so a % in the
// path stays literal.
const editorFileEnv = "TERN_KATA_EDIT_FILE"

func editorCommand(editor, file string) *exec.Cmd {
	cmd := exec.Command("cmd")
	// cmd /c strips the outer quotes, so $EDITOR keeps its own arguments and the path stays quoted.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /c "` + editor + ` "%` + editorFileEnv + `%""`}
	cmd.Env = append(os.Environ(), editorFileEnv+"="+file)
	return cmd
}
