package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"strings"
)

var closeLabels = map[model.Field]string{
	model.CloseReason:      "Reason",
	model.CloseMessage:     "Message",
	model.CloseCommit:      "Commit",
	model.ClosePR:          "Pull request",
	model.CloseTest:        "Test command",
	model.CloseReviewed:    "Reviewed paths",
	model.CloseExternal:    "External account",
	model.CloseAttachments: "Attachments",
	model.CloseReference:   "Duplicate of",
	model.CloseRationale:   "Why nothing changed",
}

// closeHints are the one muted line under a field, shown while it has focus.
var closeHints = map[model.Field]string{
	model.CloseCommit:   "A commit SHA.",
	model.ClosePR:       "A URL.",
	model.CloseReviewed: "Separate with commas.",
}

// closeLabel is the word above a field. The reference reads by the reason: a superseding Issue replaces this one.
func closeLabel(f model.CloseFormState, name model.Field) string {
	if name == model.CloseReference && strings.TrimSpace(f.Values.Reason) == string(model.ReasonSuperseded) {
		return "Replaced by"
	}
	return closeLabels[name]
}

// Like the Weekdays of the Recurrence card it takes several values at once, so it stays a row.
func attachmentPicker(f model.CloseFormState, tone ui.Tone, title string) tern.Element {
	cursor := -1
	if f.Field == model.CloseAttachments {
		cursor = f.AttachmentAt
	}
	return choiceField(model.CloseAttachments, tone, title, title, model.AttachmentLabels(f.Attachments), func(i int) bool { return f.Cites(f.Attachments[i].ID) }, cursor)
}

func closeLayer(m model.Model) tern.Element {
	if !m.CloseForm.Open {
		return nil
	}
	children := make([]tern.Element, 0, 12)
	roomFor := 0
	for _, field := range model.CloseFields {
		if m.CloseForm.Hides(field.Name) {
			continue
		}
		label := closeLabel(m.CloseForm, field.Name)
		focused := m.CloseForm.Field == field.Name
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		switch field.Name {
		case model.CloseReason:
			children = append(children, choiceControl(m, model.CardClose, field.Name, label, focused, "", ""))
			if set, ok := m.CardChoices(model.CardClose, field.Name); ok && set.Control == model.ControlSelect && focused {
				roomFor = len(set.Labels)
			}
			continue
		case model.CloseAttachments:
			children = append(children, attachmentPicker(m.CloseForm, tone, label))
			continue
		}
		value := m.CloseForm.Values.Value(field.Name)
		input := ui.Input{Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: label}, Field: ui.Field{Text: value, Cursor: caretAt(m.CloseForm.Caret, field.Name, value), Mode: "insert", Readonly: m.CloseForm.Saving}}
		children = append(children, labelledInput(string(field.Name), label, closeHints[field.Name], focused, input)...)
	}
	if roomFor > 0 {
		// Room under the last field for the open menu, which the scrolling body would otherwise clip.
		children = append(children, menuRoom(roomFor))
	}
	if m.CloseForm.Err != nil {
		children = append(children, toneText("error", ui.ToneError, m.CloseForm.Err.Error()))
	}
	save := cardButton("save", model.ModeCloseForm, model.CloseSave, "Close Issue", true)
	quick := unavailable(cardButton("quick", model.ModeCloseForm, model.CloseQuick, "Close without evidence", false), m.CloseQuickRefusal())
	cancel := exitButton("cancel", model.ModeCloseForm, model.CloseCancel, "Cancel")
	if m.CloseForm.Saving {
		pending := model.Progress("Closing", "Issue")
		for _, button := range []*ui.Button{&save, &quick, &cancel} {
			button.Disabled = ui.Because(pending)
		}
		children = append(children, toneText("pending", ui.TonePending, pending))
	}
	children = append(children, formFoot(save, cancel, quick))
	overlay := ui.Overlay{Common: ui.Common{Key: layerKey(model.CardClose)}, Modal: true, Size: ui.SizeLG, Head: ui.T(closeTitle(m)), Icon: ui.IconCheck}
	if ref := m.CloseIssueRef(); ref != "" {
		overlay.Meta = refMeta(ref)
	}
	return ui.Nodes(popup(overlay, children))
}

func closeTitle(m model.Model) string {
	if m.CloseForm.UndoEntry != 0 {
		return "Undo reopen"
	}
	return "Close Issue"
}
