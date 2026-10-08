package model

func updateProject(m Model, msg ProjectLoaded) (Model, []Cmd) {
	m.Project = msg.Project
	m.Snapshot = msg.Snapshot
	m.Issues = msg.Snapshot.Collection
	m.ETag = msg.ETag
	m.Loading = false
	m.Err = nil
	for _, issue := range m.Issues {
		if issue.UID == m.SelectedUID {
			return m, nil
		}
	}
	m.SelectedUID = ""
	if len(m.Issues) > 0 {
		m.SelectedUID = m.Issues[0].UID
	}
	return m, nil
}