package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// fakeEditor builds a command that runs script with the temp file path as $1.
func fakeEditor(script string) func(editor, file string) *exec.Cmd {
	return func(_, file string) *exec.Cmd {
		return exec.Command("sh", "-c", script, "fake", file)
	}
}

func TestEditDraft(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editors use sh")
	}
	tests := []struct {
		name    string
		editor  string
		script  string
		want    string
		wantErr string
	}{
		{"edit replaces text", "x", `printf edited > "$1"`, "edited", ""},
		{"failing editor keeps original", "x", `exit 3`, "original", "$EDITOR"},
		{"unset editor", "", `printf edited > "$1"`, "original", "$EDITOR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			t.Setenv("EDITOR", tt.editor)
			got := editDraft(model.OpenDraftEditor{Text: "original"}, fakeEditor(tt.script))
			if got.Text != tt.want {
				t.Errorf("text = %q, want %q", got.Text, tt.want)
			}
			if (tt.wantErr == "") != (got.Err == nil) || (got.Err != nil && !strings.Contains(got.Err.Error(), tt.wantErr)) {
				t.Errorf("err = %v, want containing %q", got.Err, tt.wantErr)
			}
			if left, _ := filepath.Glob(filepath.Join(tmp, "*")); len(left) != 0 {
				t.Errorf("leftover files: %v", left)
			}
		})
	}
}

func TestEditDraftSpacedTempPathThroughShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exercises the Unix sh -c adapter")
	}
	tmp := filepath.Join(t.TempDir(), "with space")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	t.Setenv("EDITOR", `sh -c 'test $# -eq 1 && printf one > "$1"' editor`)
	got := editDraft(model.OpenDraftEditor{Text: "original"}, editorCommand)
	if got.Err != nil || got.Text != "one" {
		t.Errorf("got text %q err %v", got.Text, got.Err)
	}
}
