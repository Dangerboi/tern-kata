package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// draftMaxLines is how many lines the Draft editor shows before it scrolls: the card is the one place a body is written at length.
const draftMaxLines = 24

// TextEditor presents the reusable buffer; the model owns text, selection and undo.
func caretAt(c model.Caret, field model.Field, text string) *int {
	offset := c.Offset(field, text)
	return &offset
}

func TextEditor(key string, editor model.TextEditor, readonly bool) tern.Element {
	return TextEditorLines(key, editor, readonly, draftMaxLines)
}

// TextEditorLines is TextEditor for a card that shows fewer lines than the Draft editor, so the card's foot stays on screen.
func TextEditorLines(key string, editor model.TextEditor, readonly bool, maxLines int) tern.Element {
	return ui.Editor{Common: ui.Common{Key: key, Tone: ui.ToneAccent}, Field: ui.Field{Text: editor.Text, Cursor: &editor.Cursor, Anchor: &editor.Anchor, Readonly: readonly}, MaxLines: maxLines}
}

func editorLayer(m model.Model) tern.Element {
	draft, ok := m.ActiveDraft()
	if !ok {
		return nil
	}
	if draft.Conflict != nil {
		return conflictLayer(m, draft)
	}
	title := "Edit Issue body"
	if draft.Comment {
		title = "Add comment"
	}
	if draft.ReplyTo != "" {
		title = replyTitle(draft)
	}
	children := ui.Nodes(TextEditor(inputLeaf, draft.Editor, draft.Saving || draft.DiscardConfirm))
	if draft.ReplyTo != "" {
		if kinds := choiceControl(m, model.CardDraft, model.DraftReplyKind, "Kind", false, "", ""); kinds != nil {
			children = append(ui.Nodes(kinds), children...)
		}
	}
	busy := ""
	if draft.Saving {
		busy = model.Progress("Saving", "")
		children = append(children, toneText("saving", ui.TonePending, busy))
	}
	if draft.Err != nil {
		children = append(children, toneText("error", ui.ToneError, draft.Failure()))
	}
	if draft.DiscardConfirm {
		children = append(children, toneText("question", ui.ToneWarning, "Discard what you wrote?"), discardButtons())
	} else {
		buttons := []ui.Button{}
		if draft.Duplicate() {
			buttons = append(buttons, unavailable(cardButton("force", model.ModeEditor, model.SendReplyAnyway, "Send anyway", false), busy))
		}
		buttons = append(buttons, unavailable(cardButton("editor", model.ModeEditor, model.ExternalEditorAction, "Open in editor", false), busy))
		save := unavailable(cardButton("save", model.ModeEditor, model.SaveDraftAction, "Save", true), busy)
		cancel := unavailable(exitButton("cancel", model.ModeEditor, model.CancelDraft, "Cancel"), busy)
		children = append(children, formFoot(save, cancel, buttons...))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDraft)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Meta: draftMeta(m), Icon: ui.IconPen}, children))
}

// draftMeta is the ref of the Issue a Draft is for, the card's meta; none when the Issue is not in the snapshot.
func draftMeta(m model.Model) ui.Rich {
	if issue, ok := m.LoadedIssue(m.EditorUID); ok {
		return refMeta(m.RefName(issue.ProjectName, issue.ShortID))
	}
	return ui.Rich{}
}

// conflictLayer is the card for a Draft whose Issue changed while it was written: both versions side by side, and the two ways
// to settle it.
func conflictLayer(m model.Model, draft model.Draft) tern.Element {
	current := []tern.Element{ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Current", S: "muted strong"}}}}
	if !draft.Conflict.UpdatedAt.IsZero() {
		current = append(current, ui.Text{Common: ui.Common{Key: "edited"}, Spans: []ui.Span{{T: "edited "}, timeSpan(m.Calendar.Read(draft.Conflict.UpdatedAt, m.Now))}})
	}
	current = append(current, markdown("", draft.Conflict.Body))
	children := ui.Nodes(ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
		ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Yours", S: "muted strong"}}}, TextEditorLines("body", draft.Editor, true, 12))},
		ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: current},
	)}, buttonRow(
		destructive(cardButton("overwrite", model.ModeConflict, model.OverwriteDraft, "Overwrite", false)),
		exitButton("discard", model.ModeConflict, model.DiscardDraft, "Discard mine")))
	ref := m.RefName(draft.Conflict.ProjectName, draft.Conflict.ShortID)
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDraft)}, Modal: true, Size: ui.SizeLG, Head: ui.T("Issue changed while you edited"), Meta: refMeta(ref), Icon: ui.IconPen}, children))
}

// replyTitle is the head of a Reply card for every kind: the kind control shows which one it is.
func replyTitle(draft model.Draft) string {
	whose := "a comment"
	if draft.ReplyAuthor != "" {
		whose = draft.ReplyAuthor + "'s comment"
	}
	return "Reply to " + whose
}
