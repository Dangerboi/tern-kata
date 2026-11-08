package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// MoveListID is the id of the Move card's list of Projects.
var MoveListID = LayerID(model.CardMove) + ".list"

// MoveSingleID is the id of the Move card's "Only this Issue" switch.
var MoveSingleID = LayerID(model.CardMove) + ".single." + choiceControlKey

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
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardMove)}, Head: ui.T("Move to another Project"), Meta: refMeta("#" + f.ShortID), Icon: ui.IconFolderGo, Modal: true, Size: ui.SizeMD}, children))
}

// moveChoosing is the card while it lists Projects: the filter, the list and kata's refusal of the last preview.
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
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Filter Projects"}, Field: ui.Field{Text: f.Filter, Cursor: caretAt(f.Caret, "", f.Filter), Mode: "insert", Placeholder: ui.T("Filter Projects")}},
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Empty: ui.T("No other Projects match."), Max: ui.MaxLines(12), Children: items},
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	return append(children, buttonRow(cardButton("cancel", model.ModeMoveProject, model.MoveProjectClose, "Cancel", false)))
}

// moveConfirming is the card once a Project is chosen: the preview kata accepted, the conflict, or kata's refusal of the move.
func moveConfirming(m model.Model) []tern.Element {
	f := m.MoveProject
	children := []tern.Element{toneText("target", ui.ToneAccent, "To "+f.Target.Name)}
	switch {
	case f.Sending && f.Previewed:
		children = append(children, toneText("pending", ui.TonePending, "Moving Issue…"))
	case f.Sending:
		children = append(children, toneText("pending", ui.TonePending, "Asking kata what the move would do…"))
	default:
		children = append(children, ui.Text{Common: ui.Common{Key: "preview"}, Text: m.MovePreview()})
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	if f.Sending {
		return children
	}
	children = append(children, ui.FormField{Common: ui.Common{Key: "single", Aria: "Only this Issue"}, Label: ui.T("Only this Issue"),
		Hint:     ui.T("Leave its descendants where they are. Key: o"),
		Children: ui.Nodes(ui.Switch{Common: ui.Common{Key: choiceControlKey, Aria: "Only this Issue"}, On: f.Single, Variant: ui.SwitchSegment, Labels: [2]string{"No", "Yes"}})})
	words := "Move"
	if f.Conflict || !f.Previewed {
		words = "Preview again"
	}
	return append(children, buttonRow(
		cardButton("send", model.ModeMoveConfirm, model.MoveProjectSend, words, true),
		cardButton("back", model.ModeMoveConfirm, model.MoveProjectBack, "Other Project", false)))
}
