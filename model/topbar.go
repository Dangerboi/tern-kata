package model

import "fmt"

// Daemon is the kata daemon this Pane talks to, read at discovery.
type Daemon struct {
	Remote  bool
	Version string
}

// DaemonLine is the top bar's daemon line.
func (m Model) DaemonLine() string {
	kind := "local"
	if m.Daemon.Remote {
		kind = "remote"
	}
	line := "Daemon: " + kind
	if m.Daemon.Version != "" {
		line += " · kata " + m.Daemon.Version
	}
	return line
}

// CountsLine is the top bar's counts line: the Project's catalog stats when
// the Collection is a bound Project, otherwise counts over the listed Issues.
func (m Model) CountsLine() string {
	var open, closed int
	counted := false
	if m.Collection.View == "project" {
		for _, entry := range m.Snapshot.Catalog {
			if entry.Project.UID == m.Collection.ProjectUID && entry.Stats != nil {
				open, closed, counted = int(entry.Stats.Open), int(entry.Stats.Closed), true
			}
		}
	}
	if !counted {
		for _, issue := range m.Issues {
			if issue.Status == "closed" {
				closed++
			} else {
				open++
			}
		}
	}
	return fmt.Sprintf("open: %d · closed: %d · all: %d", open, closed, open+closed)
}
