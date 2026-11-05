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
// names the Project and not the id. kata moves an Issue with its descendants unless the card's "Only this Issue" switch is on,
// and the preview names the descendants that move. (The Lane Move of a card is a different thing: see Move in the glossary.)

const (
	ModeMoveProject Mode = "move-project"
	// ModeMoveConfirm is the card after a Project is chosen: previewing, previewed, or showing a conflict.
	ModeMoveConfirm Mode = "move-confirm"

	OpenMoveProject   Action = "open-move-project"
	MoveProjectChoose Action = "move-project-choose"
	MoveProjectSend   Action = "move-project-send"
	MoveProjectBack   Action = "move-project-back"
	MoveProjectClose  Action = "move-project-close"
	// MoveProjectSingle toggles "Only this Issue" on the card that previews a move.
	MoveProjectSingle Action = "move-project-single"
)

// moveListLimit is how many descendants the preview names before it counts the rest.
const moveListLimit = 8

// MoveToProjectCmd is one request to kata's move action. A DryRun changes nothing; Revision is the revision the card opened
// with, sent as If-Match, and zero for the fresh preview that follows a conflict. Single moves the Issue alone; without it kata
// moves the Issue with its descendants.
type MoveToProjectCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	ToProjectUID          string
	DryRun                bool
	Single                bool
	Revision              int64
}

// MovedIssue is a descendant that moves with the Issue: its short id and the uid of the Project it lives in now.
type MovedIssue struct{ ShortID, ProjectUID string }

func (MoveToProjectCmd) cmd() {}

// ProjectMoveAnswer is the one answer to a MoveToProjectCmd. Revision is the Issue's revision kata reported; NewShortID is set
// only by a move that happened; Conflict says kata refused the revision as stale, with Err nil. Count is how many Issues move or
// moved, the Issue included, and Descendants are the others of them. Subtree says Err is a refusal that comes from the
// descendants, which "Only this Issue" or a new preview may get past, so the card keeps its Project.
type ProjectMoveAnswer struct {
	UID, ToProjectUID string
	DryRun            bool
	Revision          int64
	NewShortID        string
	Count             int
	Descendants       []MovedIssue
	Conflict          bool
	Subtree           bool
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
	// Single is the "Only this Issue" switch: move the Issue without its descendants.
	Single bool
	// Previewed says kata accepted the dry run for Target, so Enter moves.
	Previewed bool
	// Descendants are the Issues below this one that the accepted preview says move with it.
	Descendants []MovedIssue
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
	case f.Previewed && len(f.Descendants) == 0:
		return fmt.Sprintf("#%s moves to %s and gets its new id there.", f.ShortID, f.Target.Name)
	case f.Previewed:
		return fmt.Sprintf("#%s and %s move to %s and get new ids there: %s.", f.ShortID, descendantCount(len(f.Descendants)), f.Target.Name, m.descendantRefs())
	}
	return ""
}

// descendantCount is "1 descendant" or "N descendants".
func descendantCount(n int) string {
	if n == 1 {
		return "1 descendant"
	}
	return fmt.Sprintf("%d descendants", n)
}

// descendantRefs names the previewed descendants by short id, qualified by Project name when one lives in another Project than
// the Issue, and counts the ones past moveListLimit.
func (m Model) descendantRefs() string {
	f := m.MoveProject
	var source string
	names := map[string]string{}
	for _, entry := range m.Snapshot.Catalog {
		names[entry.Project.UID] = entry.Project.Name
		if entry.Project.ID == f.ProjectID {
			source = entry.Project.UID
		}
	}
	var refs []string
	for _, d := range f.Descendants[:min(len(f.Descendants), moveListLimit)] {
		ref := "#" + d.ShortID
		if name := names[d.ProjectUID]; d.ProjectUID != "" && d.ProjectUID != source && name != "" {
			ref = name + ref
		}
		refs = append(refs, ref)
	}
	if more := len(f.Descendants) - len(refs); more > 0 {
		refs = append(refs, fmt.Sprintf("and %d more", more))
	}
	return strings.Join(refs, ", ")
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
	cmd := MoveToProjectCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, ToProjectUID: f.Target.UID, DryRun: dryRun, Single: f.Single, Revision: f.Revision}
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

// confirmMove is Enter after a Project is chosen: move when previewed, preview again after a conflict or a refusal that
// comes from the subtree.
func confirmMove(m Model) (Model, []Cmd) {
	if m.MoveProject.Previewed {
		return sendMoveProject(m, false)
	}
	return sendMoveProject(m, true)
}

// MoveSingleMsg is the pointer flipping the card's "Only this Issue" switch to On.
type MoveSingleMsg struct{ On bool }

func (MoveSingleMsg) msg() {}

// setMoveSingle sets "Only this Issue" on the card that has a Project chosen and asks kata for a new preview, since the
// Issues that move change with it. It does nothing while a request is out.
func setMoveSingle(m Model, on bool) (Model, []Cmd) {
	f := &m.MoveProject
	if !f.Open || f.Sending || f.Target.UID == "" || f.Single == on {
		return m, nil
	}
	f.Single, f.Previewed, f.Descendants = on, false, nil
	return sendMoveProject(m, true)
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
	case msg.Err != nil && msg.Subtree:
		f.Previewed, f.Descendants, f.Conflict, f.Err = false, nil, false, msg.Err
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
		f.Previewed, f.Descendants, f.Conflict, f.Err = true, msg.Descendants, false, nil
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
	moved := "#" + f.ShortID
	if msg.Count > 1 {
		moved = fmt.Sprintf("#%s and %s", f.ShortID, descendantCount(msg.Count-1))
	}
	m.Notice, m.NoticeLink = fmt.Sprintf("Moved %s to %s.", moved, link.Text), &link
	m = issueGone(m, f.UID)
	return snapshotChanged(m.issueWritten(f.UID))
}

// issueGone is an Issue the Collection stops listing, moved away or deleted: the selection goes to the neighbouring row once a
// snapshot no longer lists it, and its detail is left.
func issueGone(m Model, uid string) Model {
	m.afterMove = &movedIssue{UID: uid, Next: m.neighbourRow(uid)}
	if m.OpenedIssueUID() == uid || m.uid == uid {
		m.Focus = ColumnList
		m.openedUID = ""
	}
	return m
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
	case MoveProjectSingle:
		return setMoveSingle(m, !f.Single)
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
