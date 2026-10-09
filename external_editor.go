package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func editorOptions() tern.Options {
	return tern.Options{App: "tern-kata", Version: "dev", Features: []string{tern.FeatureEdit, tern.FeatureUndo}}
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
	if err := surface.Stylesheet("tern-kata", view.StripeCSS+"\n"+view.SelectedCSS); err != nil {
		surface.Close()
		return nil, err
	}
	return surface, nil
}

func editExternally(cmd model.OpenDraftEditor) model.DraftEditorClosed {
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
	// The Session must be closed first: Suspend leaves its stdin pump and raw mode active.
	process := exec.Command("sh", "-c", editor+" \"$1\"", "tern-kata-editor", file.Name())
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	err = process.Run()
	if err != nil {
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
