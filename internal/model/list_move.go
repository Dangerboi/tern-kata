package model

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

// listPage is how many rows a page key moves on the Jobs and Runs Pages.
const listPage = 10
