package model

import (
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
	dayInFlight      bool // the end-of-day wake fired while a read was in flight; a result still dated that day reads again
	readOwed         bool // an Issue was written while a read was in flight; that read may predate the write, so its result is followed by another
	wakeGeneration   int64
	// resubscribe says the next snapshot starts a new event stream: the Pane switched to another daemon, whose events the old stream does not carry.
	resubscribe bool
}

// WakeAt asks for one wake at At, the end of the local day Day. Generation numbers the asks; a wake replaces an older one
// and is ignored once a newer one has been asked for.
type WakeAt struct {
	Day        string
	At         time.Time
	Generation int64
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
		m, cmds := askAttention(m)
		if m.Stale {
			m, more := snapshotChanged(m)
			return m, append(cmds, more...)
		}
		return m, cmds
	case StreamEvent:
		if msg.Generation != m.StreamGeneration {
			return m, nil
		}
		var asked []Cmd
		if attentionChangesTheCount(msg.Type) {
			m, asked = askAttention(m)
		}
		if msg.Type == "sync.reset_required" {
			m.Stale = true
			m.Resetting = true
			m.StreamConnected = false
			m.Refreshing = false
			m, more := snapshotChanged(m)
			return m, append(asked, more...)
		}
		if msg.ID <= m.Cursor {
			return m, asked
		}
		m.PendingCursor = max(m.PendingCursor, msg.ID)
		m, more := snapshotChanged(m)
		return m, append(asked, more...)
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
		m.dayInFlight = m.Refreshing
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
			return m, []Cmd{LoadProject{Workspace: m.Workspace, Actor: m.Actor}}
		}
	}
	return m, nil
}
