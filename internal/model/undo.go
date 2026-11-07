package model

import (
	"fmt"
	"slices"
	"strings"
)

// The Undo history holds the last changes this Pane made to Issues, newest last, so `u` can take the newest back. It lives in
// memory for the Pane's connection. Every change the Pane makes to an Issue either has an inverse here and enters the history
// when it completes, or is a boundary that clears it with its reason; a write whose Cmd is in neither list is a boundary, so a
// write added later never lets `u` reach past it unless it is given an inverse.

// UndoLimit is how many completed changes the history holds.
const UndoLimit = 20

const UndoAction Action = "undo"

// UndoKind says which change an entry takes back.
type UndoKind string

const (
	UndoClose       UndoKind = "close"
	UndoReopen      UndoKind = "reopen"
	UndoOwner       UndoKind = "owner.assign"
	UndoPriority    UndoKind = "priority.set"
	UndoBody        UndoKind = "body.edit"
	UndoLabelAdd    UndoKind = "label.add"
	UndoLabelRemove UndoKind = "label.remove"
	UndoLinkAdd     UndoKind = "link.add"
	UndoLaneMove    UndoKind = "lane.move"
	UndoDates       UndoKind = "dates.set"
	UndoDelete      UndoKind = "delete"
)

// IssueState is the part of an Issue an undo compares and restores.
type IssueState struct {
	Status       string
	ClosedReason *string
	Owner        *string
	Priority     *int64
	Body         string
	Revision     int64
}

// UndoLink is the link a link change created.
type UndoLink struct {
	ID                   int64
	Type, FromUID, ToUID string
}

// UndoEntry is one completed change, with what the gateway needs to take it back and to refuse when the world moved on.
// Principal is the account the daemon said the change ran as, kept as the daemon wrote it.
type UndoEntry struct {
	ID                       uint64
	Kind                     UndoKind
	UID, ProjectUID, ShortID string
	ProjectID                int64
	Actor                    string
	InstanceUID, Principal   string
	Before, After            IssueState
	// Revision is the Issue revision the change produced.
	Revision int64
	Label    string
	Link     *UndoLink
	// Added and Removed are the labels a Lane Move added and removed.
	Added, Removed []string
	// Labels is the Issue's full label set right after a Lane Move, nil when not recorded.
	Labels []string
	// Dates are the date keys a Dates card change wrote, as written.
	Dates []DateChange
}

// WriteRecord is what a write to an Issue says about the Undo history: the entry it completed, the reason it cannot be undone,
// or that its outcome is unknown.
type WriteRecord struct {
	Entry    *UndoEntry
	Boundary string
	Unknown  bool
}

// UndoHistory is the Pane's Undo history.
type UndoHistory struct {
	Entries []UndoEntry
	// Boundary is why the history was last cleared, "" when it never was.
	Boundary string
	// InFlight says an undo is out; another waits for it.
	InFlight bool
	// out is the entry the undo in flight takes back, kept for when the history no longer holds it at the answer.
	out    UndoEntry
	nextID uint64
}

// entry finds the entry with id: in the history wherever it sits, else the one the undo in flight takes back, which a cleared
// history no longer holds.
func (h UndoHistory) entry(id uint64) (UndoEntry, bool) {
	if id == 0 {
		return UndoEntry{}, false
	}
	if i := slices.IndexFunc(h.Entries, func(e UndoEntry) bool { return e.ID == id }); i >= 0 {
		return h.Entries[i], true
	}
	return h.out, h.out.ID == id
}

func (h UndoHistory) cleared(reason string) UndoHistory {
	h.Entries = nil
	h.Boundary = reason
	return h
}

func (h UndoHistory) pushed(e UndoEntry) UndoHistory {
	h.nextID++
	e.ID = h.nextID
	entries := append(slices.Clone(h.Entries), e)
	if len(entries) > UndoLimit {
		entries = entries[len(entries)-UndoLimit:]
	}
	h.Entries = entries
	return h
}

