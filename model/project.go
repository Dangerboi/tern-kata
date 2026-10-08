package model

func updateProject(m Model, msg ProjectLoaded) (Model, []Cmd) {
	m.Project = msg.Project
	m.Bound = msg.Project.UID != ""
	m.Collection = Collection{View: "inbox", Title: "Inbox"}
	if msg.Project.UID != "" {
		m.Collection = Collection{View: "project", ProjectUID: msg.Project.UID, Title: msg.Project.Name}
	}
	m.LocalDate, m.TimeZone = msg.LocalDate, msg.TimeZone
	m.Daemon = msg.Daemon
	return m.ApplySnapshot(msg.Snapshot, msg.ETag), nil
}
