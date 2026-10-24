package model

import (
	"fmt"
	"slices"
	"strings"
)

// A Move takes an open Issue one Lane left or right among No lane and the Lanes. It never enters or leaves Done: closing and
// reopening have their own keys.

// MoveIssueCmd is one Move as plain values. The Daemon gateway adds Add first, then removes each of Remove, and stops at the
// first failure, so a Move that fails halfway leaves the Issue in two Lanes, never none.
type MoveIssueCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	// Add is the target Lane's label, empty when the Issue already has it or the target is No lane.
	Add string
	// Remove are the Lane labels the Issue has besides the target.
	Remove []string
}

func (MoveIssueCmd) cmd() {}

// MoveResultMsg is the one answer to a MoveIssueCmd.
type MoveResultMsg struct {
	UID    string
	Err    error
	Record WriteRecord
}

func (MoveResultMsg) msg() {}

// MoveCard moves the open Issue uid one Lane towards dir (-1 left, +1 right). The card shows in its target Lane at once; the
// snapshot after the result is the truth.
func MoveCard(m Model, uid string, dir int) (Model, []Cmd) {
	if m.Moving != "" {
		return m, nil
	}
	at := slices.IndexFunc(m.Issues, func(i Issue) bool { return i.UID == uid })
	if at < 0 {
		return m, nil
	}
	issue := m.Issues[at]
	if m.Stale {
		m.Notice = "Daemon disconnected; reconnect before moving."
		return m, nil
	}
	if IsClosed(issue) {
		m.Notice = fmt.Sprintf("%s is closed; press r to reopen it.", issue.ShortID)
		return m, nil
	}
	lanes := m.LaneList()
	columns := append([]string{LaneNone}, lanes...)
	to := slices.Index(columns, LabelLane(issue, lanes)) + dir
	switch {
	case to < 0:
		return m, nil
	case to >= len(columns):
		m.Notice = fmt.Sprintf("%s is in the last Lane; press x to close it.", issue.ShortID)
		return m, nil
	}
	target := columns[to]
	cmd := MoveIssueCmd{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Actor: m.Actor}
	if target != LaneNone && !slices.Contains(issue.Labels, target) {
		cmd.Add = target
	}
	for _, label := range LaneLabels(issue, lanes) {
		if label != target {
			cmd.Remove = append(cmd.Remove, label)
		}
	}
	issue.Labels = slices.DeleteFunc(slices.Clone(issue.Labels), func(l string) bool { return slices.Contains(cmd.Remove, l) })
	if cmd.Add != "" {
		issue.Labels = append(issue.Labels, cmd.Add)
	}
	m.Issues = slices.Clone(m.Issues)
	m.Issues[at] = issue
	m.Moving = uid
	if !matchesLabelFilter(issue, m.Filters.Label) {
		m.Notice = fmt.Sprintf("%s moved to %s, outside the current filters.", issue.ShortID, LaneTitle(target))
	}
	return m, []Cmd{cmd}
}

// matchesLabelFilter says issue carries any label of the comma-separated label filter, as kata's filter does; an empty filter matches.
func matchesLabelFilter(issue Issue, filter string) bool {
	empty := true
	for _, label := range strings.Split(filter, ",") {
		if label = strings.TrimSpace(label); label != "" {
			empty = false
			if slices.ContainsFunc(issue.Labels, func(l string) bool { return strings.EqualFold(l, label) }) {
				return true
			}
		}
	}
	return empty
}

func updateMove(m Model, msg MoveResultMsg) (Model, []Cmd) {
	m.Moving = ""
	if msg.Err != nil {
		m.Notice = msg.Err.Error()
	}
	return snapshotChanged(m.issueWritten(msg.UID))
}
