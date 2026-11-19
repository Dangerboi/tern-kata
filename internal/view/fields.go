package view

import (
	"fmt"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func fieldLayer(m model.Model) tern.Nodes {
	if !m.FieldForm.Open {
		return nil
	}
	title := m.FieldFormTitle()
	input := ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: m.FieldFormLabel()}, Field: ui.Field{Text: m.FieldForm.Value, Cursor: caretAt(m.FieldForm.Caret, "", m.FieldForm.Value), Mode: "insert", Readonly: m.FieldForm.Saving}}
	if placeholder := m.FieldFormPlaceholder(); placeholder != "" {
		input.Placeholder = ui.T(placeholder)
	}
	children := []tern.Element{input}
	if label := m.FieldFormLabel(); label != "" {
		children = labelledInput(inputLeaf, label, "", true, input)
	}
	if m.FieldForm.Action == model.PriorityAction {
		children = []tern.Element{choiceControl(m, model.CardField, model.FieldPriority, m.FieldFormLabel(), true, "", "")}
	}
	suggestions := m.FieldSuggestions()
	for i, label := range suggestions {
		tone, style := ui.ToneMuted, ""
		if i == m.FieldForm.Suggestion%max(len(suggestions), 1) {
			tone, style = ui.ToneNeutral, "accent strong"
		}
		children = append(children, ui.Text{Common: ui.Common{Key: fmt.Sprint("label", i), Tone: tone}, Spans: []ui.Span{{T: label, S: style}}})
	}
	if found := m.FieldForm.Found; found != nil {
		status, tone := "This Issue is deleted.", ui.ToneMuted
		if !found.Deleted {
			status, tone = "This Issue is not deleted.", ui.ToneError
		}
		children = append(children,
			ui.Text{Common: ui.Common{Key: "found"}, Spans: []ui.Span{{T: m.DeletedRef(*found) + " " + found.Title, S: "strong"}}},
			toneText("found-state", tone, status))
	}
	if m.FieldForm.Err != nil {
		children = append(children, toneText("error", ui.ToneError, m.FieldForm.Err.Error()))
	}
	if m.FieldForm.Saving {
		children = append(children, toneText("saving", ui.TonePending, "Saving…"))
	} else {
		children = append(children, formFoot(cardButton("save", model.ModeFieldForm, model.FieldSubmit, saveLabel(m), true), exitButton("cancel", model.ModeFieldForm, model.FieldCancel, "Cancel")))
	}
	overlay := ui.Overlay{Common: ui.Common{Key: layerKey(model.CardField)}, Modal: true, Head: ui.T(title), Icon: fieldIcon(m.FieldForm.Action)}
	if meta := m.FieldFormMeta(); meta != "" {
		overlay.Meta = refMeta(meta)
	}
	return ui.Nodes(popup(overlay, children))
}

// fieldIcon is the head icon of a field card: what the card changes.
func fieldIcon(action model.Action) ui.IconName {
	switch action {
	case model.AddLabelAction, model.RemoveLabelAction:
		return ui.IconTag
	case model.AssignOwnerAction, model.ClearOwnerAction:
		return ui.IconUser
	case model.PriorityAction:
		return ui.IconGauge
	case model.SetParentAction, model.AddBlockerAction, model.AddRelatedAction:
		return ui.IconLink
	case model.TimedAssignAction:
		return ui.IconTimer
	case model.AttachFileAction:
		return ui.IconFilePlus
	case model.DetachFileAction:
		return ui.IconFile
	case model.RestoreDeletedAction:
		return ui.IconUndo
	}
	return ui.IconPen
}

// menuRoomRow is the height of one option of a select's menu, in pixels, with the menu's padding left over.
const menuRoomRow = 32

// menuRoom is blank space under a select that makes the card body tall enough to hold the select's open menu. The menu is drawn
// inside the card body, which scrolls and clips what overflows it, so a card whose only content is the select would cut the
// list off after its first options.
func menuRoom(options int) tern.Element {
	return ui.Text{Common: ui.Common{Key: "menu-room", Shrink: new(float64), Min: &ui.Bounds{H: ui.Px(float64(options*menuRoomRow + 8))}}, Text: "\u00a0"}
}

// selectRoom is the blank space under a card's fields that lets the focused field's open menu show, nothing when that field is
// not a select.
func selectRoom(m model.Model, card model.Card, focused model.Field) []tern.Element {
	if set, ok := m.CardChoices(card, focused); ok && set.Control == model.ControlSelect {
		return []tern.Element{menuRoom(len(set.Labels))}
	}
	return nil
}

// saveLabel is the word on the form's save button: a restore finds the Issue first, then restores it.
func saveLabel(m model.Model) string {
	switch {
	case m.FieldForm.Action != model.RestoreDeletedAction:
		return "Save"
	case m.FieldForm.Found == nil:
		return "Find"
	}
	return "Restore"
}

// labelledInput is a form input's three nodes, for the caller to spread into its own column: the label above (default text, like a choice field's label,
// accent while the input has focus), the input itself, and its format hint as one muted line, present only while the input has
// focus and hint is not empty. The nodes keep flat ids: key-label, the input's own key and key-hint.
func labelledInput(key, label, hint string, focused bool, input tern.Element) []tern.Element {
	nodes := []tern.Element{fieldLabel(key, label, focused), input}
	if focused && hint != "" {
		nodes = append(nodes, toneText(key+"-hint", ui.ToneMuted, hint))
	}
	return nodes
}

// fieldLabel is the label row above a field: default text, accent while the field has focus.
func fieldLabel(key, label string, focused bool) tern.Element {
	tone := ui.ToneNeutral
	if focused {
		tone = ui.ToneAccent
	}
	return fieldLabelTone(key, label, tone)
}

// fieldLabelTone is a label row in tone. A text straight before an input is taken by Tern for the input's accessible name and
// is not drawn, so the label sits in a row of its own.
func fieldLabelTone(key, label string, tone ui.Tone) tern.Element {
	name := toneText("name", tone, label)
	name.Wrap = ui.WrapNone
	return ui.Row{Common: ui.Common{Key: key + "-label", Role: FieldLabelRole}, Children: ui.Nodes(name)}
}
