package view

import (
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// keyButton is a button whose click is the key press chord, so a click and the key never disagree.
func keyButton(key, text, chord string, primary bool) el.Button {
	button := el.Button{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: "key=" + chord}}, Text: text}
	if primary {
		button.Tone = ui.ToneAccent
	}
	return button
}

// buttonRow is the row of buttons that ends a card.
func buttonRow(buttons ...tern.Element) tern.Element {
	return ui.Row{Common: ui.Common{Key: "actions"}, Gap: ui.GapMD, Wrap: true, Children: buttons}
}

// discardButtons answer the discard prompt of a Draft.
func discardButtons() tern.Element {
	return buttonRow(keyButton("discard", "Discard (y)", "y", true), keyButton("keep", "Keep editing (n)", "n", false))
}