// without drops the entry with id, wherever it sits: another change may have landed on top of it while its undo was out.
func (h UndoHistory) without(id uint64) UndoHistory {
	h.Entries = slices.DeleteFunc(slices.Clone(h.Entries), func(e UndoEntry) bool { return e.ID == id })
	return h
}

// UndoChange asks the Daemon gateway to take back Entry. Close is the evidence-bearing close the Close card collected when kata
// requires evidence to close.
type UndoChange struct {
	Workspace string
	Entry     UndoEntry
	Close     *CloseIssueCmd
}

func (UndoChange) cmd() {}

// UndoResult is how an undo ended.
type UndoResult int

const (
	UndoChanged UndoResult = iota
	UndoAlready
	UndoNeedsEvidence
	UndoConflict
	UndoFailed
	UndoUnknown
)

// ChangeUndone answers UndoChange. Conflict is the refusal's reason, Ref the Issue as the daemon names it, and FromRevision and
// ToRevision the Issue's revision before and after the inverse was written.
type ChangeUndone struct {
	EntryID                  uint64
	Result                   UndoResult
	Conflict, Ref            string
	FromRevision, ToRevision int64
	Err                      error
}

func (ChangeUndone) msg() {}

// Boundaries: the reasons the history is cleared. A Cmd that is neither a read nor in these lists clears with
// undoUnknownBoundary.
const (
	undoUnknownBoundary = "an Issue change cannot be undone"
	undoUnknownOutcome  = "previous action's outcome is unknown"
)

// undoBoundary reports whether emitting cmd clears the Undo history, and why. Reads and local writes leave it alone; the writes
// with an inverse enter it when they complete; every other Cmd clears it, so a write added later does so by default.
func undoBoundary(cmd Cmd) (string, bool) {
	switch cmd := cmd.(type) {
	case Scroll, RevealActivityTab, OpenDraftEditor, LoadDetail, LoadLanes, SaveLanes, ShareLanes, SaveFilters, SaveAgentStates, LoadViews, SaveView, DeleteView, LoadWorkerPickers,
		LoadJobWorkflows, LoadJobs, LoadRuns, ShowRun, LoadRecurTarget, LoadFieldLabels, LoadProject, ReadSnapshot, Subscribe,
		WakeAt, OpenIssue, LookupSearchID, LoadWorkflows, ReadWorkflowText, LoadWorker, LoadWorkerSetup, SaveWorkerSetup,
		SaveWorkerEdit, StopWorker, SetWorkerSupervision, LoadLocalRun, RunJobNow, ResumeJob, ResumeLocalRun, AbandonLocalRun,
		OpenRunSession, StartAgent, PruneAgents, RefreshAgents, ReadAgentSessions, WaitAgentSessions, FocusAgent, OpenAgent, SendAgentBack, SetAgentOpen, CloseAgent, StopAgent, HandOffAgent, InterruptAgent, ReadAgentIssue, RaiseAgentAttention, ClearAgentAttention, ClearAgentAddress, CreateProject, BindProject, LookupDeleted, LoadDaemons, SwitchDaemon, LoadAttention, LoadAuthCapabilities, OpenAttachment,
		LoadCredentials, LoadFederation, LoadFederationHubs, LoadFederationHubProjects, BrowseFederationHubs:
		return "", false
	case CloseIssueCmd, ReopenIssueCmd, MoveIssueCmd, SetDatesCmd, DeleteIssueCmd, UndoChange:
		return "", false
	case RestoreIssueCmd:
		return "restoring an Issue cannot be undone", true
	case MoveToProjectCmd:
		if cmd.DryRun {
			return "", false
		}
		return "moving an Issue cannot be undone", true
	case WriteField:
		if cmd.Action == TimedAssignAction {
			return "timed assignment cannot be undone", true
		}
		return "", false
	case SaveDraft:
		if cmd.Comment {
			return "comment addition cannot be undone", true
		}
		return "", false
	case CreateIssueCmd:
		return "issue creation cannot be undone", true
	case AttachFile:
		return "attaching a file cannot be undone", true
	case MigrateAttachments:
		return "moving old attachments cannot be undone", true
	case UnlinkAttachment:
		return "unlinking an attachment cannot be undone", true
	case CreateRecurrenceCmd:
		return "making a recurrence cannot be undone", true
	case ChangeRecurrenceCmd:
		return "changing a recurrence cannot be undone", true
	case StopRecurrenceCmd:
		return "stopping a recurrence cannot be undone", true
	case SaveJobCmd, ArchiveJobCmd, RestoreJobCmd:
		return "a Cron job change cannot be undone", true
	case SaveWorkflow, ArchiveWorkflow, RestoreWorkflow:
		return "a Workflow change cannot be undone", true
	}
	return undoUnknownBoundary, true
}

