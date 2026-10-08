package model

func updateNavigation(m Model, action Action) (Model, []Cmd) {
	if action == RetryAction {
		return updateLive(m, Retry{})
	}
	if action == QuitAction {
		m.Quitting = true
		return m, []Cmd{Quit{}}
	}
	if len(m.Issues) == 0 {
		return m, nil
	}
	index := 0
	for i, issue := range m.Issues {
		if issue.UID == m.SelectedUID {
			index = i
			break
		}
	}
	switch action {
	case MoveDown:
		index = min(index+1, len(m.Issues)-1)
	case MoveUp:
		index = max(index-1, 0)
	case MoveFirst:
		index = 0
	case MoveLast:
		index = len(m.Issues) - 1
	}
	m.SelectedUID = m.Issues[index].UID
	return m, nil
}
