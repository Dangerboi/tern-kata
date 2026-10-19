package model

import (
	"fmt"
	"strings"
)

// Daemon is the kata daemon this Pane talks to, read at discovery.
type Daemon struct {
	Kind    DaemonKind
	Version string
}

// DaemonKind is whether the daemon runs on this machine; the zero value means the kind check did not answer.
type DaemonKind string

const (
	DaemonKindUnknown DaemonKind = ""
	DaemonLocal       DaemonKind = "local"
	DaemonRemote      DaemonKind = "remote"
)

// DaemonSeparator divides the top bar's daemon line and its Menu button.
const DaemonSeparator = " · "

// DaemonLine is the top bar's daemon line.
func (m Model) DaemonLine() string {
	var parts []string
	if m.Daemon.Kind != DaemonKindUnknown {
		parts = append(parts, string(m.Daemon.Kind))
	}
	if m.Daemon.Version != "" {
		parts = append(parts, "kata "+m.Daemon.Version)
	}
	return "Daemon: " + strings.Join(parts, DaemonSeparator)
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
