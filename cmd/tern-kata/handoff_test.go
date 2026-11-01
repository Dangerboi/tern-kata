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

	"github.com/Dangerboi/tern-kata/internal/data"
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
		script  string
		want    string
		wantErr string
	}{
		{"edit replaces text", `printf edited > "$1"`, "edited", ""},
		{"failing editor keeps original", `exit 3`, "original", "editor x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			got, kept := editDraft(model.OpenDraftEditor{Text: "original"}, "x", fakeEditor(tt.script))
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

// The editor is $VISUAL, else $EDITOR, else vi; with none of them the Pane is never handed over, so it does not blink.
func TestEditInTerminalPicksVisualThenEditorThenVi(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editors use sh")
	}
	withVi := t.TempDir()
	if err := os.WriteFile(filepath.Join(withVi, "vi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, visual, editor, path, want string
	}{
		{"VISUAL first", "v", "e", withVi, "v"},
		{"then EDITOR", "", "e", withVi, "e"},
		{"then vi", "", "", withVi, "vi"},
		{"none", "", "", t.TempDir(), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)
			t.Setenv("PATH", tt.path)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			suspended := false
			resume := func(context.Context) (paneSession, paneSurface, error) {
				suspended = true
				return stubSession{}, stubSurface{}, nil
			}
			var ran string
			build := func(editor, file string) *exec.Cmd {
				ran = editor
				return exec.Command("/bin/sh", "-c", `printf edited > "$1"`, "fake", file)
			}
			got, err := editInTerminal(newTerminal(ctx, stubSession{}, stubSurface{}, resume), model.OpenDraftEditor{UID: "w", Text: "original"}, build)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if suspended || got.Err == nil || !strings.Contains(got.Err.Error(), "$VISUAL or $EDITOR") || got.Text != "original" || got.UID != "w" {
					t.Errorf("suspended %v, result %#v; want the Pane kept and a refusal naming $VISUAL and $EDITOR", suspended, got)
				}
				return
			}
			if ran != tt.want || got.Text != "edited" || !suspended {
				t.Errorf("ran %q (text %q, suspended %v), want %q", ran, got.Text, suspended, tt.want)
			}
		})
	}
}

func TestEditDraftGivesTheEditorAFileWithTheRequestedExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editors use sh")
	}
	t.Setenv("TMPDIR", t.TempDir())
	for suffix, want := range map[string]string{"": ".md", ".json": ".json"} {
		got, kept := editDraft(model.OpenDraftEditor{Text: "original", Suffix: suffix}, "x", fakeEditor(`printf '%s' "$1" > "$1"`))
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
	got, kept := editDraft(model.OpenDraftEditor{Text: "original"}, `sh -c 'test $# -eq 1 && printf one > "$1"' editor`, editorCommand)
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

func TestAgentEnv(t *testing.T) {
	environ := []string{"PATH=/bin", "KATA_REF=old#1", "KATA_INBOX_USER=me", "KATA_AUTHOR=old", "Kata_Ref=x", "KATA_SESSION_ID=s"}
	if got, want := agentEnv(environ, "ann", "demo#ab12", false), []string{"PATH=/bin", "Kata_Ref=x", "KATA_SESSION_ID=s", "KATA_AUTHOR=ann", "KATA_REF=demo#ab12"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sensitive: %q, want %q", got, want)
	}
	if got, want := agentEnv(environ, "ann", "demo#ab12", true), []string{"PATH=/bin", "KATA_SESSION_ID=s", "KATA_AUTHOR=ann", "KATA_REF=demo#ab12"}; !reflect.DeepEqual(got, want) {
		t.Errorf("insensitive: %q, want %q", got, want)
	}
	if got, want := agentEnv(environ, "ann", "", false), []string{"PATH=/bin", "Kata_Ref=x", "KATA_SESSION_ID=s", "KATA_AUTHOR=ann"}; !reflect.DeepEqual(got, want) {
		t.Errorf("no ref: %q, want %q", got, want)
	}
	if got, want := agentEnv(environ, "", "demo#ab12", false), []string{"PATH=/bin", "KATA_AUTHOR=old", "Kata_Ref=x", "KATA_SESSION_ID=s", "KATA_REF=demo#ab12"}; !reflect.DeepEqual(got, want) {
		t.Errorf("no actor: %q, want %q", got, want)
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

func TestPaneClosableFollowsConfirmQuit(t *testing.T) {
	for name, tc := range map[string]struct {
		catalog data.Catalog
		err     error
		want    bool
	}{
		"confirm_quit false": {data.Catalog{ConfirmQuit: false}, nil, true},
		"confirm_quit true":  {data.Catalog{ConfirmQuit: true}, nil, false},
		"unreadable config":  {data.Catalog{}, errors.New("bad"), false},
	} {
		if got := editorSurfaceOptions(quitsAtOnce(tc.catalog, tc.err)).Closable; got != tc.want {
			t.Errorf("%s: Closable = %v, want %v", name, got, tc.want)
		}
	}
}
