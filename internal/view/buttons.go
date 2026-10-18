package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// choiceField is a popup card's choice field: its prompt, then every choice as a label whose click picks it, the chosen ones
// bracketed in the accent as the active Activity tab is. The choice at cursor gets the keys' cursor mark; -1 marks none.
func choiceField(name model.Field, tone ui.Tone, title, prompt string, labels []string, chosen func(int) bool, cursor int) tern.Element {
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "prompt"}, Text: prompt})
	for i, label := range labels {
		style := ""
		if chosen(i) {
			label, style = "[ "+label+" ]", "accent strong"
		}
		if i == cursor {
			label = "▸" + label
		}
		children = append(children, ui.Text{Common: ui.Common{Key: strconv.Itoa(i), Actions: &ui.Actions{Click: ChooseClick(name, i)}}, Spans: []ui.Span{{T: label, S: style}}})
	}
	return ui.Row{Common: ui.Common{Key: string(name), Tone: tone, Aria: title}, Gap: ui.GapMD, Wrap: true, Children: children}
}

// keyButton is a button whose click is the key press chord, so a click and the key never disagree.
func keyButton(key, text, chord string, primary bool) el.Button {
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
	return buttonRow(keyButton("discard", "Discard (y)", "y", false), keyButton("keep", "Keep editing (n / esc)", "n", true))
}
