package model

import g "go.kenn.io/kata/pkg/client/generated"

type Issue = g.UIIssue
type Project = g.ProjectOut

type Model struct {
	Workspace   string
	Project     Project
	Snapshot    g.UISnapshotResponseBody
	Issues      []Issue
	ETag        string
	SelectedUID string
	Loading     bool
	Err         error
	Cols        int
	Quitting    bool
	HelpOpen    bool
}

func Update(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case Start:
		m.Workspace = msg.Workspace
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		return updateProject(m, msg)
	case Failed:
		m.Loading = false
		m.Err = msg.Err
	case KeyMsg:
		if binding, ok := LookupBindingForMode(msg.Chord, m.CurrentMode()); ok {
			if binding.Action == ToggleHelp {
				m.HelpOpen = !m.HelpOpen
				return m, nil
			}
			if binding.Action == CloseHelp {
				m.HelpOpen = false
				return m, nil
			}
			return updateNavigation(m, binding.Action)
		}
	case SelectMsg:
		for _, issue := range m.Issues {
			if issue.UID == msg.UID {
				m.SelectedUID = msg.UID
				break
			}
		}
	case ResizeMsg:
		m.Cols = msg.Cols
	}
	return m, nil
}
