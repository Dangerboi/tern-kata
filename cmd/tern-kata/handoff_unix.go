//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// execKata replaces this process with `kata tui`.
func execKata(kata string, args, env []string) error {
	return syscall.Exec(kata, append([]string{"kata", "tui"}, args...), env)
}

func editorCommand(editor, file string) *exec.Cmd {
	return exec.Command("sh", "-c", editor+" \"$1\"", "tern-kata-editor", file)
}
