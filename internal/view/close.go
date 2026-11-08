package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var closeLabels = map[model.Field]string{
	model.CloseReason:      "Reason (done / wontfix / duplicate / superseded / audit-no-change)",
	model.CloseMessage:     "Message",
	model.CloseCommit:      "Commit SHA (optional)",
	model.ClosePR:          "PR URL (optional)",
	model.CloseTest:        "Test command (optional)",
	model.CloseReviewed:    "Reviewed paths, comma-separated (optional)",
	model.CloseExternal:    "External account (optional)",
	model.CloseAttachments: "Attachments to cite (optional, done only)",
	model.CloseReference:   "Issue reference (duplicate / superseded)",
	model.CloseRationale:   "Rationale (audit-no-change)",
}

// Like the Weekdays of the Recurrence card it takes several values at once, so it stays a row.
func attachmentPicker(f model.CloseFormState, tone ui.Tone, title string) tern.Element {
	cursor := -1
	if f.Field == model.CloseAttachments {
		cursor = f.AttachmentAt
	}
	prompt := title + ": "
	if cursor >= 0 {
		prompt = "> " + prompt
	}
	return choiceField(model.CloseAttachments, tone, title, prompt, model.AttachmentLabels(f.Attachments), func(i int) bool { return f.Cites(f.Attachments[i].ID) }, cursor)
}

func closeLayer(m model.Model) tern.Element {
	if !m.CloseForm.Open {
		return nil
	}
	children := make([]tern.Element, 0, 12)
	for _, field := range model.CloseFields {
		label := closeLabels[field.Name]
		tone := ui.ToneNeutral
		if m.CloseForm.Field == field.Name {
			tone = ui.ToneAccent
		}
		if field.Name == model.CloseAttachments {
			if m.CloseForm.ShowsAttachments() {
				children = append(children, attachmentPicker(m.CloseForm, tone, label))
			}
			continue
		}
		children = append(children,
			toneText(string(field.Name)+"-label", tone, label),
			ui.Input{Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: label}, Field: ui.Field{Text: m.CloseForm.Values.Value(field.Name), Cursor: caretAt(m.CloseForm.Caret, field.Name, m.CloseForm.Values.Value(field.Name)), Mode: "insert", Readonly: m.CloseForm.Saving}})
	}
	if m.CloseForm.Err != nil {
		children = append(children, toneText("error", ui.ToneError, m.CloseForm.Err.Error()))
	}
	if m.CloseForm.Saving {
		children = append(children, toneText("pending", ui.TonePending, "Closing Issue…"))
	} else {
		children = append(children,
			toneText("exemption", ui.ToneMuted, "Quick done skips message/evidence only on the owner-local daemon. Parent and throttle guards still apply."),
			buttonRow(
				cardButton("save", model.ModeCloseForm, model.CloseSave, "Close Issue", true),
				cardButton("quick", model.ModeCloseForm, model.CloseQuick, "Quick done", false),
				cardButton("cancel", model.ModeCloseForm, model.CloseCancel, "Cancel", false)))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardClose)}, Modal: true, Size: ui.SizeLG, Head: ui.T(closeTitle(m)), Icon: ui.IconCheck}, children))
}

func closeTitle(m model.Model) string {
	if m.CloseForm.UndoEntry != 0 {
		return "Undo reopen — complete as done"
	}
	return "Close Issue"
}
