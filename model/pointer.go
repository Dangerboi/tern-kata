package model

// selectRow is a click on an Issue row: the list gets the keys with that row
// selected, as moving there with j/k leaves them. Rows folded away are ignored.
func selectRow(m Model, uid string) Model {
	for _, row := range m.VisibleRows() {
		if row.UID == uid {
			m.PendingCreatedUID = ""
			m.SelectedUID = uid
			m.Focus = ColumnList
			break
		}
	}
	return m
}

// toggleRow is a click on a parent row from any column or mode: select it like
// any row, then fold its children as space does.
func toggleRow(m Model, uid string) (Model, []Cmd) {
	for _, row := range m.VisibleRows() {
		if row.UID == uid && row.HasChildren {
			return updateHierarchy(selectRow(m, uid), ToggleExpansion), nil
		}
	}
	return m, nil
}

// openRow is a double click on an Issue row from any column or mode: select it
// like a click, then make the state change enter makes in the list. Rows folded
// away are ignored. The click leaves the detail, which ends its jump history.
func openRow(m Model, uid string) (Model, []Cmd) {
	selected := selectRow(m, uid)
	if selected.SelectedUID != uid {
		return m, nil
	}
	selected.DetailBack = nil
	return updateNavigation(selected, DrillIn)
}
