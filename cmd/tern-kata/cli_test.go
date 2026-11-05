package main

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
		{[]string{"agent", "--"}, dashed(agentFlagNames())},
		{[]string{"agent", "--as", "bot", "--re"}, "--ref"},
		{[]string{"agent", "--as", "bot", "--ref", "demo#ab12", "--"}, dashed(agentFlagNames())},
		{[]string{"agent", "--as", "bot", "--add"}, "--address-prefix"},
		{[]string{"agent", "--ref", "demo#ab12", "--as", "bot", "--a"}, "--as --address-prefix --auth-file --allow-insecure"},
		{[]string{"agent", "--as", "bot", "--ref", "demo#ab12", "--", "omp", "--"}, ""}, // after the lone -- the words are the command's
		{[]string{"migrate-attachments", "--"}, "--as"},
		{[]string{"doc"}, "docbank"},
		{[]string{"docbank", "s"}, "setup supervised supervise"},
		{[]string{"docbank", "supervise", ""}, "on off"},
		{[]string{"docbank", "setup", "--"}, dashed(docbankFlagNames())},
		{[]string{"docbank", "setup", "--url", "https://d.example", "--key-"}, "--key-file --key-env"},
	} {
		if got := strings.Join(complete(t, tc.words...), " "); got != tc.want {
			t.Errorf("tern-kata %s<tab> = %q, want %q", strings.Join(tc.words, " "), got, tc.want)
		}
	}
}

// fishFlags are the long flags fish offers for a command line that has these words, as fish's conditions read them; flags with
// no condition (--as, --help, --version) are left out.
func fishFlags(t *testing.T, words ...string) map[string]bool {
	t.Helper()
	var script bytes.Buffer
	if err := writeCompletion("fish", &script); err != nil {
		t.Fatal(err)
	}
	seen := func(list string) bool {
		return slices.ContainsFunc(strings.Fields(list), func(w string) bool { return slices.Contains(words, w) })
	}
	flags := map[string]bool{}
	line := regexp.MustCompile(`^complete -c tern-kata (?:-n (?:'([^']*)'|(\S+)) )?.*-l (\S+)`)
	for _, l := range strings.Split(script.String(), "\n") {
		m := line.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		condition := m[1] + m[2]
		if condition == "" {
			continue
		}
		applies := true
		for _, clause := range strings.Split(condition, "; and ") {
			if clause == "not contains -- -- (commandline -opc)" { // the lone -- that ends the table flags
				if slices.Contains(words, "--") {
					applies = false
					break
				}
				continue
			}
			list, ok := strings.CutPrefix(clause, "not ")
			list, found := strings.CutPrefix(list, "__fish_seen_subcommand_from ")
			if !found {
				applies = false // __fish_use_subcommand: no subcommand typed yet
				break
			}
			if seen(list) == ok {
				applies = false
				break
			}
		}
		if applies {
			flags["--"+m[3]] = true
		}
	}
	return flags
}

func TestAgentFlagsAreInUsageHelpAndBothCompletions(t *testing.T) {
	helpEntry := usage[strings.Index(usage, "\n  agent ["):strings.Index(usage, "\n  completion <shell>")]
	var usageLine string
	for _, l := range strings.Split(usage, "\n") {
		if strings.HasPrefix(l, "  tern-kata agent ") {
			usageLine = l
		}
	}
	bash := complete(t, "agent", "--")
	fish := fishFlags(t, "agent")
	fish["--as"] = true // the global --as, which fishFlags leaves out
	for _, f := range agentFlags {
		flag := "--" + f.name
		for place, text := range map[string]string{"agentUsage": agentUsage, "the usage line": usageLine, "the help entry": helpEntry} {
			if !regexp.MustCompile(regexp.QuoteMeta(flag) + `[^-\w]`).MatchString(text) {
				t.Errorf("%s does not name %s", place, flag)
			}
		}
		if !slices.Contains(bash, flag) {
			t.Errorf("bash completion of agent does not offer %s", flag)
		}
		if !fish[flag] {
			t.Errorf("fish completion of agent does not offer %s", flag)
		}
	}
}

