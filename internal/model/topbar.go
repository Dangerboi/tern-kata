package model

import (
	"fmt"
)

// Daemon is the kata daemon this Pane talks to, read at discovery. Name is the daemon's name in kata's catalog, empty when the
// connection is not a catalog entry; Account is the account the daemon says the Pane acts as, empty when it authenticates none.
type Daemon struct {
	Name    string
	Kind    DaemonKind
	Version string
	Account string
}

// DaemonKind is whether the daemon runs on this machine; the zero value means the Daemon has not been read yet.
type DaemonKind string

const (
	DaemonLocal  DaemonKind = "local"
	DaemonRemote DaemonKind = "remote"
)

// Fact is one cell of the top bar's muted line. Version, when set, follows Text and is drawn in the mono face.
type Fact struct{ Text, Version string }

// DaemonFacts are the cells that describe the Daemon, in bar order: its catalog name when it has one, local or remote, the kata
// version, and the account the Pane acts as. The view appends the Pane's own `tern-kata` cell. Cells the Daemon has not
// reported yet are left out.
func (m Model) DaemonFacts() []Fact {
	var facts []Fact
	if m.Daemon.Name != "" {
		facts = append(facts, Fact{Text: oneLine(m.Daemon.Name)})
	}
	if m.Daemon.Kind != "" {
		facts = append(facts, Fact{Text: string(m.Daemon.Kind)})
	}
	if m.Daemon.Version != "" {
		facts = append(facts, Fact{Text: "kata", Version: m.Daemon.Version})
	}
	if m.Daemon.Account != "" {
		facts = append(facts, Fact{Text: "as " + oneLine(m.Daemon.Account)})
	}
	return facts
}

// issueCounts are the open and closed Issues the top bar counts, and whether they are known. A Project Collection (bound or
// picked) uses the Project's catalog stats, which no filter narrows. Any other Collection counts the Issues it lists, which the
// status, search, owner and label filters have already narrowed: the snapshot carries stats for Projects only, so there is no
// cheap unfiltered count for a named view. Those are known only once the snapshot has loaded.
func (m Model) issueCounts() (open, closed int, known bool) {
	if m.Collection.IsProject() {
		if entry, ok := CatalogEntry(m.Snapshot, m.Collection.ProjectUID); ok && entry.Stats != nil {
			return entry.Stats.Open, entry.Stats.Closed, true
		}
	}
	for _, issue := range m.Issues {
		if IsClosed(issue) {
			closed++
		} else {
			open++
		}
	}
	return open, closed, true
}

// ShowCounts is whether the top bar carries the Issue counts: on the Issues Page only, and not while there is nothing to count.
func (m Model) ShowCounts() bool {
	open, closed, known := m.issueCounts()
	return m.Page == PageIssues && known && open+closed > 0
}

// CountsLine is the top bar's counts line, "5 open · 7 closed".
func (m Model) CountsLine() string {
	open, closed, _ := m.issueCounts()
	return fmt.Sprintf("%d open · %d closed", open, closed)
}
