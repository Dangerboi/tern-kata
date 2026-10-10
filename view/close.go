package view

import (
	"fmt"
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func closeLayer(m model.Model) tern.Element {
	if !m.CloseForm.Open {
		return nil
	}
	labels := [...]string{"Reason (done / wontfix / duplicate / superseded / audit-no-change)", "Message", "Commit SHA (optional)", "PR URL (optional)", "Test command (optional)", "Issue reference (duplicate / superseded)", "Rationale (audit-no-change)"}
	children := make([]tern.Element, 0, 12)
	for i, label := range labels {
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
			el.Button{Common: ui.Common{Key: "save", Actions: &ui.Actions{Click: "close-save"}}, Text: "Close Issue (ctrl+o)"},
			el.Button{Common: ui.Common{Key: "quick", Actions: &ui.Actions{Click: "close-quick"}}, Text: "Quick done (ctrl+d)"},
			el.Button{Common: ui.Common{Key: "cancel", Actions: &ui.Actions{Click: "close-cancel"}}, Text: "Cancel (esc)"},
			ui.Text{Common: ui.Common{Key: "exemption", Tone: ui.ToneMuted}, Text: "Quick done skips message/evidence only on the owner-local daemon. Parent and throttle guards still apply."})
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "close"}, Modal: true, Size: ui.SizeLG, Head: ui.T("Close Issue"), Children: children})
}

func CloseInputFocusID(m model.Model) string {
	if m.CloseForm.Open && !m.CloseForm.Pending {
		return fmt.Sprintf("layer.close.field%d", m.CloseForm.Field)
	}
	return ""
}
