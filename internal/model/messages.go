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

// ButtonPressMsg is a click on a card's button: Chord is pressed after the card's open calendar and dropdown are closed, so the
// button does what it says where the same key would first drive the overlay.
type ButtonPressMsg struct{ Chord string }

func (ButtonPressMsg) msg() {}

// ContextMenuPickMsg is a pick from the context menu of an Issue's row or card: select the Issue, then run what Chord is bound to in
// Mode, the mode the menu was built for, whatever mode the Pane is in now.
type ContextMenuPickMsg struct {
	UID   string
	Mode  Mode
	Chord string
}

func (ContextMenuPickMsg) msg() {}

// ResizeMsg is the Pane's new size: its width in columns and its height in rows. Rows is zero when the terminal does not say.
type ResizeMsg struct{ Cols, Rows int }

func (ResizeMsg) msg() {}

// PaneRowsMsg is the Pane's new height in rows when only its height changed: Tern reports a resize only when the width changes, so
// the Pane's terminal size is watched for the height.
type PaneRowsMsg struct{ Rows int }

func (PaneRowsMsg) msg() {}

// LoadProject reads the Project the workspace is bound to.
//
// Switched says the Pane just switched to another daemon: a Project the workspace is bound to but that daemon does not know is then
// not a failure, and the Pane starts on its Inbox.
type LoadProject struct {
	Workspace  string
	Connection int
	Switched   bool
}

func (LoadProject) cmd() {}
