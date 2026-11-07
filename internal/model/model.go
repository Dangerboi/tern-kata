package model

import (
	"fmt"
	"time"
)

// Issue is an Issue as the Pane shows it. The gateway translates it from kata's wire shape.
type Issue struct {
	AssignmentExpiresOn *time.Time
	// Attribution is kata's creation attribution of the Issue.
	Attribution  Attribution
	Author       string
	Body         string
	ClosedAt     *time.Time
	ClosedReason *string
	CreatedAt    time.Time
	// CronOccurrence is set on an Issue a Cron job made.
	CronOccurrence  *CronOccurrence
	DeadlineOnDate  *string
	ID              int64
	Labels          []string
	Metadata        map[string]any
	Owner           *string
	Priority        *int64
	ProjectID       int64
	ProjectName     string
	ProjectUID      *string
	QualifiedID     string
	RecurrenceID    *int64
	Revision        int64
	ScheduledOnDate *string
	ShortID         string
	Status          string
	Title           string
	UID             string
	UpdatedAt       time.Time
}

// Qualified is the Issue as kata names it across Projects, project#short id: its own qualified id when set, else built from
// its Project and short id; empty when it has neither.
func (i Issue) Qualified() string {
	if i.QualifiedID != "" || i.ProjectName == "" || i.ShortID == "" {
		return i.QualifiedID
	}
	return i.ProjectName + "#" + i.ShortID
}

// IsClosed says the Issue is closed; any other status counts as open.
func IsClosed(issue Issue) bool { return issue.Status == "closed" }

// Project is a Project as the Pane holds it. The gateway translates it from kata's wire shape.
type Project struct {
	ID       int64
	Name     string
	Revision int64
	UID      string
	// Archived says kata has archived the Project: Issues cannot move into it.
	Archived bool
	// Inbox says the Project's metadata role is "inbox": the Project capture lands in, whose Issues the Inbox Collection lists.
	Inbox bool
}

