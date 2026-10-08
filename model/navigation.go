package model

func updateNavigation(m Model, action Action) (Model, []Cmd) {
	if action == QuitAction {
		m.Quitting = true
		return m, []Cmd{Quit{}}
	}
	switch action {
	case ToggleExpansion, ExpandIssue, CollapseIssue, ToggleAllExpansion, ToggleIssueView:
		return updateHierarchy(m, action), nil
	}
	rows := m.VisibleRows()
	if len(rows) == 0 {
		return m, nil
	}
	index := 0
	for i, issue := range rows {
		if issue.UID == m.SelectedUID {
			index = i
			break
		}
	}
	switch action {
	case MoveDown:
		index = min(index+1, len(rows)-1)
	case MoveUp:
		index = max(index-1, 0)
	case MoveFirst:
		index = 0
	case MoveLast:
		index = len(rows) - 1
	}
	m.SelectedUID = rows[index].UID
	return m, nil
}
