package model

func updateProject(m Model, msg ProjectLoaded) (Model, []Cmd) {
	m.Project = msg.Project
	m.Snapshot = msg.Snapshot
	m.Issues = msg.Snapshot.Collection
	m.ETag = msg.ETag
	m.Loading = false
	m.Err = nil
	return m.WithVisibleSelection(), nil
}
