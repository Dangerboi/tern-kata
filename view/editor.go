package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const DraftEditorID = "layer.editor.body"

// TextEditor presents the reusable buffer; the model owns text, selection and undo.
func TextEditor(key string, editor model.TextEditor, readonly bool) tern.Element {
	return ui.Editor{Common: ui.Common{Key: key, Tone: ui.ToneAccent}, Field: ui.Field{Text: editor.Text, Cursor: &editor.Cursor, Anchor: &editor.Anchor, Readonly: readonly}, MaxLines: 24}
}

func editorLayer(m model.Model) tern.Element {
	draft, ok := m.ActiveDraft()
	if !ok {
		return help(m)
	}
	title := "Edit Issue body"
	if draft.Comment {
		title = "Add comment"
	}
	children := ui.Nodes(TextEditor("body", draft.Editor, draft.Saving || draft.DiscardConfirm || draft.Conflict != nil))
	if draft.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Saving Draft…"})
	}
	if draft.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: draft.Err.Error()})
	}
	if draft.DiscardConfirm {
		children = append(children, ui.Text{Common: ui.Common{Key: "discard", Tone: ui.ToneWarning}, Text: "Discard this Draft? y discards; n or esc keeps editing."})
	}
	if draft.Conflict != nil {
		title = "Issue changed: Draft kept"
		children = ui.Nodes(ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
			ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("Your Draft (base revision %d)", draft.BaseRevision)}, TextEditor("body", draft.Editor, true))},
			ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("New version (revision %d)", draft.Conflict.Revision)}, ui.Md{Text: draft.Conflict.Body})},
		)}, ui.Text{Common: ui.Common{Key: "choice", Tone: ui.ToneWarning}, Text: "o overwrites against the new revision; d discards only your Draft."})
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "editor"}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}
