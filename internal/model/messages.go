package model

type Msg interface{ msg() }
type Cmd interface{ cmd() }

// Start opens the Pane; Nonce is random per run and prefixes the idempotency keys the model mints.
type Start struct{ Workspace, IssueRef, Nonce string }

func (Start) msg() {}

// ProjectLoaded is the Project the workspace resolves to; its first Collection snapshot is read next.
type ProjectLoaded struct {
	Project Project
	Daemon  Daemon
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

type LoadProject struct{ Workspace string }

func (LoadProject) cmd() {}
