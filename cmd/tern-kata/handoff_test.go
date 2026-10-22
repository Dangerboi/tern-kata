package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
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
			got, kept := editDraft(model.OpenDraftEditor{Text: "original"}, fakeEditor(tt.script))
			if kept != "" {
				os.Remove(kept)
			}
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

func TestEditDraftGivesTheEditorAFileWithTheRequestedExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editors use sh")
	}
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EDITOR", "x")
	for suffix, want := range map[string]string{"": ".md", ".json": ".json"} {
		got, kept := editDraft(model.OpenDraftEditor{Text: "original", Suffix: suffix}, fakeEditor(`printf '%s' "$1" > "$1"`))
		os.Remove(kept)
		if got.Err != nil || !strings.HasSuffix(got.Text, want) {
			t.Errorf("suffix %q: editor was given %q (err %v), want a %s file", suffix, got.Text, got.Err, want)
		}
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
	got, kept := editDraft(model.OpenDraftEditor{Text: "original"}, editorCommand)
	os.Remove(kept)
	if got.Err != nil || got.Text != "one" {
		t.Errorf("got text %q err %v", got.Text, got.Err)
	}
}

func TestChildEnv(t *testing.T) {
	tests := []struct {
		name            string
		environ         []string
		caseInsensitive bool
		want            []string
	}{
		{"none, sensitive", []string{"PATH=/bin"}, false, []string{"PATH=/bin", "KATA_AUTHOR=ann"}},
		{"none, insensitive", []string{"PATH=/bin"}, true, []string{"PATH=/bin", "KATA_AUTHOR=ann"}},
		{"one, sensitive", []string{"PATH=/bin", "KATA_AUTHOR=old"}, false, []string{"PATH=/bin", "KATA_AUTHOR=ann"}},
		{"one, insensitive", []string{"PATH=/bin", "Kata_Author=old"}, true, []string{"PATH=/bin", "KATA_AUTHOR=ann"}},
		{"other spelling kept when sensitive", []string{"kata_author=old", "KATA_AUTHOR=old"}, false, []string{"kata_author=old", "KATA_AUTHOR=ann"}},
		{"several, insensitive", []string{"KATA_AUTHOR=a", "HOME=/h", "kata_author=b", "Kata_Author=c"}, true, []string{"HOME=/h", "KATA_AUTHOR=ann"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := childEnv(tt.environ, "ann", tt.caseInsensitive); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("childEnv = %q, want %q", got, tt.want)
			}
		})
	}
	environ := []string{"KATA_AUTHOR=old"}
	if got := childEnv(environ, "", true); !reflect.DeepEqual(got, environ) {
		t.Errorf("no override: childEnv = %q, want unchanged", got)
	}
}

type stubSession struct{}

func (stubSession) Next(ctx context.Context) (tern.Input, error) { <-ctx.Done(); return nil, ctx.Err() }
func (stubSession) Caps() tern.Caps                              { return tern.Caps{} }
func (stubSession) Close() error                                 { return nil }

type stubSurface struct{}

func (stubSurface) Render(tern.View) error { return nil }
func (stubSurface) Send(...tern.Op) error  { return nil }
func (stubSurface) Close() error           { return nil }

func TestEditInTerminalKeepsTextWhenResumeFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editors use sh")
	}
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EDITOR", "x")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fail := func(context.Context) (paneSession, paneSurface, error) { return nil, nil, errors.New("tern gone") }
	term := newTerminal(ctx, stubSession{}, stubSurface{}, fail)
	_, err := editInTerminal(term, model.OpenDraftEditor{Text: "original"}, fakeEditor(`printf edited > "$1"`))
	if err == nil || !strings.Contains(err.Error(), "tern gone") {
		t.Fatalf("err = %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(os.TempDir(), "tern-kata-Draft-*.md"))
	if len(files) != 1 || !strings.Contains(err.Error(), files[0]) {
		t.Fatalf("files = %v, error = %v", files, err)
	}
	if text, _ := os.ReadFile(files[0]); string(text) != "edited" {
		t.Errorf("kept text = %q", text)
	}
}

func TestEditInTerminalRemovesFileWhenResumeWorks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editors use sh")
	}
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EDITOR", "x")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ok := func(context.Context) (paneSession, paneSurface, error) { return stubSession{}, stubSurface{}, nil }
	term := newTerminal(ctx, stubSession{}, stubSurface{}, ok)
	got, err := editInTerminal(term, model.OpenDraftEditor{Text: "original"}, fakeEditor(`printf edited > "$1"`))
	if err != nil || got.Text != "edited" {
		t.Fatalf("got %q, %v", got.Text, err)
	}
	if left, _ := filepath.Glob(filepath.Join(os.TempDir(), "*")); len(left) != 0 {
		t.Errorf("leftover files: %v", left)
	}
}
