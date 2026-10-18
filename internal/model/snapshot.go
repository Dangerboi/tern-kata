package model

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
)

// The Collection snapshot is how the shown Collection is brought in line with the daemon: the first load, a Collection switch,
// a filter change, an id search and a live refresh all go through snapshotChanged (the writers' one call) and arrive as one
// SnapshotLoaded. It owns the ETag, the stale-result guard, the pending selection after Create and the short-id lookup.

// ReadSnapshot asks the daemon for the snapshot answering Query. ETag is sent only when the snapshot on screen answers the same query.
type ReadSnapshot struct {
	Workspace string
	Query     g.ReadUISnapshotQuery
	ETag      string
	Request   int64
}

func (ReadSnapshot) cmd() {}

// SnapshotLoaded is the one message that carries a snapshot into the model. Query is the query it answers; the daemon side
// adds the local calendar the dated Collections need and reports it in LocalDate and TimeZone.
type SnapshotLoaded struct {
	Request             int64
	Query               g.ReadUISnapshotQuery
	Snapshot            g.UISnapshotResponseBody
	Selected            *IssueDetailData // the snapshot's selected part, translated by the gateway
	ChildCounts         map[string]g.ChildCounts
	ETag                string
	NotModified         bool
	LocalDate, TimeZone string
	TimeZoneNote        string // set when UTC stands in for an unknown or unloadable zone
}

func (SnapshotLoaded) msg() {}

type SnapshotFailed struct {
	Request int64
	Err     error
}

func (SnapshotFailed) msg() {}

// DatedView says the Collection's query needs the local calendar date.
func DatedView(view string) bool {
	return view == "today" || view == "upcoming" || view == "deadlines"
}

// CollectionQuery is the snapshot query for a Collection and the filters alone.
func CollectionQuery(c Collection, f Filters) g.ReadUISnapshotQuery {
	q := g.ReadUISnapshotQuery{View: new(c.View)}
	if c.ProjectUID != "" {
		q.ProjectUID = new(c.ProjectUID)
	} else if f.ProjectUID != "" {
		q.ProjectUID = new(f.ProjectUID)
	}
	if f.Status != "" {
		q.Status = []string{f.Status}
	}
	if f.Search != "" {
		q.Text = new(f.Search)
	}
	if f.Owner != "" {
		q.Owner = []string{f.Owner}
	}
	for _, label := range strings.Split(f.Label, ",") {
		if label = strings.TrimSpace(label); label != "" {
			q.Label = append(q.Label, label)
		}
	}
	if f.Relationship != "" {
		q.Relationship = []g.ReadUISnapshotQueryRelationship{g.ReadUISnapshotQueryRelationship(f.Relationship)}
	}
	return q
}

// SnapshotQuery is the query the shown Collection answers; a search that names an Issue by short id is answered by that Issue, not by text.
func (m Model) SnapshotQuery() g.ReadUISnapshotQuery {
	q := CollectionQuery(m.Collection, m.Filters)
	if m.idMatch() != "" {
		q.Text = nil
	}
	return q
}

// liveQuery is SnapshotQuery plus the selected Issue's detail, which rides along so the shown Issue refreshes in place.
func liveQuery(m Model) g.ReadUISnapshotQuery {
	q := m.SnapshotQuery()
	if m.SelectedUID != "" && !m.DetailHidden() {
		q.SelectedIssueUID = new(m.SelectedUID)
		q.IncludeHistory = new(true)
	}
	return q
}

// sameCollectionQuery compares what picks the Issues, not the detail riding along.
func sameCollectionQuery(a, b g.ReadUISnapshotQuery) bool {
	a.SelectedIssueUID, b.SelectedIssueUID = nil, nil
	a.IncludeHistory, b.IncludeHistory = nil, nil
	return reflect.DeepEqual(a, b)
}

// snapshotChanged is what writers say: something changed, bring the Collection in line. It reads the snapshot unless a read is
// already in flight; that read's result, if it no longer answers the current query, asks again.
func snapshotChanged(m Model) (Model, []Cmd) {
	if _, ok := searchShortID(m.Filters.Search); !ok {
		m.IDSearch = IDSearch{} // an answer is kept only while the query stays an id, so retyping the id asks again
	}
	if m.Refreshing {
		return m, nil
	}
	m.Refreshing = true
	m.RefreshRequest++
	if lookup, ok := m.idLookup(); ok {
		lookup.Request = m.RefreshRequest
		return m, []Cmd{lookup}
	}
	query := liveQuery(m)
	etag := ""
	if !m.Resetting && sameCollectionQuery(query, m.etagQuery) {
		etag = m.etag
	}
	return m, []Cmd{ReadSnapshot{Workspace: m.Workspace, Query: query, ETag: etag, Request: m.RefreshRequest}}
}

// collectionSwitched is a Collection switch: deliberate, so it neither waits for the read of the Collection left behind (that
// answer is dropped by its Request) nor keeps an id answer for it.
func collectionSwitched(m Model) (Model, []Cmd) {
	m.Refreshing = false
	m.IDSearch = IDSearch{}
	return snapshotChanged(m)
}

