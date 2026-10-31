package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// emptyState is Tern's empty state: what is missing in a few words, one muted line on why or what to do, and the buttons that
// fill it. It grows to the room its list would have had. listAndEmpty keys it after its list.
func emptyState(title, text string, actions ...tern.Element) ui.Empty {
	return ui.Empty{
		CommonWithoutTitle: ui.CommonWithoutTitle{Grow: 1},
		Title:              ui.T(title),
		Text:               ui.T(text),
		Children:           actions,
	}
}

// emptyButton is the empty state's button for action, drawn only while the Pane's current mode binds action: the click presses
// that mode's chord, so with Search, the Menu or another column in front a button would do nothing.
func emptyButton(m model.Model, key string, action model.Action, words string, primary bool) []tern.Element {
	if len(model.CardKeys(m.CurrentMode(), action)) == 0 {
		return nil
	}
	return ui.Nodes(cardButton(key, m.CurrentMode(), action, words, primary))
}

// emptyLane is the narrow variant of emptyState, one row of just the title, for a column too narrow to hold a line of
// text. It sits at the top where the rows would start.
func emptyLane(title string) ui.Empty {
	e := emptyState(title, "")
	e.Variant = ui.EmptyInline
	e.Grow = 0
	return e
}

// listAndEmpty is list, followed by empty when list has no children and nothing is loading. The list stays mounted, hidden,
// so a Collection switch only adds or removes the empty node and never remounts the list. A list that is loading keeps its
// own Empty text, so the empty state never shows while rows are on their way.
func listAndEmpty(list ui.List, loading bool, empty ui.Empty) []tern.Element {
	if len(list.Children) > 0 || loading {
		return []tern.Element{list}
	}
	list.Hidden = true
	empty.Key = list.Key + "-empty"
	return []tern.Element{list, empty}
}
