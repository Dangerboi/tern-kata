package model

import (
	"fmt"
	"slices"
	"strings"
)

// A Move takes an open Issue one Lane left or right among No lane and the Lanes, or, as a drag of its card onto another Lane
// on the Board, straight to that Lane. It never enters or leaves Done: closing and reopening have their own keys, and a card
// dragged onto Done opens the Close card.

// MoveIssueCmd is one Move as plain values. The Daemon gateway adds Add first, then removes each of Remove, and stops at the
// first failure, so a Move that fails halfway leaves the Issue in two Lanes, never none.
type MoveIssueCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	// Add is the target Lane's label, empty when the Issue already has it or the target is No lane.
	Add    string
	Remove []string
}

func (MoveIssueCmd) cmd() {}

type MoveResultMsg struct {
	UID    string
	Err    error
	Record WriteRecord
}

func (MoveResultMsg) msg() {}

// MoveCard moves the open Issue uid one Lane towards dir (-1 left, +1 right). The card shows in its target Lane at once; the
// snapshot after the result is the truth.
func MoveCard(m Model, uid string, dir int) (Model, []Cmd) {
	m, at, cmds, ok := movable(m, uid)
	if !ok {
		return m, cmds
	}
	issue := m.Issues[at]
	lanes := m.LaneList()
	columns := append([]string{LaneNone}, lanes...)
	to := slices.Index(columns, LabelLane(issue, lanes)) + dir
	switch {
	case to < 0:
		return m, nil
	case to >= len(columns):
		m, wait := m.raiseToast(Toast{Text: fmt.Sprintf("%s is in the last Lane.", issue.ShortID), Label: "Close", Chord: "x", UID: issue.UID})
		return m, []Cmd{wait}
	}
	return moveCardTo(m, at, columns[to])
}

// movable says whether the Issue uid may be moved now and where it is in m.Issues. When it may not, cmds and the returned
// Model are the whole answer: nothing for a Move in flight or an unknown Issue, a failure Notice while disconnected, the
// Reopen toast for a closed Issue.
func movable(m Model, uid string) (_ Model, at int, cmds []Cmd, ok bool) {
	if m.Moving != "" {
		return m, 0, nil, false
	}
	at = slices.IndexFunc(m.Issues, func(i Issue) bool { return i.UID == uid })
	if at < 0 {
		return m, 0, nil, false
	}
	issue := m.Issues[at]
	if m.Stale {
		return m.fail(Cannot("move", "", ErrDisconnected)), 0, nil, false
	}
	if IsClosed(issue) {
		m, wait := m.raiseToast(Toast{Text: fmt.Sprintf("%s is closed.", issue.ShortID), Label: "Reopen", Chord: "r", UID: issue.UID})
		return m, 0, []Cmd{wait}, false
	}
	return m, at, nil, true
}

// moveCardTo moves the open Issue m.Issues[at] straight to the Lane target (LaneNone for No lane), whichever Lane it is in.
func moveCardTo(m Model, at int, target string) (Model, []Cmd) {
	issue := m.Issues[at]
	lanes := m.LaneList()
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
	m.Moving = issue.UID
	if !matchesLabelFilter(issue, m.Filters.Label) {
		m = m.say(fmt.Sprintf("Moved %s to %s, outside the current filters.", m.RefName(issue.ProjectName, issue.ShortID), LaneTitle(target)))
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
		m = m.fail(Cannot("move", "", msg.Err))
	}
	return snapshotChanged(m.issueWritten(msg.UID))
}
