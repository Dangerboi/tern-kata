package driver

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// textFocusID is the node that should hold Tern's caret, "" for none.
func textFocusID(m model.Model) string {
	if card, field, ok := m.TextCaret(); ok {
		return view.FieldID(card, field)
	}
	if m.TopCard() == model.NoCard && m.SearchInputFocused() {
		return view.SearchID
	}
	return ""
}

// transientTarget reports whether id is a node that can disappear between a
// queued focus, reveal or scroll op and the next render, so that Tern's
// "unknown id" answer for it is expected.
func transientTarget(id string) bool {
	if _, _, ok := view.FieldFromID(id); ok || id == view.SearchID {
		return true
	}
	if view.IsIssueID(id) || view.IsBoardCardID(id) {
		return true
	}
	// The active tab's rows and its empty line go when the tab switches, and the tab names with them when the detail turns
	// to loading another Issue before Tern runs the reveal.
	row, ok := strings.CutPrefix(id, view.DetailActivityID+".")
	return ok && !strings.Contains(row, ".")
}
