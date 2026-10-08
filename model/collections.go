package model

import g "go.kenn.io/kata/pkg/client/generated"

type Collection struct{ View, ProjectUID, Title string }
type IssueGroup struct {
	ID, Title string
	Issues    []Issue
}
type CollectionSelected struct{ Collection Collection }

func (CollectionSelected) msg() {}

type CollectionPointerSelected struct{ ID string }

func (CollectionPointerSelected) msg() {}

type CollectionLoaded struct {
	Collection                Collection
	Snapshot                  g.UISnapshotResponseBody
	ETag, LocalDate, TimeZone string
	NotModified               bool
}

func (CollectionLoaded) msg() {}

type LoadCollection struct {
	Workspace  string
	Collection Collection
	ETag       string
}

func (LoadCollection) cmd() {}

func (m Model) SnapshotQuery() g.ReadUISnapshotQuery {
	q := g.ReadUISnapshotQuery{View: new(m.Collection.View)}
	if m.Collection.ProjectUID != "" {
		q.ProjectUID = new(m.Collection.ProjectUID)
		q.Status = []string{"open"}
	}
	if m.Collection.View == "today" || m.Collection.View == "upcoming" || m.Collection.View == "deadlines" {
		q.LocalDate = new(m.LocalDate)
		q.TimeZone = new(m.TimeZone)
	}
	return q
}

func (m Model) ApplySnapshot(snapshot g.UISnapshotResponseBody, etag string) Model {
	m.Snapshot = snapshot
	m.Cursor = snapshot.Cursor
	m.ETag = etag
	m.Collections = []Collection{{View: "inbox", Title: "Inbox"}, {View: "today", Title: "Today"}, {View: "upcoming", Title: "Upcoming"}, {View: "deadlines", Title: "Deadlines"}, {View: "all-open", Title: "All Open"}, {View: "logbook", Title: "Logbook"}}
	for _, p := range snapshot.Catalog {
		m.Collections = append(m.Collections, Collection{View: "project", ProjectUID: p.Project.UID, Title: p.Project.Name})
	}
	m.Groups = GroupIssues(m.Collection, snapshot.Collection, m.LocalDate)
	m.Issues = nil
	for _, group := range m.Groups {
		m.Issues = append(m.Issues, group.Issues...)
	}
	m.Loading = false
	m.Err = nil
	for i, collection := range m.Collections {
		if collection == m.Collection {
			m.CollectionCursor = i
			break
		}
	}
	if m.Focus == ColumnDetail && m.OpenedIssueUID != "" && m.SelectedUID == m.OpenedIssueUID {
		return m
	}
	return m.WithVisibleSelection()
}

func updateCollection(m Model, msg CollectionSelected) (Model, []Cmd) {
	for i, collection := range m.Collections {
		if collection != msg.Collection {
			continue
		}
		etag := ""
		if collection == m.Collection {
			etag = m.ETag
		}
		m.ETag = etag
		m.Collection = collection
		m.CollectionCursor = i
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadCollection{Workspace: m.Workspace, Collection: collection, ETag: etag}}
	}
	return m, nil
}

func collectionLoaded(m Model, msg CollectionLoaded) (Model, []Cmd) {
	if msg.Collection != m.Collection {
		return m, nil
	}
	m.Loading = false
	m.Err = nil
	if msg.NotModified {
		return m, nil
	}
	m.LocalDate = msg.LocalDate
	m.TimeZone = msg.TimeZone
	return m.ApplySnapshot(msg.Snapshot, msg.ETag), nil
}