// noteWrites clears the Undo history for each Cmd that is a boundary.
func (m Model) noteWrites(cmds []Cmd) Model {
	for _, cmd := range cmds {
		if reason, clears := undoBoundary(cmd); clears {
			m.Undo = m.Undo.cleared(reason)
		}
	}
	return m
}

// recordWrite enters the change a write answer reports into the Undo history, or clears it.
func (m Model) recordWrite(msg Msg) Model {
	var record WriteRecord
	switch msg := msg.(type) {
	case FieldWritten:
		record = msg.Record
	case CloseResultMsg:
		record = msg.Record
	case MoveResultMsg:
		record = msg.Record
	case DatesResultMsg:
		record = msg.Record
	case DraftSaved:
		record = msg.Record
	case DraftFailed:
		record = msg.Record
	case IssueDeleted:
		record = msg.Record
	default:
		return m
	}
	switch {
	case record.Unknown:
		m.Undo = m.Undo.cleared(undoUnknownOutcome)
	case record.Boundary != "":
		m.Undo = m.Undo.cleared(record.Boundary)
	case record.Entry != nil:
		m.Undo = m.Undo.pushed(*record.Entry)
	}
	return m
}

// startUndo is `u`.
func startUndo(m Model) (Model, []Cmd) {
	h := m.Undo
	switch {
	case h.InFlight:
		m.Notice = "undo already in progress"
	case len(h.Entries) == 0 && h.Boundary != "":
		m.Notice = "nothing to undo: " + h.Boundary
	case len(h.Entries) == 0:
		m.Notice = "nothing to undo"
	case m.Stale:
		m.Notice = "Daemon disconnected; reconnect before undoing."
	default:
		return m.sendUndo(h.Entries[len(h.Entries)-1], nil)
	}
	return m, nil
}

// newestUndoIsDelete says the newest Undo entry is a delete: the one case in which the Attention Page offers `u`.
func newestUndoIsDelete(m Model) bool {
	n := len(m.Undo.Entries)
	return n > 0 && m.Undo.Entries[n-1].Kind == UndoDelete
}

// sendUndo marks the undo in flight for entry and sends it, with the close the Close card collected when the inverse is a close.
func (m Model) sendUndo(entry UndoEntry, closeCmd *CloseIssueCmd) (Model, []Cmd) {
	m.Undo.InFlight = true
	m.Undo.out = entry
	return m, []Cmd{UndoChange{Workspace: m.Workspace, Entry: entry, Close: closeCmd}}
}

// undoTarget names the Issue of an entry the way the Pane shows it: project#id when the Pane lists it, else #id.
func (m Model) undoTarget(e UndoEntry, ref string) string {
	if ref == "" {
		ref = e.ShortID
	}
	if ref == "" {
		return e.UID
	}
	if strings.Contains(ref, "#") {
		return ref
	}
	if issue := m.issueByUID(e.UID); issue != nil && issue.ProjectName != "" && issue.ProjectName != m.Project.Name {
		return issue.ProjectName + "#" + ref
	}
	return "#" + ref
}

