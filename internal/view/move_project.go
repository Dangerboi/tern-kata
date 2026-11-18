package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var MoveListID = LayerID(model.CardMove) + ".list"

func moveLayer(m model.Model) tern.Element {
	f := m.MoveProject
	if !f.Open {
		return nil
	}
	var children []tern.Element
	if m.MoveCardMode() == model.ModeMoveProject {
		children = moveChoosing(m)
	} else {
		children = moveConfirming(m)
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardMove)}, Head: ui.T("Move to Project"), Meta: refMeta(m.MoveRef()), Icon: ui.IconFolderGo, Modal: true, Size: ui.SizeMD}, children))
}

func moveChoosing(m model.Model) []tern.Element {
	f := m.MoveProject
	targets := m.MoveTargets()
	items := make([]tern.Element, 0, len(targets))
	for _, p := range targets {
		items = append(items, ui.Item{Common: ui.Common{Key: p.UID}, Label: ui.T(p.Name)})
	}
	selected := ""
	if len(targets) > 0 {
		selected = itemID(MoveListID, targets[min(f.Cursor, len(targets)-1)].UID)
	}
	empty := "No other Projects."
	if f.Filter != "" {
		empty = fmt.Sprintf("Nothing matches %q.", f.Filter)
	}
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Filter"}, Field: ui.Field{Text: f.Filter, Cursor: caretAt(f.Caret, "", f.Filter), Mode: "insert", Placeholder: ui.T("Filter")}, Clear: true},
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Empty: ui.T(empty), Max: ui.MaxLines(12), Children: items},
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	return append(children, buttonRow(exitButton("cancel", model.ModeMoveProject, model.MoveProjectClose, "Cancel")))
}

func moveConfirming(m model.Model) []tern.Element {
	f := m.MoveProject
	children := []tern.Element{propGrid(propRow("Project", ui.Text{Common: ui.Common{Key: "target"}, Text: f.Target.Name}))}
	busy := ""
	switch {
	case f.Sending && f.Previewed:
		busy = model.Progress("Moving", "")
		children = append(children, toneText("pending", ui.TonePending, busy))
	case f.Sending:
		busy = model.Progress("Checking the move", "")
		children = append(children, toneText("pending", ui.TonePending, busy))
	case f.Conflict:
		children = append(children, toneText("error", ui.ToneError, m.MovePreview()))
	default:
		children = append(children, ui.Text{Common: ui.Common{Key: "preview"}, Text: m.MovePreview()})
		if f.Err != nil {
			children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
		}
	}
	if !f.Sending {
		children = append(children, choiceControl(m, model.CardMove, model.MoveFieldSingle, "Only this Issue", false, "", ""))
	}
	words := "Move"
	if f.Conflict || !f.Previewed {
		words = "Preview again"
	}
	send := unavailable(cardButton("send", model.ModeMoveConfirm, model.MoveProjectSend, words, true), busy)
	back := unavailable(exitButton("back", model.ModeMoveConfirm, model.MoveProjectBack, "Back"), busy)
	return append(children, buttonRow(send, back))
}
