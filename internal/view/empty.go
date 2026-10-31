package view

import (
	"github.com/stencil-hq/tern-sdk/go/tern"
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

// emptyLane is the narrow variant of emptyState, one row of just the title, for a column too narrow to hold a line of
// text. It sits at the top where the rows would start.
func emptyLane(key, title string) ui.Empty {
	e := emptyState(key, title, "")
	e.Variant = ui.EmptyInline
	e.Grow = 0
	return e
}

// listOrEmpty is list, or empty when list has no children and nothing is loading. A list that is loading keeps its own
// Empty text, so the empty state never shows while rows are on their way.
func listOrEmpty(list ui.List, loading bool, empty ui.Empty) tern.Element {
	if len(list.Children) == 0 && !loading {
		return empty
	}
	return list
}