// openUndoClose opens the Close card for an evidence-requiring reopen's inverse: it completes the Issue as done.
func openUndoClose(m Model, e UndoEntry) Model {
	m.CloseForm = CloseFormState{Open: true, Field: CloseFields[1].Name, UID: e.UID, ProjectID: e.ProjectID, UndoEntry: e.ID}
	m.CloseForm.Values.Reason = string(ReasonDone)
	if issue := m.issueByUID(e.UID); issue != nil {
		m.CloseForm.Attachments = Attachments(issue.Metadata)
	}
	return m
}

// undoDone applies the answer to an undo, as kata tui does.
func undoDone(m Model, msg ChangeUndone) (Model, []Cmd) {
	m.Undo.InFlight = false
	card := msg.EntryID != 0 && m.CloseForm.Open && m.CloseForm.UndoEntry == msg.EntryID
	h := m.Undo
	// Other changes may have landed while the undo was out, so the entry is not necessarily the newest; a boundary write may
	// even have cleared it, and the answer is still shown and the Issue read again.
	entry, found := h.entry(msg.EntryID)
	if !found {
		if card {
			m.CloseForm.Saving = false
		}
		return m, nil
	}
	target := m.undoTarget(entry, msg.Ref)
	if card {
		m.CloseForm.Saving = false
		if msg.Result == UndoFailed {
			m.CloseForm.Err = fmt.Errorf("undo failed: %w", msg.Err)
			return m, nil
		}
		if msg.Result != UndoNeedsEvidence {
			m.CloseForm = CloseFormState{}
		}
	}
	if msg.Result == UndoConflict && (msg.Conflict == "daemon principal changed" || msg.Conflict == "daemon instance changed") {
		m.Undo = h.cleared(msg.Conflict)
		m.Notice = "cannot undo " + target + ": " + msg.Conflict
		return m, nil
	}
	switch msg.Result {
	case UndoNeedsEvidence:
		return openUndoClose(m, entry), nil
	case UndoConflict:
		m.Undo = h.without(entry.ID)
		m.Notice = "skipped undo for " + target + ": " + msg.Conflict
		return m, nil
	case UndoUnknown:
		m.Undo = h.cleared("undo outcome is unknown")
		m.Notice = "undo outcome unknown; refreshed issue state"
		return refreshAfterUndo(m, entry)
	case UndoFailed:
		m.Notice = "undo failed for " + target + ": " + msg.Err.Error()
		return m, nil
	case UndoAlready:
		m.Undo = h.without(entry.ID)
		m.Notice = target + " already restored; no change made"
		return refreshAfterUndo(m, entry)
	}
	m.Undo = h.without(entry.ID)
	if msg.FromRevision == entry.Revision {
		entries := slices.Clone(m.Undo.Entries)
		for i := range entries {
			older := &entries[i]
			if older.InstanceUID == entry.InstanceUID && older.ProjectID == entry.ProjectID && older.UID == entry.UID && older.Revision == entry.Before.Revision {
				older.Revision = msg.ToRevision
			}
		}
		m.Undo.Entries = entries
	}
	m.Notice = "undid " + string(entry.Kind) + " on " + target
	return refreshAfterUndo(m, entry)
}

// refreshAfterUndo reads the Issues an undo may have changed again.
func refreshAfterUndo(m Model, e UndoEntry) (Model, []Cmd) {
	m = m.issueWritten(e.UID)
	if e.Link != nil {
		m = m.issueWritten(e.Link.FromUID).issueWritten(e.Link.ToUID)
	}
	m, cmds := snapshotChanged(m)
	if m.Page == PageAttention {
		// A restored Issue can bring its requests back; the stream says so only while it is connected.
		var asked []Cmd
		m, asked = askAttention(m)
		cmds = append(cmds, asked...)
	}
	return m, cmds
}
