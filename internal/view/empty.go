package view

import (
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// emptyState is Tern's empty state under key: what is missing in a few words, one muted line on why or what to do, and the
// buttons that fill it. It grows to the room its list would have had.
func emptyState(key, title, text string, actions ...tern.Element) ui.Empty {
	return ui.Empty{
		CommonWithoutTitle: ui.CommonWithoutTitle{Key: key, Grow: 1},
		Title:              ui.T(title),
		Text:               ui.T(text),
		Children:           actions,
	}
}

// emptyLane is the narrow variant of emptyState, one row, for a Lane with no cards.
func emptyLane(key, title, text string) ui.Empty {
	e := emptyState(key, title, text)
	e.Variant = ui.EmptyInline
	return e
}

// keyButton is a button that presses chord, the key that fills the empty state it sits in; label is its words, with the key.
func keyButton(key, chord, label string) tern.Element {
	return el.Button{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: KeyClick(chord)}}, Class: "tk-button", Text: label}
}

// listOrEmpty is list, or empty when list has no children and nothing is loading. A list that is loading keeps its own
// Empty text, so the empty state never shows while rows are on their way.
func listOrEmpty(list ui.List, loading bool, empty ui.Empty) tern.Element {
	if len(list.Children) == 0 && !loading {
		return empty
	}
	return list
}