type Model struct {
	// Connection is the number of the daemon connection the Pane holds, stepped by each successful switch. Answers to ProjectLoaded, ClearAttention and token creation echo the number they were asked on, and an older one is dropped.
	Connection int
	// IssueScoped says the connection's credential is limited to one Issue, as the daemon's auth capabilities report. kata refuses
	// delete and restore to such an account, so the Pane does not offer them. It stays false until the answer arrives, and when the read fails.
	IssueScoped bool
	// accountReads numbers the asks for the connection's own capabilities; the Credentials Page numbers its own.
	accountReads Latest
	// accountAnswered says a read of them succeeded on this connection, so the stream opening again asks no more.
	accountAnswered bool
	LiveState
	IssueDetail
	Create CreateState
	// Agents is the latest agent state list the window half relayed.
	Agents AgentStatesRelay
	// AgentClose is what ending finished Background agents remembers.
	AgentClose agentCloseState
	// pending is the Issue Create just made, selected as soon as a snapshot lists it.
	pending string
	// etag answers etagQuery: the snapshot on screen.
	etag         string
	etagQuery    SnapshotQuery
	CloseForm    CloseFormState
	DatesForm    DatesFormState
	DateCalendar CalendarState
	DeleteForm   DeleteFormState
	TokenForm    TokenFormState
	RevokeCard   RevokeState
	// AgentCard is the Agent card, which lists an Issue's agents.
	AgentCard AgentCardState
	// AgentSessions is what the Agent card read of the agents' session files.
	AgentSessions  agentSessionState
	RecurrenceForm RecurrenceFormState
	JobForm        JobFormState
	// Backfill is the card that asks whether the runs a native job missed are created before the job runs again.
	Backfill BackfillState
	// Now is the instant the Pane driver last stepped the model at: the model reads no clock, so what it counts against the
	// time, as the runs a job missed, counts against this.
	Now   time.Time
	Actor string
	// InboxUser is KATA_INBOX_USER as read at launch: the Inbox address when not blank, else the actor is.
	InboxUser string
	// Host is this machine's host name, which tells an Attachment held here from one held elsewhere.
	Host      string
	FieldForm FieldFormState
	// Notice reports a write that has nothing to show in the Issues, and goes at the next key.
	Notice string
	// NoticeLink is the part of the Notice a click opens as an Issue; it goes with the Notice.
	NoticeLink *NoticeLink
	// MoveProject is the Move card, which moves the selected Issue to another Project.
	MoveProject MoveProjectState
	// afterMove is the Issue just moved to another Project and its neighbouring row, until a snapshot stops listing it.
	afterMove *movedIssue
	Workspace string
	// PaneBlock is the block id of this kata Pane ($TERN_PANE), "" when Tern did not say; agents split beside it.
	PaneBlock string
	// AgentMapping is agents.json as this Pane last read or changed it: the agents Panes on this host started.
	AgentMapping []AgentEntry
	// AgentAttention are the Attention requests this Pane raised for waiting agents and has not cleared.
	AgentAttention []RaisedAttention
	// AgentGone are the entries of agents this Pane started whose block went (pruned from agents.json) without a relay showing them
	// exited, until the exit request for their Issue is decided.
	AgentGone []AgentEntry
	// AgentMessage is the Message card, which writes a message to an agent from the Attention Page.
	AgentMessage AgentMessageState
	// addressesCleared are the blocks of this Pane's exited agents whose Agent address the Pane has cleared, while they are mapped or gone.
	addressesCleared []string
	// agentFresh are the agents started since a request was asked, by block, with the number of the last request asked before the
	// start: an answer that read the file before the start wrote the entry does not hold it, and that is no sign the agent is gone.
	// Only an answer to a later request settles it.
	agentFresh map[string]int64
	// agentAsks counts the RefreshAgents and PruneAgents the Pane has asked for.
	agentAsks int64
	Project   Project
	Daemon    Daemon
	Snapshot  Snapshot
	// Relationships is every Issue's Relationships, built from Snapshot's links each time a snapshot is applied.
	Relationships RelationshipIndex
	Issues        []Issue
	SelectedUID   string
	Loading       bool
	Err           error
	Cols          int
	Rows          int
	Quitting      bool
	// AskQuit is the question "Quit tern-kata? y/n" while it is on screen.
	AskQuit bool
	// QuitAtOnce is kata's [tui].confirm_quit = false, read at launch: q quits without asking.
	QuitAtOnce       bool
	Collection       Collection
	Collections      []Collection
	CollectionCursor int
	SidebarLevel     MenuLevel
	Bound            bool
	Picker           PickerState
	// Views is views.toml as the Pane last read it; ViewForm is the View card; ActiveView is the saved view this Pane opened.
	Views       ViewsState
	ViewForm    ViewFormState
	ActiveView  ActiveView
	ProjectForm ProjectFormState
	LanesForm   LanesFormState
	WorkerCard  WorkerCardState
	// Dropdown is the open list of a popup card's select, if one is open.
	Dropdown     Dropdown
	Focus        Column
	Groups       []IssueGroup
	Calendar     PaneCalendar
	HelpOpen     bool
	Flat         bool
	GraphOrder   bool // nested siblings in blocking order, not creation order; Pane state, never saved
	ExpandedUIDs map[string]bool
	Drafts       map[string]Draft
	EditorUID    string
	Filters      Filters
	// DefaultFilters is the bound Project's saved default filter, empty when it has none.
	DefaultFilters Filters
	Search         SearchState
	// PageSearch is the search of the Jobs, Runs or Workflows Page that is current.
	PageSearch PageSearchState
	IDSearch   IDSearch
	FilterForm FilterFormState
	Page       Page
	// pageDetailHidden is the detail column's hidden flag from before the current Page took its place; leavePage restores it.
	pageDetailHidden bool
	Board            BoardState
	Jobs             JobsState
	Runs             RunsState
	// Credentials is the Credentials Page's state; letteredFrom is the Page a Lettered Page was opened from, which its back key returns to.
	Credentials  CredentialsState
	Federation   FederationState
	Daemons      DaemonsState
	letteredFrom Page
	// Worker is what the Pane knows of this machine's worker: whether it runs, what it serves and the local state of its runs.
	Worker    WorkerState
	Workflows WorkflowsState
	// Attention is the Attention Page's state, which no Project or Collection change resets.
	Attention AttentionState
	// Moving is the Issue whose Move is in flight; further Moves wait for its result.
	Moving string
	// Undo is the history of changes `u` takes back.
	Undo UndoHistory
	// nonce is the random per-run prefix of every idempotency key and keys counts the keys minted.
	nonce string
	keys  int
	// collection keeps the rows and columns worked out from Issues and the snapshot; see collectionRows.
	collection collectionRows
}

