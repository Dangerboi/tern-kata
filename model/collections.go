package model

import (
	"strings"

	g "go.kenn.io/kata/pkg/client/generated"
)

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
	Filters Filters
}

func (CollectionLoaded) msg() {}

type LoadCollection struct {
	Workspace  string
	Collection Collection
	ETag       string
	Filters Filters
}

func (LoadCollection) cmd() {}

func (m Model) SnapshotQuery() g.ReadUISnapshotQuery {
	q := g.ReadUISnapshotQuery{View: new(m.Collection.View)}
	if m.Collection.ProjectUID != "" {
		q.ProjectUID = new(m.Collection.ProjectUID)
		q.Status = []string{"open"}
	} else if m.Filters.ProjectUID != "" {
		q.ProjectUID = new(m.Filters.ProjectUID)
	}
	if m.Collection.View == "today" || m.Collection.View == "upcoming" || m.Collection.View == "deadlines" {
		q.LocalDate = new(m.LocalDate)
		q.TimeZone = new(m.TimeZone)
	}
	if m.Filters.Status != "" {
		q.Status = []string{m.Filters.Status}
	}
	if m.Filters.Search != "" {
		q.Text = new(m.Filters.Search)
	}
	if m.Filters.Owner != "" {
		q.Owner = []string{m.Filters.Owner}
	}
	for _, label := range strings.Split(m.Filters.Label, ",") {
		if label = strings.TrimSpace(label); label != "" {
			q.Label = append(q.Label, label)
		}
	}
	if m.Filters.Relationship != "" {
		q.Relationship = []g.ReadUISnapshotQueryRelationship{g.ReadUISnapshotQueryRelationship(m.Filters.Relationship)}
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
	groupCollection := m.Collection
	if m.Filters != (Filters{}) {
		groupCollection = Collection{View: "results", Title: "Results"}
	}
	issues := snapshot.Collection
	// Snapshot accepts one Project UID; incompatible Project scopes intersect to empty.
	if m.Collection.ProjectUID != "" && m.Filters.ProjectUID != "" && m.Collection.ProjectUID != m.Filters.ProjectUID {
		issues = nil
	}
	m.Groups = GroupIssues(groupCollection, issues, m.LocalDate)
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
		return m, []Cmd{LoadCollection{Workspace: m.Workspace, Collection: collection, ETag: etag, Filters: m.Filters}}
	}
	return m, nil
}

func collectionLoaded(m Model, msg CollectionLoaded) (Model, []Cmd) {
	if msg.Collection != m.Collection || msg.Filters != m.Filters {
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
