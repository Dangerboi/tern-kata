package model

import "slices"

// movedIndex is where a list's selection lands after a move key: one row, a page of page rows, or either end. at is the row now
// selected, -1 when none is, and the result stays inside a list of n rows, n above zero.
func movedIndex(at, n, page int, action Action) int {
	at = max(at, 0)
	switch action {
	case MoveDown:
		at++
	case MoveUp:
		at--
	case PageDown:
		at += page
	case PageUp:
		at -= page
	case MoveFirst:
		at = 0
	case MoveLast:
		at = n - 1
	}
	return max(0, min(at, n-1))
}

// moveSelected moves the selected uid within items by a move key; with no items it leaves the selection alone.
func moveSelected[T any](items []T, uid func(T) string, sel *string, page int, action Action) {
	if len(items) == 0 {
		return
	}
	at := slices.IndexFunc(items, func(t T) bool { return uid(t) == *sel })
	*sel = uid(items[movedIndex(at, len(items), page, action)])
}

// keepSelected keeps the selected uid while items still hold it, else selects the first item, or none.
func keepSelected[T any](items []T, uid func(T) string, sel *string) {
	if slices.ContainsFunc(items, func(t T) bool { return uid(t) == *sel }) {
		return
	}
	*sel = ""
	if len(items) > 0 {
		*sel = uid(items[0])
	}
}

// listPage is how far Page Up/Page Down move a list's selection; the 'nearest' reveal then keeps it in view. Tern's scroll op does
// not drive a list's own scroller and the Pane's rows are unknown here.
// ponytail: a fixed step, not a measured page; if ResizeMsg ever carries rows, use rows minus the chrome.
const listPage = 10

// issuePage is how far Page Up/Page Down move the Issue list: a row takes two lines under TwoLineRowCols, so a page holds half
// the rows.
func (m Model) issuePage() int {
	if m.Cols < TwoLineRowCols {
		return listPage / 2
	}
	return listPage
}
