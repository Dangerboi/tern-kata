package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"strings"
)

var createKeys = [...]string{"title", "body", "labels", "owner", "priority"}

func CreateFieldIndex(id string) (int, bool) {
	for i, key := range createKeys {
		if id == "layer.create."+key {
			return i, true
		}
	}
	return 0, false
}
func createLayer(m model.Model) tern.Element {
	f := m.Create
	title := "Create Issue"
	if f.ParentUID != "" {
		title = "Create child Issue"
	}
	values := [...]string{f.Title, f.Editor.Text, f.Labels, f.Owner, f.Priority}
	names := [...]string{"Title", "Body (optional)", "Labels (optional, comma-separated)", "Owner (optional)", "Priority (optional, 0-4)"}
	children := []tern.Element{}
	for i, key := range createKeys {
		tone := ui.ToneNeutral
		if i == f.Field {
			tone = ui.ToneAccent
		}
		children = append(children, ui.Text{Common: ui.Common{Key: key + "-label", Tone: tone}, Text: names[i]})
		readonly := f.Saving || f.DiscardConfirm
		if i == 1 {
			children = append(children, TextEditor(key, f.Editor, readonly))
		} else {
			children = append(children, ui.Input{Common: ui.Common{Key: key, Tone: tone, Aria: names[i]}, Field: ui.Field{Text: values[i], Mode: "insert", Readonly: readonly}})
		}
	}
	if f.Field == 2 {
		children = append(children, ui.Text{Common: ui.Common{Key: "completion", Tone: ui.ToneMuted}, Text: strings.Join(m.CreateSuggestions(), ", ")})
	}
	if f.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err.Error()})
	}
	switch {
	case f.Saving:
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Creating Issue…"})
	case f.DiscardConfirm:
		children = append(children, ui.Text{Common: ui.Common{Key: "discard", Tone: ui.ToneWarning}, Text: "Discard this Draft? y discards; n or esc keeps editing."}, discardButtons())
	default:
		children = append(children, buttonRow(keyButton("save", title+" (ctrl+o)", "ctrl+o", true), keyButton("cancel", "Cancel (esc)", "escape", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "create"}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}
