package model

import "fmt"

func updateProject(m Model, msg ProjectLoaded) (Model, []Cmd) {
	m.Project = msg.Project
	m.Bound = msg.Project.UID != ""
	m.Collection = InboxCollection()
	if msg.Project.UID != "" {
		m.Collection = ProjectCollection(msg.Project.UID, msg.Project.Name)
	}
	m.Daemon = msg.Daemon
	m.DefaultFilters = msg.Default
	if m.Filters == (Filters{}) {
		m.Filters = msg.Default
	}
	if msg.DefaultErr != nil {
		m.Notice = fmt.Sprintf("Default filter not applied: %v", msg.DefaultErr)
	}
	m.Loading = true
	m.Err = nil
	return snapshotChanged(m)
}
