package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// holdOld starts a copy of the test binary as target.old and keeps it running, so Windows will neither delete nor overwrite
// that file until the returned function stops the process.
func holdOld(t *testing.T, old string) (release func()) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, content, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(old)
	cmd.Env = append(os.Environ(), "TERN_KATA_TEST_BINARY=hold")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	release = func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}
	t.Cleanup(release)
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("the held process said %q, %v", line, err)
	}
	return release
}

func swapFiles(t *testing.T) (target, fresh string) {
	t.Helper()
	dir := t.TempDir()
	target, fresh = filepath.Join(dir, "tern-kata.exe"), filepath.Join(dir, "new.exe")
	for path, text := range map[string]string{target: "installed", fresh: "new release"} {
		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return target, fresh
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSwapRenamesAHeldOldAside(t *testing.T) {
	target, fresh := swapFiles(t)
	release := holdOld(t, target+".old")
	held := readText(t, target+".old")

	if err := swapBinary(target, fresh); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, target); got != "new release" {
		t.Errorf("the binary is %q", got)
	}
	if got := readText(t, target+".old"); got != "installed" {
		t.Errorf(".old is %q, want the replaced binary", got)
	}
	aside, _ := filepath.Glob(target + ".old-*")
	if len(aside) != 1 || readText(t, aside[0]) != held {
		t.Fatalf("the held file is at %v, want one .old-* with its content", aside)
	}

	release()
	newer := filepath.Join(filepath.Dir(target), "newer.exe")
	if err := os.WriteFile(newer, []byte("newer release"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := swapBinary(target, newer); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(target + ".old-*"); len(left) != 0 {
		t.Errorf("the next update left %v", left)
	}
}

func TestSwapRenamesTheOldBinaryBackWhenTheLastRenameFails(t *testing.T) {
	target, fresh := swapFiles(t)
	failing := errors.New("access denied")
	renameFile = func(from, to string) error {
		if from == fresh {
			return failing
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { renameFile = os.Rename })

	if err := swapBinary(target, fresh); !errors.Is(err, failing) {
		t.Fatalf("error %v, want the failed rename", err)
	}
	if got := readText(t, target); got != "installed" {
		t.Errorf("the binary is %q, want the installed one back", got)
	}
	if _, err := os.Stat(target + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".old still exists: %v", err)
	}
	if got := readText(t, fresh); got != "new release" {
		t.Errorf("the new file is %q", got)
	}
}
