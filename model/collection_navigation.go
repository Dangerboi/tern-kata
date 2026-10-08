package model

type Column uint8

const (
	ColumnList Column = iota
	ColumnCollections
	ColumnDetail
)

func updateCollectionNavigation(m Model, action Action) (Model, []Cmd) {
	if len(m.Collections) == 0 {
		return m, nil
	}
	switch action {
	case MoveDown:
		m.CollectionCursor = min(m.CollectionCursor+1, len(m.Collections)-1)
	case MoveUp:
		m.CollectionCursor = max(0, m.CollectionCursor-1)
	case MoveFirst:
		m.CollectionCursor = 0
	case MoveLast:
		m.CollectionCursor = len(m.Collections) - 1
	case DrillIn:
		m.Focus = ColumnList
		return updateCollection(m, CollectionSelected{Collection: m.Collections[m.CollectionCursor]})
	}
	return m, nil
}
