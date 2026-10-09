package model

import (
	"reflect"
	"slices"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
)

type LiveState struct {
	Cursor           int64
	StreamGeneration int64
	StreamConnected  bool
	Stale            bool
	StreamErr        error
	ReconnectAttempt int
	Refreshing       bool
	RefreshRequest   int64
	PendingCursor    int64
	Resetting        bool
}

type Subscribe struct {
	Workspace  string
	AfterID    int64
	Generation int64
	Delay      time.Duration
}

func (Subscribe) cmd() {}

type StreamOpened struct{ Generation int64 }

func (StreamOpened) msg() {}

type StreamEvent struct {
	Generation int64
	ID         int64
	Type       string
}

func (StreamEvent) msg() {}

type StreamDropped struct {
	Generation int64
	Err        error
}

func (StreamDropped) msg() {}

type RefreshSnapshot struct {
	Workspace string
	Query     g.ReadUISnapshotQuery
	ETag      string
	Request   int64
}

func (RefreshSnapshot) cmd() {}

type SnapshotRefreshed struct {
	Snapshot    g.UISnapshotResponseBody
	ChildCounts map[string]g.ChildCounts
	Query       g.ReadUISnapshotQuery
	ETag        string
	NotModified bool
	Request     int64
}

func (SnapshotRefreshed) msg() {}

type SnapshotFailed struct {
	Request int64
	Err     error
}

func (SnapshotFailed) msg() {}

type Retry struct{}

func (Retry) msg() {}

func liveQuery(m Model) g.ReadUISnapshotQuery {
	q := m.SnapshotQuery()
	if m.SelectedUID != "" && !m.DetailHidden {
		q.SelectedIssueUID = new(m.SelectedUID)
		q.IncludeHistory = new(true)
	}
	return q
}

func startStream(m Model, delay time.Duration) (Model, []Cmd) {
	m.StreamGeneration++
	m.StreamConnected = false
	return m, []Cmd{Subscribe{Workspace: m.Workspace, AfterID: m.Cursor, Generation: m.StreamGeneration, Delay: delay}}
}

func refreshLive(m Model) (Model, []Cmd) {
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
	etag := m.ETag
	if m.Resetting {
		etag = ""
	}
	return m, []Cmd{RefreshSnapshot{Workspace: m.Workspace, Query: liveQuery(m), ETag: etag, Request: m.RefreshRequest}}
}

func updateLive(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case StreamOpened:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		m.StreamConnected = true
		if m.Stale {
			return refreshLive(m)
		}
	case StreamEvent:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		if msg.Type == "sync.reset_required" {
			m.Stale = true
			m.Resetting = true
			m.StreamConnected = false
			m.Refreshing = false
			return refreshLive(m)
		}
		if msg.ID <= m.Cursor {
			return m, nil
		}
		m.PendingCursor = max(m.PendingCursor, msg.ID)
		return refreshLive(m)
	case SearchIDLooked:
		if msg.Request != m.RefreshRequest {
			return m, nil
		}
		m.Refreshing = false
		m.IDSearch = IDSearch{ShortID: msg.ShortID, UID: msg.UID}
		return refreshLive(m)
	case StreamDropped:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		m.Stale = true
		m.StreamErr = msg.Err
		m.ReconnectAttempt++
		return startStream(m, time.Second<<min(m.ReconnectAttempt-1, 5))
	case SnapshotRefreshed:
		if msg.Request != m.RefreshRequest {
			return m, nil
		}
		m.Refreshing = false
		current, received := liveQuery(m), msg.Query
		current.SelectedIssueUID, received.SelectedIssueUID = nil, nil
		current.IncludeHistory, received.IncludeHistory = nil, nil
		if !reflect.DeepEqual(current, received) {
			return refreshLive(m)
		}
		if uid := m.idMatch(); uid != "" && !msg.NotModified && !slices.ContainsFunc(msg.Snapshot.Collection, func(issue Issue) bool { return issue.UID == uid }) {
			// The Collection does not list the Issue the id names, so the id does not count.
			m.IDSearch.UID = ""
			return refreshLive(m)
		}
		if !msg.NotModified {
			selected := m.SelectedUID
			cursorChild := m.cursorChildUID()
			m = m.ApplySnapshot(msg.Snapshot, msg.ChildCounts, msg.ETag)
			if m.Focus == ColumnDetail && len(m.DetailBack) > 0 {
				m.SelectedUID = selected
			}
			if msg.Snapshot.Selected != nil && msg.Snapshot.Selected.Issue != nil && msg.Snapshot.Selected.Issue.UID == m.SelectedUID {
				m.Detail = msg.Snapshot.Selected
				m.DetailUID = m.SelectedUID
				m.DetailLoading = false
				m.DetailErr = nil
				m.DetailIndex = min(m.DetailIndex, max(m.DetailCursorCount()-1, 0))
				for i, child := range m.DetailChildren() {
					if cursorChild != "" && child.UID == cursorChild {
						m.DetailIndex = i
					}
				}
			} else if m.DetailUID == m.SelectedUID {
				m.DetailUID = ""
			}
			m.Cursor = msg.Snapshot.Cursor
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
		if m.PendingCursor > m.Cursor {
			return refreshLive(m)
		}
	case SnapshotFailed:
		if msg.Request != m.RefreshRequest {
			return m, nil
		}
		m.Refreshing = false
		m.Stale = true
		m.StreamErr = msg.Err
		m.ReconnectAttempt++
		return startStream(m, time.Second<<min(m.ReconnectAttempt-1, 5))
	case Retry:
		if m.Err != nil && !m.Loading {
			m.Err = nil
			m.Loading = true
			return m, []Cmd{LoadProject{Workspace: m.Workspace}}
		}
	}
	return m, nil
}
