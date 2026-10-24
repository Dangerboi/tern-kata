package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The Move card moves the selected Issue to another Project, through kata's move action. Its list is the Collection snapshot's
// Projects without the Issue's own and the archived ones; Enter on one asks kata for a dry run, which kata checks and answers
// without changing anything, and Enter again moves. kata assigns the new short id only when it really moves, so the preview
// names the Project and not the id. (The Lane Move of a card is a different thing: see Move in the glossary.)

const (
	ModeMoveProject Mode = "move-project"
	// ModeMoveConfirm is the card after a Project is chosen: previewing, previewed, or showing a conflict.
	ModeMoveConfirm Mode = "move-confirm"

	OpenMoveProject   Action = "open-move-project"
	MoveProjectChoose Action = "move-project-choose"
	MoveProjectSend   Action = "move-project-send"
	MoveProjectBack   Action = "move-project-back"
	MoveProjectClose  Action = "move-project-close"
)

// MoveToProjectCmd is one request to kata's move action. A DryRun changes nothing; Revision is the revision the card opened
// with, sent as If-Match, and zero for the fresh preview that follows a conflict.
type MoveToProjectCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	ToProjectUID          string
	DryRun                bool
	Revision              int64
}

func (MoveToProjectCmd) cmd() {}

// ProjectMoveAnswer is the one answer to a MoveToProjectCmd. Revision is the Issue's revision kata reported; NewShortID is set
// only by a move that happened; Conflict says kata refused the revision as stale, with Err nil.
type ProjectMoveAnswer struct {
	UID, ToProjectUID string
	DryRun            bool
	Revision          int64
	NewShortID        string
	Conflict          bool
	Err               error
}

func (ProjectMoveAnswer) msg() {}

// MoveProjectState is the Move card.
type MoveProjectState struct {
	Open bool
	// Sending says a request is out; the card takes no keys until it answers.
	Sending bool
	UID     string
	// ProjectID, ShortID and From are the Issue's Project and short id and the Project's name when the card opened.
	ProjectID int64
	ShortID   string
	From      string
	// Revision is what the card opened with, the guard of the move.
	Revision int64
	Filter   string
	Caret    Caret
	Cursor   int
	// Target is the Project chosen, zero while the card still lists Projects.
	Target Project
	// Previewed says kata accepted the dry run for Target, so Enter moves.
	Previewed bool
	// Conflict says kata refused the revision: Enter asks for a fresh preview.
	Conflict bool
	Err      error
}

// NoticeLink is the part of the Notice that is a link opening an Issue; Ref is <project>/<short id>.
type NoticeLink struct{ Text, Ref string }

// OpenNoticeLinkMsg is a click on the Notice's link.
type OpenNoticeLinkMsg struct{}

func (OpenNoticeLinkMsg) msg() {}

// MoveTargets are the Projects the open card lists: the catalog's, without the Issue's own and archived ones, whose name
// contains the filter in any case.
func (m Model) MoveTargets() []Project {
	f := m.MoveProject
	filter := strings.ToLower(f.Filter)
	var targets []Project
	for _, entry := range m.Snapshot.Catalog {
		p := entry.Project
		if p.ID == f.ProjectID || p.Archived || !strings.Contains(strings.ToLower(p.Name), filter) {
			continue
		}
		targets = append(targets, p)
	}
	return targets
}

// MoveCardMode is the keymap mode of the open card.
func (m Model) MoveCardMode() Mode {
	if m.MoveProject.Target.UID != "" {
		return ModeMoveConfirm
	}
	return ModeMoveProject
}

// MovePreview is the sentence the card shows for the chosen Project: what a dry run accepted, or the conflict. Empty before
// kata answered.
func (m Model) MovePreview() string {
	f := m.MoveProject
	switch {
	case f.Conflict:
		return fmt.Sprintf("#%s changed since this card opened; nothing was moved.", f.ShortID)
	case f.Previewed:
		return fmt.Sprintf("#%s moves to %s and gets its new id there.", f.ShortID, f.Target.Name)
	}
	return ""
}

// openMoveProject is `M`.
func openMoveProject(m Model) Model {
	if m.Stale || m.MoveProject.Sending {
		return m
	}
	issue := closeIssue(m)
	if issue == nil {
		return m
	}
	m.MoveProject = MoveProjectState{Open: true, UID: issue.UID, ProjectID: issue.ProjectID, ShortID: issue.ShortID, From: issue.ProjectName, Revision: issue.Revision}
	return m
}

// sendMoveProject sends the dry run or the move for the chosen Project.
func sendMoveProject(m Model, dryRun bool) (Model, []Cmd) {
	f := &m.MoveProject
	if m.Stale {
		f.Err = errors.New("Daemon disconnected; reconnect before moving.")
		return m, nil
	}
	cmd := MoveToProjectCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, ToProjectUID: f.Target.UID, DryRun: dryRun, Revision: f.Revision}
	if f.Conflict {
		cmd.Revision = 0 // the fresh preview reads the revision as it stands now
	}
	f.Sending, f.Err = true, nil
	return m, []Cmd{cmd}
}

// chooseMoveTarget is Enter in the list: preview the Project under the cursor.
func chooseMoveTarget(m Model) (Model, []Cmd) {
	targets := m.MoveTargets()
	if len(targets) == 0 {
		return m, nil
	}
	f := &m.MoveProject
	f.Target, f.Previewed, f.Conflict = targets[min(f.Cursor, len(targets)-1)], false, false
	return sendMoveProject(m, true)
}

