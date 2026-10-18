package view

import (
	"fmt"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const DraftEditorID = "layer.editor.body"

// BodyTarget is the body a Tern editor node edits: the Draft editor's or the Create card's, none for any other node.
func BodyTarget(id string) model.EditTarget {
	switch id {
	case DraftEditorID:
		return model.EditDraftBody
	case CreateFieldID(model.CreateBody):
		return model.EditCreateBody
	}
	return model.NoEditTarget
}

// TextEditor presents the reusable buffer; the model owns text, selection and undo.
func TextEditor(key string, editor model.TextEditor, readonly bool) tern.Element {
	return ui.Editor{Common: ui.Common{Key: key, Tone: ui.ToneAccent}, Field: ui.Field{Text: editor.Text, Cursor: &editor.Cursor, Anchor: &editor.Anchor, Readonly: readonly}, MaxLines: 24}
}

func editorLayer(m model.Model) tern.Element {
	draft, ok := m.ActiveDraft()
	if !ok {
		return nil
	}
	title := "Edit Issue body"
	if draft.Comment {
		title = "Add comment"
	}
	if draft.ReplyTo != "" {
		title = replyTitle(draft)
	}
	children := ui.Nodes(TextEditor("body", draft.Editor, draft.Saving || draft.DiscardConfirm || draft.Conflict != nil))
	if draft.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Saving Draft…"})
	}
	if draft.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: draft.Err.Error()})
	}
	switch {
	case draft.DiscardConfirm:
		children = append(children, ui.Text{Common: ui.Common{Key: "discard", Tone: ui.ToneWarning}, Text: "Discard this Draft? y discards; n or esc keeps editing."}, discardButtons())
	case !draft.Saving && draft.Conflict == nil:
		buttons := []tern.Element{keyButton("save", "Save (ctrl+o)", "ctrl+o", true)}
		if draft.Duplicate() {
			buttons = append(buttons, keyButton("force", "Send anyway (ctrl+f)", "ctrl+f", false))
		}
		if draft.ReplyTo != "" {
			buttons = append(buttons, keyButton("kind", "Kind (ctrl+r)", "ctrl+r", false))
		}
		buttons = append(buttons, keyButton("editor", "$EDITOR (ctrl+e)", "ctrl+e", false), keyButton("discard", "Discard (esc)", "escape", false))
		children = append(children, buttonRow(buttons...))
	}
	if draft.Conflict != nil {
		title = "Issue changed: Draft kept"
		children = ui.Nodes(ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
			ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("Your Draft (base revision %d)", draft.BaseRevision)}, TextEditor("body", draft.Editor, true))},
			ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("New version (revision %d)", draft.Conflict.Revision)}, ui.Md{Text: draft.Conflict.Body})},
		)}, ui.Text{Common: ui.Common{Key: "choice", Tone: ui.ToneWarning}, Text: "o overwrites against the new revision; d discards only your Draft."},
			buttonRow(keyButton("overwrite", "Overwrite (o)", "o", true), keyButton("discard", "Discard Draft (d)", "d", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "editor"}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}

// replyTitle names the kind of a Reply Draft and whose comment it answers, e.g. "Confirm dana's comment".
func replyTitle(draft model.Draft) string {
	whose := "a comment"
	if draft.ReplyAuthor != "" {
		whose = draft.ReplyAuthor + "'s comment"
	}
	kind := string(draft.ReplyKind)
	if draft.ReplyKind == model.ReplyPlain {
		return "Reply to " + whose
	}
	return strings.ToUpper(kind[:1]) + kind[1:] + " " + whose
}
