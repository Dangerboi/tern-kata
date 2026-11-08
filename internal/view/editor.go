package view

import (
	"fmt"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// TextEditor presents the reusable buffer; the model owns text, selection and undo.
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
		children = append(children, toneText("saving", ui.TonePending, "Saving Draft…"))
	}
	if draft.Err != nil {
		children = append(children, toneText("error", ui.ToneError, draft.Err.Error()))
	}
	switch {
	case draft.DiscardConfirm:
		children = append(children, toneText("discard", ui.ToneWarning, "Discard this Draft? y discards; n or esc keeps editing."), discardButtons())
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
		)}, toneText("choice", ui.ToneWarning, "o overwrites against the new revision; d discards only your Draft."),
			buttonRow(cardButton("overwrite", model.ModeConflict, model.OverwriteDraft, "Overwrite", true), cardButton("discard", model.ModeConflict, model.DiscardDraft, "Discard Draft", false)))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDraft)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconPen}, children))
}

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
