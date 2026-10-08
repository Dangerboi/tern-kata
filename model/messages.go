package model

import g "go.kenn.io/kata/pkg/client/generated"

type Msg interface{ msg() }
type Cmd interface{ cmd() }

type Start struct{ Workspace, IssueRef string }

func (Start) msg() {}

type ProjectLoaded struct {
	Project             Project
	Snapshot            g.UISnapshotResponseBody
	ETag                string
	LocalDate, TimeZone string
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

type Quit struct{}

func (Quit) cmd() {}
