package model

import (
	"fmt"
	"strings"

	g "go.kenn.io/kata/pkg/client/generated"
)

type ActionMsg struct {
	Action Action
	Value  string
}

func (ActionMsg) msg() {}

type OpenIssue struct {
	Workspace, Ref string
	ProjectID      int64
}

func (OpenIssue) cmd() {}

type IssueOpened struct {
	Ref      string
	Selected *g.UISelectedAuthority
}

func (IssueOpened) msg() {}

type IssueOpenFailed struct {
	Ref string
	Err error
}

func (IssueOpenFailed) msg() {}

func ParseIssueRef(ref string) (project, shortID string, err error) {
	project, shortID, ok := strings.Cut(ref, "/")
	if !ok || project == "" || shortID == "" || strings.ContainsAny(project+shortID, "/?#%\\\x00\r\n") {
		return "", "", fmt.Errorf("Issue reference must be <project>/<short_id>: %q", ref)
	}
	return project, shortID, nil
}

func openIssue(m Model, ref string) (Model, []Cmd) {
	project, _, err := ParseIssueRef(ref)
	if err != nil {
		m.Err = err
		return m, nil
	}
	m = m.openRequested(ref)
	if m.Loading && m.Snapshot.Catalog == nil {
		return m, nil
	}
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.Name == project {
			m.Err = nil
			return m, []Cmd{OpenIssue{Workspace: m.Workspace, Ref: ref, ProjectID: entry.Project.ID}}
		}
	}
	m = m.openFailed(ref)
	m.Err = fmt.Errorf("kata Project %q not found", project)
	return m, nil
}
