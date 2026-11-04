package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
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

func TestKataTextReachesTheTerminalWithoutControlSequences(t *testing.T) {
	var out bytes.Buffer
	text := "Fix \x1b]0;owned\x07title\x1b[2J é \u009b31m\tdone\n"
	if _, err := fmt.Fprint(printableWriter{&out}, text); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Fix \ufffd]0;owned\ufffdtitle\ufffd[2J é \ufffd31m\tdone\n"; got != want {
		t.Fatalf("printed %q, want %q", got, want)
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
		{[]string{"vi"}, "views"},
		{[]string{"views", "a"}, "add"},
		{[]string{"views", "show", "x", "--"}, "--json --as"},
		{[]string{"views", "add", "x", "--re"}, "--relationship --replace"},
		{[]string{"completion", "z"}, "zsh"},
		{[]string{"agent", "--"}, "--as --ref --inbox --home --db --schema --dsn-file --env-file --daemon --server --token-file --auth-file --allow-insecure --trust-private-network"},
		{[]string{"agent", "--as", "bot", "--re"}, "--ref"},
		{[]string{"agent", "--as", "bot", "--ref", "demo#ab12", "--"}, "--as --ref --inbox --home --db --schema --dsn-file --env-file --daemon --server --token-file --auth-file --allow-insecure --trust-private-network"},
		{[]string{"agent", "--as", "bot", "--i"}, "--inbox"},
		{[]string{"agent", "--ref", "demo#ab12", "--as", "bot", "--a"}, "--as --auth-file --allow-insecure"},
		{[]string{"agent", "--as", "bot", "--ref", "demo#ab12", "--", "omp", "--"}, ""}, // after the lone -- the words are the command's
		{[]string{"attach", "ab12", "a"}, ""},                                           // files are the shell's own completion
	} {
		if got := strings.Join(complete(t, tc.words...), " "); got != tc.want {
			t.Errorf("tern-kata %s<tab> = %q, want %q", strings.Join(tc.words, " "), got, tc.want)
		}
	}
}

func TestFishCompletionOffersTheAgentRef(t *testing.T) {
	var script bytes.Buffer
	if err := writeCompletion("fish", &script); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script.String(), "__fish_seen_subcommand_from agent' -l ref") {
		t.Errorf("fish completion has no --ref for agent:\n%s", script.String())
	}
}

// fullDisk is stdout redirected to /dev/full: every write fails.
type fullDisk struct{}

func (fullDisk) Write([]byte) (int, error) { return 0, syscall.ENOSPC }

func TestSubcommandOutputFailuresExitOne(t *testing.T) {
	for name, plan := range map[string]LaunchPlan{
		"version":    {Version: "1.2.3"},
		"help":       {Help: true},
		"completion": {Subcommand: "completion", Operands: []string{"bash"}},
	} {
		handled, err := printInfo(plan, printableWriter{fullDisk{}})
		if !handled {
			t.Fatalf("%s: not handled", name)
		}
		if got := exitStatus(err); got != 1 {
			t.Errorf("%s: exit %d for %v, want 1", name, got, err)
		}
	}
	dirs := worker.Dirs{Config: filepath.Join(t.TempDir(), "tern-kata")}
	if err := os.MkdirAll(dirs.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirs.ConfigPath(), []byte("actor = \"robot\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runWorkerCommand(t.Context(), LaunchPlan{Subcommand: "worker", Operands: []string{"supervised"}}, workerEnv{Dirs: dirs}, printableWriter{fullDisk{}})
	if got := exitStatus(err); got != 1 {
		t.Errorf("worker output: exit %d for %v, want 1", got, err)
	}
}
