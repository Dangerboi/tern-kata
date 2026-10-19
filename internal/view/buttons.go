package view

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

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
	button := el.Button{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: KeyClick(chord)}}, Text: text}
	if primary {
		button.Tone = ui.ToneAccent
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
