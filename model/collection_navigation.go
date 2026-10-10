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
	entries := m.SidebarEntries()
	if len(entries) == 0 {
		if action == DrillIn {
			m.Focus = ColumnList
		}
		return m, nil
	}
	last := len(entries) - 1
	switch action {
	case MoveDown:
		m.CollectionCursor = min(m.CollectionCursor+1, last)
	case MoveUp:
		m.CollectionCursor = max(0, m.CollectionCursor-1)
	case MoveFirst:
		m.CollectionCursor = 0
	case MoveLast:
		m.CollectionCursor = last
	case MenuBack:
		if m.SidebarProjects {
			return setMenuLevel(m, false, false), nil
		}
	case DrillIn:
		return openEntry(m, entries[min(m.CollectionCursor, last)], false)
	}
	return m, nil
}
