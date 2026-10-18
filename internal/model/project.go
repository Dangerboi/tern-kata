package model

func updateProject(m Model, msg ProjectLoaded) (Model, []Cmd) {
	m.Project = msg.Project
	m.Bound = msg.Project.UID != ""
	m.Collection = InboxCollection()
	if msg.Project.UID != "" {
		m.Collection = ProjectCollection(msg.Project.UID, msg.Project.Name)
	}
	m.Daemon = msg.Daemon
	m.Loading = true
	m.Err = nil
	return snapshotChanged(m)
}