func TestDocbankFlagsAreInUsageHelpAndBothCompletions(t *testing.T) {
	helpEntry := usage[strings.Index(usage, "\n  docbank setup"):strings.Index(usage, "\n  agent [")]
	var usageLine string
	for _, l := range strings.Split(usage, "\n") {
		if strings.HasPrefix(l, "  tern-kata docbank ") {
			usageLine = l
		}
	}
	bash := complete(t, "docbank", "setup", "--")
	fish := fishFlags(t, "docbank", "setup")
	for _, f := range docbankFlags {
		flag := "--" + f.name
		for place, text := range map[string]string{"docbankUsage": docbankUsage, "the usage line": usageLine, "the help entry": helpEntry} {
			if !regexp.MustCompile(regexp.QuoteMeta(flag) + `[^-\w]`).MatchString(text) {
				t.Errorf("%s does not name %s", place, flag)
			}
		}
		if !slices.Contains(bash, flag) {
			t.Errorf("bash completion of docbank does not offer %s", flag)
		}
		if !fish[flag] {
			t.Errorf("fish completion of docbank does not offer %s", flag)
		}
	}
}

func TestFishCompletionOffersNoAgentFlagsAfterTheLoneDoubleDash(t *testing.T) {
	var script bytes.Buffer
	if err := writeCompletion("fish", &script); err != nil {
		t.Fatal(err)
	}
	for _, f := range agentFlags {
		if f.name == "as" { // the global --as has no condition
			continue
		}
		var found bool
		for _, l := range strings.Split(script.String(), "\n") {
			if strings.Contains(l, "__fish_seen_subcommand_from agent'") || !strings.Contains(l, "-l "+f.name+" ") && !strings.HasSuffix(l, "-l "+f.name) {
				continue
			}
			if strings.Contains(l, "__fish_seen_subcommand_from agent;") {
				found = true
				if !strings.Contains(l, "not contains -- -- (commandline -opc)") {
					t.Errorf("fish agent flag --%s has no after-- guard: %s", f.name, l)
				}
			}
		}
		if !found {
			t.Errorf("fish completion has no line for agent flag --%s", f.name)
		}
	}
	if got := fishFlags(t, "agent", "--"); len(got) != 0 {
		t.Errorf("fish offers %v after the lone --, want none", slices.Sorted(maps.Keys(got)))
	}
}

func TestBashAndFishCompletionOfferTheSameFlagsForEverySubcommand(t *testing.T) {
	for name, contexts := range map[string][][]string{
		"agent":               {{"agent"}},
		"docbank":             {{"docbank"}, {"docbank", "setup"}},
		"recur":               {{"recur", "ab12"}},
		"worker":              {{"worker"}, {"worker", "init"}, {"worker", "status"}, {"worker", "stop"}, {"worker", "resume", "x"}},
		"views":               {{"views"}, {"views", "show", "x"}, {"views", "add", "x"}, {"views", "rm", "x"}},
		"update":              {{"update"}},
		"run":                 {{"run", "x"}},
		"migrate-attachments": {{"migrate-attachments"}},
		"completion":          {{"completion", "bash"}},
	} {
		bash, fish := map[string]bool{}, map[string]bool{}
		for _, words := range contexts {
			for _, flag := range complete(t, append(slices.Clone(words), "--")...) {
				bash[flag] = true
			}
			maps.Copy(fish, fishFlags(t, words...))
		}
		delete(bash, "--as") // fish offers --as once, for every subcommand
		if !maps.Equal(bash, fish) {
			t.Errorf("%s: bash offers %v, fish offers %v", name, slices.Sorted(maps.Keys(bash)), slices.Sorted(maps.Keys(fish)))
		}
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
