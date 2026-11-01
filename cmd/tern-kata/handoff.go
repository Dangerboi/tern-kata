package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// childEnv returns environ for a child with KATA_AUTHOR set to actor, dropping every existing spelling first;
// names compare case-insensitively on Windows. An empty actor leaves environ unchanged.
func childEnv(environ []string, actor string, caseInsensitive bool) []string {
	if actor == "" {
		return environ
	}
	env := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if name == "KATA_AUTHOR" || caseInsensitive && strings.EqualFold(name, "KATA_AUTHOR") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "KATA_AUTHOR="+actor)
}

// agentEnv returns environ for an agent: childEnv's KATA_AUTHOR, no inherited KATA_REF or KATA_INBOX_USER (the person's own inbox
// is not the agent's), and KATA_REF set to ref when it is not empty.
func agentEnv(environ []string, actor, ref string, caseInsensitive bool) []string {
	env := childEnv(environ, actor, caseInsensitive)
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if dropsFromAgent(name, caseInsensitive) {
			continue
		}
		out = append(out, entry)
	}
	if ref != "" {
		out = append(out, "KATA_REF="+ref)
	}
	return out
}

func dropsFromAgent(name string, caseInsensitive bool) bool {
	for _, dropped := range [...]string{"KATA_REF", "KATA_INBOX_USER"} {
		if name == dropped || caseInsensitive && strings.EqualFold(name, dropped) {
			return true
		}
	}
	return false
}

func editorOptions() tern.Options {
	return tern.Options{App: "tern-kata", Version: buildVersion(), Features: []string{tern.FeatureEdit, tern.FeatureUndo}}
}

// editorSurfaceOptions are the Pane's surface options: Tern's sans face, in which machine strings take the mono span token.
// closable lets Tern close the Pane without asking; it is set only when kata's [tui].confirm_quit is false, so a Pane that
// asks before q quits is not lost to a close without warning.
func editorSurfaceOptions(closable bool) tern.SurfaceOptions {
	return tern.SurfaceOptions{ID: "kata", Mode: tern.Screen, Title: "tern-kata", Font: tern.FontSans, Discard: true, Closable: closable}
}

// openSurface opens the window's surface and installs the plugin's style sheet on it.
func openSurface(session *tern.Session, closable bool) (*tern.Surface, error) {
	surface, err := session.Open(editorSurfaceOptions(closable))
	if err != nil {
		return nil, err
	}
	if err := surface.Stylesheet("tern-kata", view.CSS); err != nil {
		surface.Close()
		return nil, err
	}
	return surface, nil
}

// resolveEditor is the editor command: $VISUAL, else $EDITOR, else vi as kata's own TUI falls back to. With none set and
// no vi on PATH it refuses, naming the variables to set.
func resolveEditor() (string, error) {
	if editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR")); editor != "" {
		return editor, nil
	}
	if _, err := exec.LookPath("vi"); err != nil {
		return "", errors.New("no editor to open: set $VISUAL or $EDITOR")
	}
	return "vi", nil
}

// editInTerminal gives the terminal to the editor for the Draft and resumes the Pane. With no editor to run the Pane stays
// as it is and the refusal comes back as the result. When resuming fails after the editor succeeded, the edited text
// stays in its file, whose path is printed and named in the error.
func editInTerminal(t *terminal, draft model.OpenDraftEditor, build func(editor, file string) *exec.Cmd) (model.DraftEditorClosed, error) {
	editor, err := resolveEditor()
	if err != nil {
		return model.DraftEditorClosed{UID: draft.UID, Text: draft.Text, Err: err}, nil //nolint:nilerr // the refusal is the edit's result for the card to show; the Pane carries on
	}
	var result model.DraftEditorClosed
	var kept string
	err = t.Suspend(func() { result, kept = editDraft(draft, editor, build) })
	if err != nil && kept != "" {
		fmt.Fprintf(os.Stderr, "tern-kata: could not reconnect to Tern; your edited Draft is kept in %s\n", kept)
		return result, fmt.Errorf("%w; your edited Draft is kept in %s", err, kept)
	}
	if kept != "" {
		os.Remove(kept)
	}
	return result, err
}

// editDraft round-trips the Draft text through a temp file edited by the command build returns for editor. It returns
// the file's path only when the editor succeeded; the caller removes it once the text is safe.
func editDraft(cmd model.OpenDraftEditor, editor string, build func(editor, file string) *exec.Cmd) (model.DraftEditorClosed, string) {
	result := model.DraftEditorClosed{UID: cmd.UID, Text: cmd.Text}
	suffix := cmp.Or(cmd.Suffix, ".md")
	file, err := os.CreateTemp("", "tern-kata-Draft-*"+suffix)
	if err != nil {
		result.Err = err
		return result, ""
	}
	_, err = file.WriteString(cmd.Text)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(file.Name())
		result.Err = err
		return result, ""
	}
	process := build(editor, file.Name())
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		os.Remove(file.Name())
		result.Err = fmt.Errorf("editor %s: %w; original text kept", editor, err)
		return result, ""
	}
	text, err := os.ReadFile(file.Name())
	if err != nil {
		os.Remove(file.Name())
		result.Err = err
		return result, ""
	}
	result.Text = string(text)
	return result, file.Name()
}
