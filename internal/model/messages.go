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

type ResizeMsg struct{ Cols int }

func (ResizeMsg) msg() {}

type LoadProject struct{ Workspace string }

func (LoadProject) cmd() {}
