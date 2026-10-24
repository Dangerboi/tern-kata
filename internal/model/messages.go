package model

type Msg interface{ msg() }
type Cmd interface{ cmd() }

// Start opens the Pane; Nonce is random per run and prefixes the idempotency keys the model mints.
type Start struct{ Workspace, IssueRef, Nonce string }

func (Start) msg() {}

// ProjectLoaded is the Project the workspace resolves to; its first Collection snapshot is read next.
type ProjectLoaded struct {
	// Connection is the number of the connection LoadProject was asked on.
	Connection int
	Project    Project
	Daemon     Daemon
	// Default is the Project's saved default filter, read with the Project so the first snapshot read carries it. DefaultErr
	// is why the file could not be read: the Pane starts unfiltered and says so.
	Default    Filters
	DefaultErr error
}

func (ProjectLoaded) msg() {}

type Failed struct{ Err error }

func (Failed) msg() {}

type KeyMsg struct{ Chord, Text string }

func (KeyMsg) msg() {}

type SelectMsg struct{ UID string }

func (SelectMsg) msg() {}

// ResizeMsg is the Pane's new size: its width in columns, its height in rows and the height of a row in pixels. Rows and RowPx are
// zero when Tern does not say.
type ResizeMsg struct{ Cols, Rows, RowPx int }

func (ResizeMsg) msg() {}

// PaneRowsMsg is the Pane's new height in rows when only its height changed: Tern reports a resize only when the width changes, so
// the Pane's terminal size is watched for the height.
type PaneRowsMsg struct{ Rows int }

func (PaneRowsMsg) msg() {}

// LoadProject reads the Project the workspace is bound to. Actor is who the Pane acts as, for the one-time move of old
// Attachment pointers that a Pane start triggers.
//
// Switched says the Pane just switched to another daemon: a Project the workspace is bound to but that daemon does not know is then
// not a failure, and the Pane starts on its Inbox.
type LoadProject struct {
	Workspace, Actor string
	Connection       int
	Switched         bool
}

func (LoadProject) cmd() {}
