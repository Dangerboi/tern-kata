package view

import (
	"fmt"
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var closeLabels = [...]string{"Reason (done / wontfix / duplicate / superseded / audit-no-change)", "Message", "Commit SHA (optional)", "PR URL (optional)", "Test command (optional)", "Issue reference (duplicate / superseded)", "Rationale (audit-no-change)"}

func closeLayer(m model.Model) tern.Element {
	if !m.CloseForm.Open {
		return nil
	}
	children := make([]tern.Element, 0, 12)
	for i, label := range closeLabels {
		tone := ui.ToneNeutral
		if m.CloseForm.Field == i {
			tone = ui.ToneAccent
		}
		children = append(children,
			ui.Text{Common: ui.Common{Key: fmt.Sprintf("%d-label", i), Tone: tone}, Text: label},
			ui.Input{Common: ui.Common{Key: fmt.Sprintf("field%d", i), Tone: tone, Aria: label}, Field: ui.Field{Text: m.CloseForm.Values[i], Mode: "insert", Readonly: m.CloseForm.Pending}})
	}
	if m.CloseForm.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.CloseForm.Err.Error()})
	}
	if m.CloseForm.Pending {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Closing Issue…"})
	} else {
		children = append(children,
			ui.Text{Common: ui.Common{Key: "exemption", Tone: ui.ToneMuted}, Text: "Quick done skips message/evidence only on the owner-local daemon. Parent and throttle guards still apply."},
			buttonRow(
				keyButton("save", "Close Issue (ctrl+o)", "ctrl+o", true),
				keyButton("quick", "Quick done (ctrl+d)", "ctrl+d", false),
				keyButton("cancel", "Cancel (esc)", "escape", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "close"}, Modal: true, Size: ui.SizeLG, Head: ui.T("Close Issue"), Children: children})
}

// CloseFieldID is the id of the Close card's field index.
func CloseFieldID(index int) string { return fmt.Sprintf("layer.close.field%d", index) }

func CloseFieldIndex(id string) (int, bool) {
	for index := range closeLabels {
		if id == CloseFieldID(index) {
			return index, true
		}
	}
	return 0, false
}
