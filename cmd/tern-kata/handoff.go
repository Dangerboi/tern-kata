package main

import (
	"context"
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

// handOff gives the terminal to $EDITOR for the Draft and returns the editor result with the resumed Tern session pieces.
func handOff(ctx context.Context, draft model.OpenDraftEditor, session *tern.Session, surface *tern.Surface, stopInput func(), inputs chan<- any, inputErrors chan<- error) (model.DraftEditorClosed, *tern.Session, *tern.Surface, func(), error) {
	var none model.DraftEditorClosed
	stopInput()
	surface.Close()
	// The Session must be closed first: Suspend leaves its stdin pump and raw mode active.
	session.Close()
	result := editDraft(draft, editorCommand)
	next, err := tern.Connect(ctx, editorOptions())
	if err != nil {
		return none, nil, nil, nil, err
	}
	nextSurface, err := openSurface(next)
	if err != nil {
		next.Close()
		return none, nil, nil, nil, err
	}
	return result, next, nextSurface, startInputReader(ctx, next, inputs, inputErrors), nil
}

// editDraft round-trips the Draft text through a temp file edited by the command build returns.
func editDraft(cmd model.OpenDraftEditor, build func(editor, file string) *exec.Cmd) model.DraftEditorClosed {
	result := model.DraftEditorClosed{UID: cmd.UID, Text: cmd.Text}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		result.Err = fmt.Errorf("set $EDITOR to edit the Draft externally")
		return result
	}
	file, err := os.CreateTemp("", "tern-kata-Draft-*.md")
	if err != nil {
		result.Err = err
		return result
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(cmd.Text)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		result.Err = err
		return result
	}
	process := build(editor, file.Name())
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		result.Err = fmt.Errorf("$EDITOR: %w; original Draft kept", err)
		return result
	}
	text, err := os.ReadFile(file.Name())
	if err != nil {
		result.Err = err
		return result
	}
	result.Text = string(text)
	return result
}
