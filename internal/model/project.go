package model

func updateProject(m Model, msg ProjectLoaded) (Model, []Cmd) {
	m.Project = msg.Project
	m.Bound = msg.Project.UID != ""
	m.Collection = Collection{View: "inbox", Title: "Inbox"}
	if msg.Project.UID != "" {
		m.Collection = Collection{View: "project", ProjectUID: msg.Project.UID, Title: msg.Project.Name}
	}
	m.Daemon = msg.Daemon
	m.Loading = true
	m.Err = nil
	return snapshotChanged(m)
}