// confirmMove is Enter after a Project is chosen: move when previewed, preview again after a conflict.
func confirmMove(m Model) (Model, []Cmd) {
	f := &m.MoveProject
	switch {
	case f.Conflict:
		return sendMoveProject(m, true)
	case f.Previewed:
		return sendMoveProject(m, false)
	}
	return m, nil
}

// updateMoveProject applies kata's answer to the card: a stale one is dropped.
func updateMoveProject(m Model, msg ProjectMoveAnswer) (Model, []Cmd) {
	f := &m.MoveProject
	if !f.Open || !f.Sending || f.UID != msg.UID || f.Target.UID != msg.ToProjectUID {
		return m, nil
	}
	f.Sending = false
	switch {
	case msg.Conflict:
		f.Previewed, f.Conflict, f.Err = false, true, nil
		return m, nil
	case msg.Err != nil && msg.DryRun:
		f.Target, f.Previewed, f.Conflict, f.Err = Project{}, false, false, msg.Err
		return m, nil
	case msg.Err != nil:
		f.Err = msg.Err
		return m, nil
	case msg.DryRun:
		if f.Conflict {
			f.Revision = msg.Revision
		}
		f.Previewed, f.Conflict, f.Err = true, false, nil
		return m, nil
	}
	return movedToProject(m, msg)
}

// movedToProject closes the card after the move: the Notice names the Issue's new project#id as a link, the selection goes to the
// neighbouring row once the Collection no longer shows the Issue, and the Issue's detail is left.
func movedToProject(m Model, msg ProjectMoveAnswer) (Model, []Cmd) {
	f := m.MoveProject
	m.MoveProject = MoveProjectState{}
	link := NoticeLink{Text: f.Target.Name + "#" + msg.NewShortID, Ref: f.Target.Name + "/" + msg.NewShortID}
	m.Notice, m.NoticeLink = fmt.Sprintf("Moved #%s to %s.", f.ShortID, link.Text), &link
	m.afterMove = &movedIssue{UID: f.UID, Next: m.neighbourRow(f.UID)}
	if m.OpenedIssueUID() == f.UID || m.uid == f.UID {
		m.Focus = ColumnList
		m.openedUID = ""
	}
	return snapshotChanged(m.issueWritten(f.UID))
}

// movedIssue is the Issue just moved and the row beside it, which takes the selection when a snapshot no longer lists the Issue.
type movedIssue struct{ UID, Next string }

// neighbourRow is the uid of the row after uid in the visible rows, else the one before it; empty when uid is the only row.
func (m Model) neighbourRow(uid string) string {
	rows := m.VisibleRows()
	for i, row := range rows {
		if row.UID != uid {
			continue
		}
		for _, near := range []int{i + 1, i - 1} {
			if near >= 0 && near < len(rows) && rows[near].UID != uid {
				return rows[near].UID
			}
		}
	}
	return ""
}

// afterMoveSelection puts the selection on the row beside a moved Issue once a snapshot no longer lists the Issue.
func (m Model) afterMoveSelection() Model {
	moved := m.afterMove
	if moved == nil || m.SelectedUID != moved.UID {
		return m
	}
	for _, issue := range m.Issues {
		if issue.UID == moved.UID {
			return m
		}
	}
	m.afterMove = nil
	if moved.Next != "" {
		for _, issue := range m.Issues {
			if issue.UID == moved.Next {
				m.SelectedUID = moved.Next
				return m.revealInList(moved.Next)
			}
		}
	}
	return m
}

// openNoticeLink is a click on the Notice's link: the Issue it names opens.
func openNoticeLink(m Model) (Model, []Cmd) {
	link := m.NoticeLink
	if link == nil || m.Notice == "" {
		return m, nil
	}
	m.Notice, m.NoticeLink = "", nil
	return openIssue(m, link.Ref)
}

// moveProjectKey is one key for the card on top, in either of its modes.
func moveProjectKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.MoveProject
	if f.Sending {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, m.MoveCardMode())
	if bound && m.MoveCardMode() == ModeMoveProject && f.Filter != "" && utf8.RuneCountInString(binding.Chord) == 1 {
		bound = false // once the filter has text, single characters (j, k) belong to it
	}
	if !bound {
		if m.MoveCardMode() == ModeMoveProject {
			if text, edited := editText(f.Filter, &f.Caret, "", false, msg); edited {
				f.Filter, f.Cursor, f.Err = text, 0, nil
			}
		}
		return m, nil
	}
	last := len(m.MoveTargets()) - 1
	switch binding.Action {
	case MoveProjectClose:
		f.Open = false
	case MoveProjectBack:
		f.Target, f.Previewed, f.Conflict, f.Err = Project{}, false, false, nil
	case MoveProjectChoose:
		return chooseMoveTarget(m)
	case MoveProjectSend:
		return confirmMove(m)
	case MoveDown:
		f.Cursor = max(0, min(f.Cursor+1, last))
	case MoveUp:
		f.Cursor = max(0, f.Cursor-1)
	case MoveFirst:
		f.Cursor = 0
	case MoveLast:
		f.Cursor = max(0, last)
	}
	return m, nil
}

// MoveTargetSelected is a click on a Project of the card's list; Key is its UID.
type MoveTargetSelected struct{ Key string }

func (MoveTargetSelected) msg() {}

func moveTargetSelected(m Model, key string) (Model, []Cmd) {
	f := &m.MoveProject
	if !f.Open || f.Sending || m.MoveCardMode() != ModeMoveProject {
		return m, nil
	}
	for i, p := range m.MoveTargets() {
		if p.UID == key {
			f.Cursor = i
			return chooseMoveTarget(m)
		}
	}
	return m, nil
}
