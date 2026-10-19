package model

import (
	"slices"
	"time"
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
	DayEnded         bool   // the end-of-day wake fired for the Pane calendar's date; the next read must show a newer date
	countsFresh      bool   // the counts on screen cover every event so far
	countsCovered    bool   // the read in flight asks for counts and no count-changing event has come since
	countsScope      string // the Project UID the counts on screen are scoped to
}

// countsKept are the event types known not to change an Issue's parent or status; any other event, including one this
// version does not know, makes the next Collection snapshot read ask for child counts.
var countsKept = []string{
	"issue.commented", "issue.comment_edited", "issue.assigned", "issue.unassigned", "issue.assignment_expired",
	"issue.assignment_renewed", "issue.labeled", "issue.unlabeled", "issue.priority_set", "issue.priority_cleared",
	"issue.updated", "issue.metadata_updated",
}

// WakeAt asks for one wake at At, the end of the local day Day; a newer WakeAt replaces an older one.
type WakeAt struct {
	Day string
	At  time.Time
}

func (WakeAt) cmd() {}

// DayEnded is the wake for the end of local day Day.
type DayEnded struct{ Day string }

func (DayEnded) msg() {}

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

type Retry struct{}

func (Retry) msg() {}

func startStream(m Model, delay time.Duration) (Model, []Cmd) {
	m.StreamGeneration++
	m.StreamConnected = false
	return m, []Cmd{Subscribe{Workspace: m.Workspace, AfterID: m.Cursor, Generation: m.StreamGeneration, Delay: delay}}
}

func updateLive(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case StreamOpened:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		m.StreamConnected = true
		if m.Stale {
			return snapshotChanged(m)
		}
	case StreamEvent:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		if msg.Type == "sync.reset_required" {
			m.Stale = true
			m.Resetting = true
			m.countsFresh = false
			m.StreamConnected = false
			m.Refreshing = false
			return snapshotChanged(m)
		}
		if msg.ID <= m.Cursor {
			return m, nil
		}
		if !slices.Contains(countsKept, msg.Type) {
			m.countsFresh, m.countsCovered = false, false
		}
		m.PendingCursor = max(m.PendingCursor, msg.ID)
		return snapshotChanged(m)
	case SearchIDLooked:
		if msg.Request != m.RefreshRequest {
			return m, nil
		}
		m.Refreshing = false
		m.IDSearch = IDSearch{ShortID: msg.ShortID, UID: msg.UID}
		return snapshotChanged(m)
	case StreamDropped:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		m.Stale = true
		m.StreamErr = msg.Err
		m.ReconnectAttempt++
		return startStream(m, time.Second<<min(m.ReconnectAttempt-1, 5))
	case DayEnded:
		if msg.Day != m.Calendar.Date {
			return m, nil
		}
		m.DayEnded = true
		m.etag = "" // a 304 would keep the old day
		return snapshotChanged(m)
	case SnapshotLoaded:
		return snapshotLoaded(m, msg)
	case SnapshotFailed:
		return snapshotFailed(m, msg)
	case Retry:
		if m.Err != nil && !m.Loading {
			m.Err = nil
			m.Loading = true
			return m, []Cmd{LoadProject{Workspace: m.Workspace}}
		}
	}
	return m, nil
}
