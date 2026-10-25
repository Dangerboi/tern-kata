package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestExitStatusFollowsTheCommandLineContract(t *testing.T) {
	failed := errors.New("daemon unreachable")
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"refused command line":            {usageError{failed}, 2},
		"failed subcommand":               {subcommandError{failed}, 1},
		"subcommand stopped by ctrl+c":    {interrupted{syscall.SIGINT, subcommandError{failed}}, 130},
		"subcommand stopped by terminate": {interrupted{syscall.SIGTERM, subcommandError{failed}}, 143},
		"Pane failure":                    {failed, 3},
	} {
		if got := exitStatus(tc.err); got != tc.want {
			t.Errorf("%s: exit %d, want %d", name, got, tc.want)
		}
	}
}

// complete is what bash offers for the words of a tern-kata command line, the last one being completed.
func complete(t *testing.T, words ...string) []string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	var script bytes.Buffer
	if err := writeCompletion("bash", &script); err != nil {
		t.Fatal(err)
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = "'" + w + "'"
	}
	fmt.Fprintf(&script, "COMP_WORDS=(tern-kata %s); COMP_CWORD=%d; _tern_kata; printf '%%s\\n' \"${COMPREPLY[@]}\"\n", strings.Join(quoted, " "), len(words))
	out, err := exec.Command(bash, "--norc", "-c", script.String()).Output()
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	return strings.Fields(string(out))
}

func TestBashCompletionOffersSubcommandsWorkerFormsAndFlags(t *testing.T) {
	for _, tc := range []struct {
		words []string
		want  string
	}{
		{[]string{"wo"}, "worker"},
		{[]string{"comp"}, "completion"},
		{[]string{"worker", "st"}, "status stop"},
		{[]string{"worker", "init", "--ex"}, "--executor"},
		{[]string{"worker", "status", "--"}, "--json"},
		{[]string{"recur", "ab12", "--fr"}, "--frequency"},
		{[]string{"completion", "z"}, "zsh"},
		{[]string{"attach", "ab12", "a"}, ""}, // files are the shell's own completion
	} {
		if got := strings.Join(complete(t, tc.words...), " "); got != tc.want {
			t.Errorf("tern-kata %s<tab> = %q, want %q", strings.Join(tc.words, " "), got, tc.want)
		}
	}
}
