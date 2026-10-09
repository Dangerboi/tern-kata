package model

// RevealsDetail reports whether the detail column's selection must be revealed
// after msg took the model from previous to m. Tern keeps the node of its last
// reveal in view and ignores the wheel past it until that node is removed, so
// only a move of the cursor made by a key reveals: a click has already put its
// target under the pointer, and entering the detail or a new Issue leaves the
// cursor where the reader sees it.
func RevealsDetail(previous, m Model, msg Msg) bool {
	if m.Loading || m.DetailHidden || m.Focus != ColumnDetail || !m.Shows(ColumnDetail) || m.Detail == nil || m.Detail.Issue == nil || m.DetailLoading {
		return false
	}
	switch msg := msg.(type) {
	case KeyMsg:
	case ActionMsg:
		if msg.Action != "key" {
			return false
		}
	default:
		return false
	}
	return previous.DetailSection != m.DetailSection || previous.DetailIndex != m.DetailIndex
}
