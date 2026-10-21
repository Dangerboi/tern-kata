package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var closeLabels = map[model.Field]string{
	model.CloseReason:    "Reason (done / wontfix / duplicate / superseded / audit-no-change)",
	model.CloseMessage:   "Message",
	model.CloseCommit:    "Commit SHA (optional)",
	model.ClosePR:        "PR URL (optional)",
	model.CloseTest:      "Test command (optional)",
	model.CloseReference: "Issue reference (duplicate / superseded)",
	model.CloseRationale: "Rationale (audit-no-change)",
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
		children = append(children,
			ui.Text{Common: ui.Common{Key: string(field.Name) + "-label", Tone: tone}, Text: label},
			ui.Input{Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: label}, Field: ui.Field{Text: m.CloseForm.Values.Value(field.Name), Cursor: caretAt(m.CloseForm.Caret, field.Name, m.CloseForm.Values.Value(field.Name)), Mode: "insert", Readonly: m.CloseForm.Saving}})
	}
	if m.CloseForm.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.CloseForm.Err.Error()})
	}
	if m.CloseForm.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Closing Issue…"})
	} else {
		children = append(children,
			ui.Text{Common: ui.Common{Key: "exemption", Tone: ui.ToneMuted}, Text: "Quick done skips message/evidence only on the owner-local daemon. Parent and throttle guards still apply."},
			buttonRow(
				cardButton("save", model.ModeCloseForm, model.CloseSave, "Close Issue", true),
				cardButton("quick", model.ModeCloseForm, model.CloseQuick, "Quick done", false),
				cardButton("cancel", model.ModeCloseForm, model.CloseCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "close"}, Modal: true, Size: ui.SizeLG, Head: ui.T("Close Issue"), Children: children})
}

// CloseFieldID is the id of the Close card's field called name.
func CloseFieldID(name model.Field) string { return "layer.close." + string(name) }
