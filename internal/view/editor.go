package view

import (
	"fmt"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// TextEditor presents the reusable buffer; the model owns text, selection and undo.
// caretAt is the caret of the single-line field in UTF-16 units for ui.Field.Cursor.
func caretAt(c model.Caret, field model.Field, text string) *int {
	offset := c.Offset(field, text)
	return &offset
}

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
	children := ui.Nodes(TextEditor(inputLeaf, draft.Editor, draft.Saving || draft.DiscardConfirm || draft.Conflict != nil))
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
		buttons := []tern.Element{cardButton("save", model.ModeEditor, model.SaveDraftAction, "Save", true)}
		if draft.Duplicate() {
			buttons = append(buttons, cardButton("force", model.ModeEditor, model.SendReplyAnyway, "Send anyway", false))
		}
		if draft.ReplyTo != "" {
			buttons = append(buttons, cardButton("kind", model.ModeEditor, model.CycleReplyKind, "Kind", false))
		}
		buttons = append(buttons, cardButton("editor", model.ModeEditor, model.ExternalEditorAction, "$EDITOR", false), cardButton("discard", model.ModeEditor, model.CancelDraft, "Discard", false))
		children = append(children, buttonRow(buttons...))
	}
	if draft.Conflict != nil {
		title = "Issue changed: Draft kept"
		children = ui.Nodes(ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
			ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("Your Draft (base revision %d)", draft.BaseRevision)}, TextEditor("body", draft.Editor, true))},
			ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("New version (revision %d)", draft.Conflict.Revision)}, ui.Md{Text: draft.Conflict.Body})},
		)}, ui.Text{Common: ui.Common{Key: "choice", Tone: ui.ToneWarning}, Text: "o overwrites against the new revision; d discards only your Draft."},
			buttonRow(cardButton("overwrite", model.ModeConflict, model.OverwriteDraft, "Overwrite", true), cardButton("discard", model.ModeConflict, model.DiscardDraft, "Discard Draft", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDraft)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
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
