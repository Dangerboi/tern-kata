package model

type Collection struct{ View, ProjectUID, Title string }

// IsProject says the Collection is one Project's Issues, not a named view.
func (c Collection) IsProject() bool { return c.View == projectDef.Key }

// CatalogEntry is the snapshot's catalog entry for the Project with uid.
func CatalogEntry(snapshot Snapshot, uid string) (CatalogProject, bool) {
	for _, entry := range snapshot.Catalog {
		if entry.Project.UID == uid {
			return entry, true
		}
	}
	return CatalogProject{}, false
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
		m.Collection = collection
		return m.collectionChosen()
	}
	return m, nil
}

// collectionChosen reads the snapshot of the Collection the Pane just switched to.
func (m Model) collectionChosen() (Model, []Cmd) {
	m = m.syncSidebarCursor()
	m.Loading = true
	m.Err = nil
	return collectionSwitched(m)
}
