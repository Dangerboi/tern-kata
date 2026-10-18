package model

import "fmt"

// Daemon is the kata daemon this Pane talks to, read at discovery.
type Daemon struct {
	Remote  bool
	Version string
}

// DaemonSeparator divides the top bar's daemon line and its Menu button.
const DaemonSeparator = " · "

// DaemonLine is the top bar's daemon line.
func (m Model) DaemonLine() string {
	kind := "local"
	if m.Daemon.Remote {
		kind = "remote"
	}
	line := "Daemon: " + kind
	if m.Daemon.Version != "" {
		line += DaemonSeparator + "kata " + m.Daemon.Version
	}
	return line
}

// CountsLine is the top bar's counts line. A Project Collection (bound or picked)
// shows the Project's catalog stats, which no filter narrows. Any other
// Collection counts the Issues it lists, which the status, search, owner and
// label filters have already narrowed: the snapshot carries stats for Projects
// only, so there is no cheap unfiltered count for a named view.
func (m Model) CountsLine() string {
	var open, closed int
	counted := false
	if m.Collection.IsProject() {
		if entry, ok := CatalogEntry(m.Snapshot, m.Collection.ProjectUID); ok && entry.Stats != nil {
			open, closed, counted = entry.Stats.Open, entry.Stats.Closed, true
		}
	}
	if !counted {
		for _, issue := range m.Issues {
			if IsClosed(issue) {
				closed++
			} else {
				open++
			}
		}
	}
	return fmt.Sprintf("open: %d · closed: %d · all: %d", open, closed, open+closed)
}
