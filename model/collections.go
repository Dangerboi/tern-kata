package model

import (
	g "go.kenn.io/kata/pkg/client/generated"
)

type Collection struct{ View, ProjectUID, Title string }

// IsProject says the Collection is one Project's Issues, not a named view.
func (c Collection) IsProject() bool { return c.View == "project" }

// CatalogEntry is the snapshot's catalog entry for the Project with uid.
func CatalogEntry(snapshot g.UISnapshotResponseBody, uid string) (g.UIProject, bool) {
	for _, entry := range snapshot.Catalog {
		if entry.Project.UID == uid {
			return entry, true
		}
	}
	return g.UIProject{}, false
}

type IssueGroup struct {
	ID, Title string
	Issues    []Issue
	// Heading says the list titles the group; a Project list is already
	// titled by its own heading. Fixed per snapshot, so rows loaded for one
	// Collection keep their shape while another Collection loads.
	Heading bool
}
type CollectionSelected struct{ Collection Collection }

func (CollectionSelected) msg() {}

type CollectionPointerSelected struct{ Key string }

func (CollectionPointerSelected) msg() {}

func updateCollection(m Model, msg CollectionSelected) (Model, []Cmd) {
	for _, collection := range m.Collections {
		if collection != msg.Collection {
			continue
		}
		m.IDSearch = IDSearch{}
		m.Collection = collection
		m = m.syncSidebarCursor()
		m.Loading = true
		m.Err = nil
		m.Refreshing = false // a switch is deliberate: it does not wait for the read of the Collection left behind
		return snapshotChanged(m)
	}
	return m, nil
}