func Update(m Model, msg Msg) (Model, []Cmd) {
	if staleConnection(m, msg) {
		return m, nil
	}
	if press, ok := msg.(ButtonPressMsg); ok {
		m.DateCalendar, m.Dropdown = CalendarState{}, Dropdown{}
		msg = KeyMsg{Chord: press.Chord}
	}
	if _, ok := msg.(KeyMsg); ok {
		m.Notice, m.NoticeLink = "", nil
	}
	next, cmds := updateWithDetail(m, msg)
	switch msg.(type) {
	case SnapshotLoaded, StreamEvent:
		var raised, ended []Cmd
		next, raised = attentionOnIssues(next)
		next, ended = closeFinishedAgents(next, closeOnIssues)
		cmds = append(append(cmds, raised...), ended...)
	}
	next = next.recordWrite(msg).noteWrites(cmds)
	return next.cancelPending(m, msg).settleDropdown().settleCalendar().withCollectionRows(), cmds
}

// staleConnection reports an answer asked of a daemon the Pane has since switched away from: it changes nothing.
func staleConnection(m Model, msg Msg) bool {
	switch msg := msg.(type) {
	case ProjectLoaded:
		return msg.Connection != m.Connection
	case AttentionCleared:
		return msg.Connection != m.Connection
	case TokenCreated:
		return msg.Connection != m.Connection
	}
	return false
}