// snapshotLoaded applies a result only if it answers the latest read (Request, the in-flight supersede guard) and the current query.
func snapshotLoaded(m Model, msg SnapshotLoaded) (Model, []Cmd) {
	if msg.Request != m.RefreshRequest {
		return m, nil
	}
	m.Refreshing = false
	if !sameCollectionQuery(liveQuery(m), msg.Query) {
		return snapshotChanged(m)
	}
	if uid := m.idMatch(); uid != "" && !msg.NotModified && !slices.ContainsFunc(msg.Snapshot.Collection, func(issue Issue) bool { return issue.UID == uid }) {
		// The Collection does not list the Issue the id names, so the id does not count.
		m.IDSearch.UID = ""
		return snapshotChanged(m)
	}
	m.Loading = false
	m.Err = nil
	var cmds []Cmd
	if !msg.NotModified {
		if msg.LocalDate != "" {
			m.LocalDate, m.TimeZone, m.TimeZoneNote = msg.LocalDate, msg.TimeZone, msg.TimeZoneNote
			m.zone, _ = time.LoadLocation(msg.TimeZone) // an unloadable name reads as UTC
		}
		selected := m.SelectedUID
		anchor, anchored := m.cursorRowID()
		m = m.applyCollection(msg.Snapshot, msg.ChildCounts).snapshotArrived(msg.Selected, selected, anchor, anchored)
		m.etag, m.etagQuery = msg.ETag, msg.Query
		m.Cursor = msg.Snapshot.Cursor
	}
	if m.StreamGeneration == 0 {
		// The first snapshot is on screen: follow the daemon from its cursor, and open the Issue the Pane was asked to show.
		m, cmds = startStream(m, 0)
		if ref := m.OpenIssueRef(); ref != "" {
			var open []Cmd
			m, open = openIssue(m, ref)
			cmds = append(cmds, open...)
		}
	}
	if m.Resetting {
		m.Resetting = false
		m.PendingCursor = 0
		return startStream(m, 0)
	}
	if m.StreamConnected {
		m.Stale = false
		m.StreamErr = nil
		m.ReconnectAttempt = 0
	}
	if _, lookup := m.idLookup(); lookup || m.PendingCursor > m.Cursor {
		// A short id the lookup has not answered, or events newer than this snapshot.
		var next []Cmd
		m, next = snapshotChanged(m)
		cmds = append(cmds, next...)
	}
	return m, cmds
}

// snapshotFailed ends the read: on the first load or a Collection switch it is the error shown; otherwise the Pane is stale.
func snapshotFailed(m Model, msg SnapshotFailed) (Model, []Cmd) {
	if msg.Request != m.RefreshRequest {
		return m, nil
	}
	m.Refreshing = false
	if m.Loading {
		m.Loading = false
		m.Err = msg.Err
		return m, nil
	}
	m.Stale = true
	m.StreamErr = msg.Err
	m.ReconnectAttempt++
	return startStream(m, time.Second<<min(m.ReconnectAttempt-1, 5))
}

// selectAfterCreate selects the Issue just created now, and again once the snapshot lists it.
func (m Model) selectAfterCreate(uid string) Model {
	m.pending = uid
	m.SelectedUID = uid
	return m
}

// cancelPending drops the pending selection when anything but a snapshot moved the cursor, the Collection, the filters or the opened Issue.
func (m Model) cancelPending(before Model, msg Msg) Model {
	if _, snapshot := msg.(SnapshotLoaded); snapshot || before.pending == "" || m.pending != before.pending {
		return m
	}
	if m.SelectedUID != before.SelectedUID || m.Collection != before.Collection || m.Filters != before.Filters ||
		m.CollectionCursor != before.CollectionCursor || m.openRef != before.openRef {
		m.pending = ""
	}
	return m
}

// applyCollection puts a snapshot on screen: the Collections, the grouped Issues, and the selection.
func (m Model) applyCollection(snapshot g.UISnapshotResponseBody, childCounts map[string]g.ChildCounts) Model {
	m.Snapshot = snapshot
	m.ChildCounts = childCounts
	m.Collections = []Collection{{View: "inbox", Title: "Inbox"}, {View: "today", Title: "Today"}, {View: "upcoming", Title: "Upcoming"}, {View: "deadlines", Title: "Deadlines"}, {View: "all-open", Title: "All Open"}, {View: "logbook", Title: "Logbook"}}
	for _, p := range snapshot.Catalog {
		m.Collections = append(m.Collections, Collection{View: "project", ProjectUID: p.Project.UID, Title: p.Project.Name})
	}
	groupCollection := m.Collection
	if m.Filters != (Filters{}) {
		groupCollection = Collection{View: "results", Title: "Results"}
	}
	issues := snapshot.Collection
	if uid := m.idMatch(); uid != "" {
		issues = slices.DeleteFunc(slices.Clone(issues), func(issue Issue) bool { return issue.UID != uid })
	}
	// Snapshot accepts one Project UID; incompatible Project scopes intersect to empty.
	if m.Collection.ProjectUID != "" && m.Filters.ProjectUID != "" && m.Collection.ProjectUID != m.Filters.ProjectUID {
		issues = nil
	}
	m.Groups = GroupIssues(groupCollection, issues, m.LocalDate)
	for i := range m.Groups {
		m.Groups[i].Heading = !m.Collection.IsProject()
	}
	m.Issues = nil
	for _, group := range m.Groups {
		m.Issues = append(m.Issues, group.Issues...)
	}
	m = m.syncSidebarCursor()
	if m.pending != "" {
		m.SelectedUID = m.pending
		for _, issue := range m.Issues {
			if issue.UID == m.pending {
				m.ExpandedUIDs = maps.Clone(m.ExpandedUIDs)
				if m.ExpandedUIDs == nil {
					m.ExpandedUIDs = map[string]bool{}
				}
				parents, _ := m.issueHierarchy()
				for uid := parents[issue.UID]; uid != ""; uid = parents[uid] {
					m.ExpandedUIDs[uid] = true
				}
				m.pending = ""
				break
			}
		}
		if m.pending != "" {
			return m
		}
	}
	if m.Focus == ColumnDetail && m.OpenedIssueUID() != "" && m.SelectedUID == m.OpenedIssueUID() {
		return m
	}
	return m.WithVisibleSelection()
}
