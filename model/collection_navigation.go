package model

type Column uint8

const (
	ColumnList Column = iota
	ColumnCollections
	ColumnDetail
)

func updateCollectionNavigation(m Model, action Action) (Model, []Cmd) {
	switch action {
	case MoveDown, MoveUp, MoveFirst, MoveLast, DrillIn:
		m.PendingCreatedUID = ""
	}
	if action == PageUp || action == PageDown {
		return updateNavigation(m, action)
	}
	if len(m.Collections) == 0 {
		if action == DrillIn {
			m.Focus = ColumnList
		}
		return m, nil
	}
	// The New Project row follows the Collections, so the cursor may rest one past them.
	newProjectRow := len(m.Collections)
	switch action {
	case MoveDown:
		m.CollectionCursor = min(m.CollectionCursor+1, newProjectRow)
	case MoveUp:
		m.CollectionCursor = max(0, m.CollectionCursor-1)
	case MoveFirst:
		m.CollectionCursor = 0
	case MoveLast:
		m.CollectionCursor = newProjectRow
	case DrillIn:
		if m.CollectionCursor >= newProjectRow {
			return openProjectForm(m), nil
		}
		m.Focus = ColumnList
		return updateCollection(m, CollectionSelected{Collection: m.Collections[m.CollectionCursor]})
	}
	return m, nil
}