func update(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case FocusFieldMsg:
		return focusField(m, msg.Field), nil
	case CalendarDayMsg:
		if entry, ok := m.topEntry(); ok && calendarOpen(m, entry) {
			return pickDay(m, msg.Day), nil
		}
		return m, nil
	case ChooseMsg:
		return chooseField(m, msg.Field, msg.Index), nil
	case MoveSingleMsg:
		return setMoveSingle(m, msg.On)
	case SwitchMsg:
		return switchField(m, msg.Field, msg.On), nil
	case DropdownToggleMsg:
		return toggleDropdown(m, msg.Field, msg.Open), nil
	case KeyMsg:
		if next, cmds, handled := updateTopCardKey(m, msg); handled {
			return next, cmds
		}
	case EditMsg:
		if next, handled := editBody(m, msg.Target, func(e TextEditor) TextEditor {
			return e.ApplyEdit(msg.From, msg.To, msg.Text, msg.Cursor, msg.Len)
		}); handled {
			return next, nil
		}
	case UndoMsg:
		if next, handled := editBody(m, msg.Target, TextEditor.Undo); handled {
			return next, nil
		}
	}
	if next, cmds, handled := updateCreate(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updatePages(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := workerEditorClosedMsg(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateEditor(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateProjectForm(m, msg); handled {
		return next, cmds
	}
	switch msg := msg.(type) {
	case CloseFieldMsg, CloseResultMsg:
		return updateClose(m, msg)
	case RecurrenceResultMsg:
		return updateRecurrence(m, msg)
	case DatesResultMsg:
		return updateDates(m, msg)
	case IssueDeleted:
		return updateDelete(m, msg)
	case DeletedLooked:
		return deletedLooked(m, msg), nil
	case IssueRestored:
		return issueRestored(m, msg)
	case ProjectMoveAnswer:
		return updateMoveProject(m, msg)
	case AgentCardSelected:
		return agentCardSelected(m, msg), nil
	case MoveTargetSelected:
		return moveTargetSelected(m, msg.Key)
	case OpenNoticeLinkMsg:
		return openNoticeLink(m)
	}
	if msg, ok := msg.(PickerSelected); ok {
		return pickCollectionKey(m, msg.Key)
	}
	switch msg := msg.(type) {
	case FieldValueMsg:
		if m.FieldForm.Open && !m.FieldForm.Saving {
			m.FieldForm.Value = msg.Value
			m.FieldForm.Err = nil
			m.FieldForm.Suggestion = 0
			m.FieldForm.Found = nil
		}
	case FieldLabelsLoaded:
		if m.FieldForm.Open && m.FieldForm.UID == msg.UID {
			m.FieldForm.Labels = msg.Labels
			m.FieldForm.Err = msg.Err
		}
	case MoveResultMsg:
		return updateMove(m, msg)
	case ChangeUndone:
		return undoDone(m, msg)
	case FieldWritten:
		return fieldWritten(m, msg.UID, "", msg.Err)
	case FileAttached:
		report := ""
		if issue := m.issueByUID(msg.UID); issue != nil {
			report = fmt.Sprintf("Attached %s to %s", msg.Name, issue.ShortID)
		}
		return fieldWritten(m, msg.UID, report, msg.Err)
	case AttachmentUnlinked:
		report := ""
		if issue := m.issueByUID(msg.UID); issue != nil {
			report = fmt.Sprintf("Unlinked %s from %s; its bytes stay in Docbank", msg.Name, issue.ShortID)
		}
		return fieldWritten(m, msg.UID, report, msg.Err)
	case AttachmentOpened:
		if msg.Err != nil {
			m.Notice = fmt.Sprintf("Cannot open %s: %v", msg.Name, msg.Err)
		} else {
			m.Notice = fmt.Sprintf("Opened %s (%s)", msg.Name, msg.Path)
		}
		return m, nil
	case Start:
		m.Workspace = msg.Workspace
		m.nonce = msg.Nonce
		m = m.openRequested(msg.IssueRef)
		m.Loading = true
		m.Err = nil
		m.agentAsks++
		account := askAccount(&m)
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}, LoadViews{}, PruneAgents{PaneBlock: m.PaneBlock, Seq: m.agentAsks}, account}
	case ProjectLoaded:
		return updateProject(m, msg)
	case AgentStarted:
		return restartAgentSessions(agentStarted(m, msg), nil)
	case AgentsPruned:
		return agentsPruned(m, msg)
	case FiltersSaved:
		return filtersSaved(m, msg)
	case ViewsLoaded:
		return viewsLoaded(m, msg), nil
	case ViewSaved:
		return viewSaved(m, msg)
	case ViewDeleted:
		return viewDeleted(m, msg)
	case WorkerLoaded:
		return workerLoaded(m, msg)
	case WorkerSetupLoaded:
		m = workerSetupLoaded(m, msg)
	case WorkerSetupSaved:
		return workerSetupSaved(m, msg)
	case WorkerEdited:
		return workerEdited(m, msg)
	case WorkerStopped:
		return workerStopped(m, msg)
	case WorkerSupervisionSet:
		return workerSupervisionSet(m, msg)
	case LocalRunLoaded:
		m = localRunLoaded(m, msg)
	case JobRunRequested:
		return jobRunRequested(m, msg)
	case JobResumed:
		return jobResumed(m, msg)
	case LocalRunMoved:
		return localRunMoved(m, msg)
	case DetailLoaded:
		m = m.detailLoaded(msg)
	case DetailFailed:
		m = m.detailFailed(msg)
	case JumpIssueMsg:
		return m.jumpTo(msg.UID, nil)
	case ShowCardMsg:
		return showCard(m, msg.UID)
	case FocusColumnMsg:
		if msg.Column <= ColumnDetail {
			m.Focus = msg.Column
			if m.Focus == ColumnDetail && !m.OnJobs() && !m.OnRuns() {
				m = m.focusDetail()
			}
		}
	case ActivityTabMsg:
		if msg.Tab < TabCount {
			m = m.chooseTab(msg.Tab)
		}
	case DetailItemMsg:
		if msg.Tab < TabCount {
			m = m.chooseRow(msg.Tab, msg.Key)
		}
	case CollectionSelected:
		return updateCollection(m, msg)
	case CollectionPointerSelected:
		for _, entry := range m.SidebarEntries() {
			if msg.Key == entry.Key() {
				var reload []Cmd
				if m.Focus != ColumnCollections { // the click enters the Menu as esc does
					reload = reloadViews()
				}
				if entry.Kind == EntryProjects || entry.Kind == EntryViews || entry.Kind == EntryBack {
					m.Focus = ColumnCollections
				}
				m, cmds := openEntry(m, entry, false)
				return m, append(reload, cmds...)
			}
		}
	case Failed:
		m.Loading = false
		m.Err = msg.Err
	case AgentStatesMsg:
		m, cmds := relayAgents(m, msg)
		m = trackSweep(m)
		m, back := sendAgentsBack(m)
		m, flagged := followAgents(m, m.Agents.listedStates())
		m, raised := raiseAgentAttention(m, true)
		m, ended := closeFinishedAgents(m, closeOnRelay)
		return m, append(append(append(append(cmds, back...), flagged...), raised...), ended...)
	case AgentSessionsRead:
		return agentSessionsRead(m, msg)
	case AgentSessionTick:
		return agentSessionTick(m, msg)
	case AgentHandedOff:
		m = agentHandedOff(m, msg)
	case AgentIssueRead:
		return agentIssueRead(m, msg)
	case AgentClosed:
		m = agentClosed(m, msg)
	case AgentStopped:
		return agentStopped(m, msg)
	case AgentInterrupted:
		m = agentInterrupted(m, msg)
	case AgentAttentionWritten:
		return agentAttentionWritten(m, msg), nil
	case AgentAddressCleared:
		return agentAddressCleared(m, msg), nil
	case AgentJumpMsg:
		return jumpToAgent(m, msg.Block)
	case AgentPlaced:
		m = agentPlaced(m, msg)
	case AgentOpenSet:
		m = agentOpenSet(m, msg)
	case AgentFocused:
		m = agentFocused(m, msg)
	case AgentsRefreshed:
		m, cmds := agentsRefreshed(m, msg)
		return restartAgentSessions(m, cmds)
	case ActionMsg:
		switch msg.Action {
		case SelectIssueAction:
			return openIssue(m, msg.Value)
		case WrapUpSweepAction:
			return sweepAgents(m)
		case ToggleExpansion:
			return toggleRow(m, msg.Value)
		case OpenRow:
			return openRow(m, msg.Value)
		case OpenPicker:
			return openPicker(m), reloadViews()
		}
	case IssueOpened:
		return m.openByReference(msg.Ref, msg.Selected)
	case IssueOpenFailed:
		if msg.Ref == m.OpenIssueRef() {
			m = m.openFailed(msg.Ref)
			m.Err = msg.Err
		}
	case KeyMsg:
		mode := m.CurrentMode()
		if binding, ok := LookupBindingForMode(msg.Chord, mode); ok && binding.entry != nil {
			return binding.run(m)
		}
		if inputMode(mode) {
			return updateFilterText(m, msg)
		}
	case SelectMsg:
		return selectRow(m, msg.UID), nil
	case ContextMenuPickMsg:
		selected := selectRow(m, msg.UID)
		if selected.SelectedUID != msg.UID {
			return m, nil
		}
		selected.Notice, selected.NoticeLink = "", nil
		if binding, ok := LookupBindingForMode(msg.Chord, msg.Mode); ok && binding.entry != nil {
			return binding.run(selected)
		}
		return selected, nil
	case ResizeMsg:
		m.Cols, m.Rows = msg.Cols, msg.Rows
	case PaneRowsMsg:
		m.Rows = msg.Rows
	default:
		return updateLive(m, msg)
	}
	return m, nil
}

func workerEditorClosedMsg(m Model, msg Msg) (Model, []Cmd, bool) {
	if closed, ok := msg.(DraftEditorClosed); ok {
		return workerEditorClosed(m, closed)
	}
	return m, nil, false
}
