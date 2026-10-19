package main

import (
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

func editorOptions() tern.Options {
	return tern.Options{App: "tern-kata", Version: buildVersion(), Features: []string{tern.FeatureEdit, tern.FeatureUndo}}
}
func editorSurfaceOptions() tern.SurfaceOptions {
	return tern.SurfaceOptions{ID: "kata", Mode: tern.Screen, Title: "tern-kata", Discard: true}
}

// openSurface opens the window's surface and installs the plugin's style sheet on it.
func openSurface(session *tern.Session) (*tern.Surface, error) {
	surface, err := session.Open(editorSurfaceOptions())
	if err != nil {
		return nil, err
	}
	if err := surface.Stylesheet("tern-kata", view.CSS); err != nil {
		surface.Close()
		return nil, err
	}
	return surface, nil
}

// editInTerminal gives the terminal to $EDITOR for the Draft and resumes the Pane. When resuming fails after the
// editor succeeded, the edited text stays in its file, whose path is printed and named in the error.
func editInTerminal(t *terminal, draft model.OpenDraftEditor, build func(editor, file string) *exec.Cmd) (model.DraftEditorClosed, error) {
	var result model.DraftEditorClosed
	var kept string
	err := t.Suspend(func() { result, kept = editDraft(draft, build) })
	if err != nil && kept != "" {
		fmt.Fprintf(os.Stderr, "tern-kata: could not reconnect to Tern; your edited Draft is kept in %s\n", kept)
		return result, fmt.Errorf("%w; your edited Draft is kept in %s", err, kept)
	}
	if kept != "" {
		os.Remove(kept)
	}
	return result, err
}

// editDraft round-trips the Draft text through a temp file edited by the command build returns. It returns the file's
// path only when the editor succeeded; the caller removes it once the text is safe.
func editDraft(cmd model.OpenDraftEditor, build func(editor, file string) *exec.Cmd) (model.DraftEditorClosed, string) {
	result := model.DraftEditorClosed{UID: cmd.UID, Text: cmd.Text}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		result.Err = fmt.Errorf("set $EDITOR to edit the Draft externally")
		return result, ""
	}
	file, err := os.CreateTemp("", "tern-kata-Draft-*.md")
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
		result.Err = fmt.Errorf("$EDITOR: %w; original Draft kept", err)
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
