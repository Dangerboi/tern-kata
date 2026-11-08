//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func execKata(kata string, args, environ []string, actor string) error {
	return syscall.Exec(kata, append([]string{"kata", "tui"}, args...), childEnv(environ, actor, false))
}

func editorCommand(editor, file string) *exec.Cmd {
	return exec.Command("sh", "-c", editor+" \"$1\"", "tern-kata-editor", file)
}
