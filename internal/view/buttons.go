package view

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// choiceControlKey is the key of the control inside a choice field's FormField, so the control's node id is the field's id and
// this key.
const choiceControlKey = "control"

// choiceControl is the choice field called name of card, drawn as the control the model's choice set decides: a select, segmented
// tabs or a switch, each in a FormField that holds the label, the hint and the warning. The field's node is the FormField, so
// its id is FieldID; the control is its child. The control never takes focus: the model owns the keys, and the select draws
// the menu, highlight and search text the model holds.
func choiceControl(m model.Model, card model.Card, name model.Field, title string, focused bool, hint, warning string) tern.Element {
	set, ok := m.CardChoices(card, name)
	if !ok {
		return nil
	}
	label := title
	tone := ui.ToneNeutral
	if focused {
		label, tone = "> "+title, ui.ToneAccent
	}
	field := ui.FormField{Common: ui.Common{Key: string(name), Tone: tone, Aria: title}, Label: ui.T(label)}
	if hint != "" {
		field.Hint = ui.T(hint)
	}
	if warning != "" {
		field.Warning = ui.Says(ui.T(warning))
	}
	common := ui.Common{Key: choiceControlKey, Aria: title}
	switch set.Control {
	case model.ControlTabs:
		items := make([]ui.Tab, len(set.Labels))
		for i, text := range set.Labels {
			items[i] = ui.Tab{ID: strconv.Itoa(i), Label: ui.T(text)}
		}
		field.Children = ui.Nodes(ui.Tabs{Common: common, Items: items, Active: choiceValue(set.Chosen), Variant: ui.TabsSegmented})
	case model.ControlSwitch:
		field.Children = ui.Nodes(ui.Switch{Common: common, On: set.Chosen == set.OnIndex, Variant: ui.SwitchSegment, Labels: [2]string{"No", "Yes"}})
	default:
		menu := m.OpenMenu(card, name)
		sel := ui.Select{Common: common, Value: choiceValue(set.Chosen), Open: menu.Open, Search: set.Search()}
		shown := make([]int, len(set.Labels))
		for i := range shown {
			shown[i] = i
		}
		if sel.Search {
			sel.Query = menu.Query
			if menu.Query != "" {
				shown = set.Matches(menu.Query)
				total := len(set.Labels)
				sel.Total = &total
			}
		}
		if menu.Open {
			sel.Active = choiceValue(menu.Active)
		}
		sel.Options = make([]ui.SelectOption, len(shown))
		for i, at := range shown {
			sel.Options[i] = ui.SelectOption{Value: strconv.Itoa(at), Label: set.Labels[at]}
		}
		field.Children = ui.Nodes(sel)
	}
	return field
}

// choiceValue is the value or tab id of the choice at index, none for -1.
func choiceValue(index int) string {
	if index < 0 {
		return ""
	}
	return strconv.Itoa(index)
}

// choiceField is a popup card's choice field: its prompt, then every choice as a label whose click picks it, the chosen ones
// bracketed in the accent as the active Activity tab is. The choices wrap in their own column, so a wrapped line never reads
// as another field. The choice at cursor gets the keys' cursor mark; -1 marks none.
func choiceField(name model.Field, tone ui.Tone, title, prompt string, labels []string, chosen func(int) bool, cursor int) tern.Element {
	choices := make([]tern.Element, 0, len(labels))
	for i, label := range labels {
		style := ""
		if chosen(i) {
			label, style = "[ "+label+" ]", "accent strong"
		}
		if i == cursor {
			label = "▸" + label
		}
		choices = append(choices, ui.Text{Common: ui.Common{Key: strconv.Itoa(i), Actions: &ui.Actions{Click: ChooseClick(name, i)}}, Spans: []ui.Span{{T: label, S: style}}, Wrap: ui.WrapNone})
	}
	return ui.Row{Common: ui.Common{Key: string(name), Tone: tone, Aria: title}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "prompt", Shrink: new(float64)}, Text: prompt, Wrap: ui.WrapNone},
		ui.Row{Common: ui.Common{Key: "choices", Grow: 1}, Gap: ui.GapMD, Wrap: true, Children: choices})}
}

// cardButton is a card's button for action in mode. Its click is the key press of the action's first non-headless chord and
// its label is words followed by every non-headless key label of the action, so a click and the key never disagree and a
// rebind moves both. An action with no binding in its mode gets no chord, which the driver's card test fails.
func cardButton(key string, mode model.Mode, action model.Action, words string, primary bool) el.Button {
	bindings := model.CardKeys(mode, action)
	chord, text := "", words
	if len(bindings) > 0 {
		labels := make([]string, len(bindings))
		for i, b := range bindings {
			labels[i] = b.Key
		}
		chord, text = bindings[0].Chord, words+" ("+strings.Join(labels, " / ")+")"
	}
	button := el.Button{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: KeyClick(chord)}}, Class: "tk-button", Text: text}
	if primary {
		button.Tone = ui.ToneAccent
		button.Class = "tk-button tk-button-primary"
	}
	return button
}

// menuButton is the top bar's Menu button, its label in the accent colour of the Project heading.
func menuButton() el.Button {
	button := el.Button{Common: ui.Common{Key: "menu", Tone: ui.ToneAccent, Actions: &ui.Actions{Click: MenuClick}}}
	button.Children = []tern.Element{ui.Text{Common: ui.Common{Key: "label"}, Spans: []ui.Span{{T: "Menu", S: "accent strong"}}, Wrap: ui.WrapNone}}
	return button
}

// buttonRow is the row of buttons that ends a card.
func buttonRow(buttons ...tern.Element) tern.Element {
	return ui.Row{Common: ui.Common{Key: "actions"}, Gap: ui.GapMD, Wrap: true, Children: buttons}
}

// discardButtons answer the discard prompt of a Draft.
func discardButtons() tern.Element {
	return buttonRow(cardButton("discard", model.ModeDiscard, model.DiscardDraft, "Discard", false), cardButton("keep", model.ModeDiscard, model.KeepDraft, "Keep editing", true))
}
