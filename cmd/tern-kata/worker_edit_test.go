package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The scripted editor is this test binary re-run as $EDITOR, so it works the same under `sh -c` and Windows `cmd /c`
// without a shell script. The mode and log path travel in the environment the editor inherits.
const (
	editorModeEnv = "TERN_KATA_TEST_EDITOR_MODE"
	editorLogEnv  = "TERN_KATA_TEST_EDITOR_LOG"
	// editorConfigEnv is the worker.toml the "clash" and "break" modes touch while the edit is open.
	editorConfigEnv = "TERN_KATA_TEST_EDITOR_CONFIG"
)

var executorLine = regexp.MustCompile(`(?m)^executor = .*$`)

// TestScriptedEditorProcess is the editor body. It is a no-op in a normal test run.
func TestScriptedEditorProcess(t *testing.T) {
	mode := os.Getenv(editorModeEnv)
	if mode == "" {
		return
	}
	file := os.Args[len(os.Args)-1]
	raw, err := os.ReadFile(file)
	if err != nil {
		os.Exit(2)
	}
	text := string(raw)
	logf, err := os.OpenFile(os.Getenv(editorLogEnv), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(2)
	}
	_, _ = logf.WriteString(text + "---\n") // a lost log line fails the test that reads it
	logf.Close()

	var out string
	switch mode {
	case "rename":
		out = executorLine.ReplaceAllString(text, `executor = "renamed"`)
	case "clash", "break":
		// The edit is renamed, and worker.toml is changed (clash) or turned into a folder, so no write can succeed (break).
		out = executorLine.ReplaceAllString(text, `executor = "renamed"`)
		config := os.Getenv(editorConfigEnv)
		os.Remove(config)
		if mode == "break" {
			_ = os.Mkdir(config, 0o700) // a failure leaves the write possible, which the test then catches
		} else {
			_ = os.WriteFile(config, []byte(executorLine.ReplaceAllString(text, `executor = "theirs"`)), 0o600) // as above
		}
	case "unchanged":
		out = text
	case "empty":
		out = ""
	case "bogus-then-fix":
		// First run adds an unknown key; the reopened run (it carries the "#! " problem comment) drops it and the comment.
		if !strings.HasPrefix(text, "#! ") {
			out = "bogus = 1\n" + text
			break
		}
		var kept []string
		for _, line := range strings.Split(text, "\n") {
			if !strings.HasPrefix(line, "#!") && !strings.HasPrefix(line, "bogus") {
				kept = append(kept, line)
			}
		}
		out = executorLine.ReplaceAllString(strings.Join(kept, "\n"), `executor = "second"`)
	default:
		os.Exit(2)
	}
	if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

// scriptedEditor installs the test binary as $EDITOR in the given mode and returns the file where it logs what it was
// handed (one "---" separated entry per run).
func scriptedEditor(t *testing.T, mode string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "seen")
	t.Setenv(editorModeEnv, mode)
	t.Setenv(editorLogEnv, log)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `"`+exe+`" -test.run=^TestScriptedEditorProcess$ --`)
	return log
}

func initedEnv(t *testing.T) workerEnv {
	t.Helper()
	env := setupEnv(t, boundCheckout(t, "demo"))
	if _, err := runSetup(t, env, "init"); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestWorkerEditWritesAValidEditAndSaysHowToReload(t *testing.T) {
	env := initedEnv(t)
	scriptedEditor(t, "rename")
	out, err := runSetup(t, env, "edit")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readConfig(t, env), `executor = "renamed"`) {
		t.Errorf("not written:\n%s", readConfig(t, env))
	}
	for _, want := range []string{"No worker is running"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestWorkerEditReopensARefusedEditWithTheProblemAtTheTop(t *testing.T) {
	env := initedEnv(t)
	// First run adds an unknown key; second run removes it again, after seeing the problem comment.
	log := scriptedEditor(t, "bogus-then-fix")
	if _, err := runSetup(t, env, "edit"); err != nil {
		t.Fatal(err)
	}
	seen := strings.Split(string(mustRead(t, log)), "---\n")
	if len(seen) < 3 {
		t.Fatalf("editor ran %d times", len(seen)-1)
	}
	if !strings.HasPrefix(seen[1], "#! ") || !strings.Contains(seen[1], "bogus") {
		t.Errorf("reopened file lacks the problem at the top:\n%s", seen[1])
	}
	got := readConfig(t, env)
	if !strings.Contains(got, `executor = "second"`) || strings.Contains(got, "bogus") || strings.Contains(got, "#!") {
		t.Errorf("file:\n%s", got)
	}
}

// keptCopy is the path a refused write names as holding the edited copy; the file is removed with the test.
func keptCopy(t *testing.T, err error) string {
	t.Helper()
	_, path, ok := strings.Cut(err.Error(), "your edit is kept in ")
	if !ok {
		t.Fatalf("error %q names no kept copy", err)
	}
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestWorkerEditKeepsTheEditedCopyWhenTheWriteFails(t *testing.T) {
	env := initedEnv(t)
	scriptedEditor(t, "break")
	t.Setenv(editorConfigEnv, env.Dirs.ConfigPath())
	_, err := runSetup(t, env, "edit")
	if err == nil {
		t.Fatal("a failed write reported success")
	}
	if got := string(mustRead(t, keptCopy(t, err))); !strings.Contains(got, `executor = "renamed"`) {
		t.Errorf("the kept copy:\n%s", got)
	}
}

func TestWorkerEditRefusesAFileThatChangedWhileEditingAndKeepsTheCopy(t *testing.T) {
	env := initedEnv(t)
	scriptedEditor(t, "clash")
	t.Setenv(editorConfigEnv, env.Dirs.ConfigPath())
	_, err := runSetup(t, env, "edit")
	if err == nil || !strings.Contains(err.Error(), "changed while you edited it") {
		t.Fatalf("error %v, want the changed file named", err)
	}
	if got := string(mustRead(t, keptCopy(t, err))); !strings.Contains(got, `executor = "renamed"`) {
		t.Errorf("the kept copy:\n%s", got)
	}
	if !strings.Contains(readConfig(t, env), `executor = "theirs"`) {
		t.Errorf("the other change was lost:\n%s", readConfig(t, env))
	}
}

func TestWorkerEditCancelsOnAnUnchangedOrEmptyResult(t *testing.T) {
	for name, body := range map[string]string{"unchanged": "unchanged", "empty": "empty"} {
		t.Run(name, func(t *testing.T) {
			env := initedEnv(t)
			before := readConfig(t, env)
			scriptedEditor(t, body)
			out, err := runSetup(t, env, "edit")
			if err != nil {
				t.Fatal(err)
			}
			if readConfig(t, env) != before {
				t.Error("the file changed")
			}
			if !strings.Contains(out, "No change") {
				t.Errorf("output %q", out)
			}
		})
	}
}

func TestWorkerEditWithoutAConfigPointsAtInit(t *testing.T) {
	env := setupEnv(t, t.TempDir())
	scriptedEditor(t, "unchanged")
	if _, err := runSetup(t, env, "edit"); err == nil || !strings.Contains(err.Error(), "worker init") {
		t.Fatalf("error %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestWorkerEditWithAWorkerRunningSaysStopMakesItReload(t *testing.T) {
	f := newWorkerFixture(t, "")
	f.serve(t)
	scriptedEditor(t, "rename")
	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"edit"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Wrote", "tern-kata worker stop", "reload"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No worker is running") {
		t.Errorf("a running worker was reported absent:\n%s", out)
	}
}
