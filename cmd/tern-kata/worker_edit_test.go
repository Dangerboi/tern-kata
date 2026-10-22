package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scriptedEditor installs an $EDITOR shell script that runs body with the file as $1, and returns the file where it logs
// what it was handed (one "---" separated entry per run).
func scriptedEditor(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "seen")
	script := filepath.Join(dir, "editor.sh")
	src := "#!/bin/sh\n{ cat \"$1\"; echo ---; } >> '" + log + "'\n" + body + "\n"
	if err := os.WriteFile(script, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", script)
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
	scriptedEditor(t, `sed -i 's/^executor = .*/executor = "renamed"/' "$1"`)
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
	counter := filepath.Join(t.TempDir(), "n")
	log := scriptedEditor(t, `if [ -e '`+counter+`' ]; then grep -v '^#!' "$1" | grep -v '^bogus' > "$1.new"; mv "$1.new" "$1"; sed -i 's/^executor = .*/executor = "second"/' "$1"; else : > '`+counter+`'; echo 'bogus = 1' | cat - "$1" > "$1.new"; mv "$1.new" "$1"; fi`)
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

func TestWorkerEditCancelsOnAnUnchangedOrEmptyResult(t *testing.T) {
	for name, body := range map[string]string{"unchanged": ":", "empty": `: > "$1"`} {
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
	scriptedEditor(t, ":")
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
