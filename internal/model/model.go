package model

import (
	"fmt"
	"time"
)

type Issue struct {
	AssignmentExpiresOn *time.Time
	Attribution         Attribution
	Author              string
	Body                string
	ClosedAt            *time.Time
	ClosedReason        *string
	CreatedAt           time.Time
	CronOccurrence      *CronOccurrence
	DeadlineOnDate      *string
	ID                  int64
	Labels              []string
	Metadata            map[string]any
	Owner               *string
	Priority            *int64
	ProjectID           int64
	ProjectName         string
	ProjectUID          *string
	QualifiedID         string
	RecurrenceID        *int64
	Revision            int64
	ScheduledOnDate     *string
	ShortID             string
	Status              string
	Title               string
	UID                 string
	UpdatedAt           time.Time
}

func (i Issue) Qualified() string {
	if i.QualifiedID != "" || i.ProjectName == "" || i.ShortID == "" {
		return i.QualifiedID
	}
	return i.ProjectName + "#" + i.ShortID
}

func IsClosed(issue Issue) bool { return issue.Status == "closed" }

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
	// VerifiedSenders says the daemon derives the actor of every write from the credential (kata's identity mode), as the Pane's own
	// capability read reports. A message from another actor or to another host is refused until it does; false until the answer arrives.
	VerifiedSenders bool
	accountReads    Latest
	// accountAnswered says a read of them succeeded on this connection, so the stream opening again asks no more.
	accountAnswered bool
	LiveState
	IssueDetail
	Create     CreateState
	Agents     AgentStatesRelay
	AgentClose agentCloseState
	// pending is the Issue Create just made, selected as soon as a snapshot lists it.
	pending        string
	etag           string
	etagQuery      SnapshotQuery
	CloseForm      CloseFormState
	DatesForm      DatesFormState
	DateCalendar   CalendarState
	DeleteForm     DeleteFormState
	TokenForm      TokenFormState
	RevokeCard     RevokeState
	AgentCard      AgentCardState
	AgentSessions  agentSessionState
	RecurrenceForm RecurrenceFormState
	JobForm        JobFormState
	Backfill       BackfillState
	// Now is the instant the Pane driver last stepped the model at: the model reads no clock, so what it counts against the
	// time, as the runs a job missed, counts against this.
	Now   time.Time
	Actor string
	// InboxUser is KATA_INBOX_USER as read at launch: the Inbox address when not blank, else the actor is.
	InboxUser string
	// Host is this machine's host name, which tells an Attachment held here from one held elsewhere.
	Host string
	// HostID is this host's Host id (tern-kata's state folder, ADR-0018): Agent records that carry another one are remote agents. "" when it could not be read, which lists none.
	HostID    string
	FieldForm FieldFormState
	// Notice reports a write that has nothing to show in the Issues, and goes at the next key.
	Notice      string
	NoticeLink  *NoticeLink
	MoveProject MoveProjectState
	// afterMove is the Issue just moved to another Project and its neighbouring row, until a snapshot stops listing it.
	afterMove *movedIssue
	Workspace string
	// PaneBlock is the block id of this kata Pane ($TERN_PANE), "" when Tern did not say; agents split beside it.
	PaneBlock string
	// AgentMapping is agents.json as this Pane last read or changed it: the agents Panes on this host started.
	AgentMapping []AgentEntry
	// Teams is teams.json as the last DriveTeams answered; teamDrive keeps one of them in flight.
	Teams     []Team
	teamDrive teamDriveState
	// AgentAttention are the Attention requests this Pane raised for waiting agents and has not cleared.
	AgentAttention []RaisedAttention
	// AgentGone are the entries of agents this Pane started whose block went (pruned from agents.json) without a relay showing them
	// exited, until the exit request for their Issue is decided.
	AgentGone    []AgentEntry
	AgentMessage AgentMessageState
	// goneAgentClears says, by block, how far the clear of an exited agent's Agent address and record has got: asked, or done (kept
	// while the agent is mapped or gone). A failed clear is dropped, so the next sweep asks again.
	goneAgentClears map[string]goneAgentClear
	// agentFresh are the agents started since a request was asked, by block, with the number of the last request asked before the
	// start: an answer that read the file before the start wrote the entry does not hold it, and that is no sign the agent is gone.
	// Only an answer to a later request settles it.
	agentFresh map[string]int64
	agentAsks  int64
	// agentsPruned says the first PruneAgents answer is in; sweepPending holds a wrap-up sweep asked for before it and the first
	// relay were both in.
	agentsPruned  bool
	sweepPending  bool
	Project       Project
	Daemon        Daemon
	Snapshot      Snapshot
	Relationships RelationshipIndex
	Issues        []Issue
	SelectedUID   string
	Loading       bool
	Err           error
	Cols          int
	Rows          int
	Quitting      bool
	AskQuit       bool
	// QuitAtOnce is kata's [tui].confirm_quit = false, read at launch: q quits without asking.
	QuitAtOnce       bool
	Collection       Collection
	Collections      []Collection
	CollectionCursor int
	SidebarLevel     MenuLevel
	Bound            bool
	Picker           PickerState
	Views            ViewsState
	ViewForm         ViewFormState
	ActiveView       ActiveView
	ProjectForm      ProjectFormState
	LanesForm        LanesFormState
	WorkerCard       WorkerCardState
	Dropdown         Dropdown
	Focus            Column
	Groups           []IssueGroup
	Calendar         PaneCalendar
	HelpOpen         bool
	Flat             bool
	GraphOrder       bool // nested siblings in blocking order, not creation order; Pane state, never saved
	ExpandedUIDs     map[string]bool
	Drafts           map[string]Draft
	EditorUID        string
	Filters          Filters
	DefaultFilters   Filters
	Search           SearchState
	PageSearch       PageSearchState
	IDSearch         IDSearch
	FilterForm       FilterFormState
	Page             Page
	// pageDetailHidden is the detail column's hidden flag from before the current Page took its place; leavePage restores it.
	pageDetailHidden bool
	Board            BoardState
	Jobs             JobsState
	Runs             RunsState
	// Credentials is the Credentials Page's state; letteredFrom is the Page a Lettered Page was opened from, which its back key returns to.
	Credentials  CredentialsState
	Federation   FederationState
	Daemons      DaemonsState
	ConfigPage   ConfigPageState
	letteredFrom Page
	// lanesWanted is the Lanes card a Config Page jump waits for: it opens once the Lane list read for the Board answers, so the
	// card starts on the source in use, and the wish ends when it opens or the Board is left.
	lanesWanted bool
	Worker      WorkerState
	Workflows   WorkflowsState
	// Attention is the Attention Page's state, which no Project or Collection change resets.
	Attention AttentionState
	// Moving is the Issue whose Move is in flight; further Moves wait for its result.
	Moving string
	Undo   UndoHistory
	// nonce is the random per-run prefix of every idempotency key and keys counts the keys minted.
	nonce      string
	keys       int
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
	if next, cmds, handled := workerEditorClosed(m, msg); handled {
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
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}, LoadViews{}, PruneAgents{Workspace: msg.Workspace, PaneBlock: m.PaneBlock, Seq: m.agentAsks}, account}
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
		m, tracked := trackSweep(m)
		m, back := sendAgentsBack(m)
		m, flagged := followAgents(m, m.Agents.listedStates())
		m, raised := raiseAgentAttention(m, true)
		m, ended := closeFinishedAgents(m, closeOnRelay)
		m, swept := runPendingSweep(m)
		m, driven := driveTeams(m, false)
		cmds = append(append(append(append(append(append(cmds, tracked...), back...), flagged...), raised...), ended...), swept...)
		// `tern-kata agents` runs without a Pane, so the Pane that heard the relay leaves it where the command line reads it.
		return m, append(append(cmds, driven...), SaveAgentStates{States: m.Agents.States})
	case SweepCommentsRead:
		m = sweepCommentsRead(m, msg)
	case AgentSessionsRead:
		return agentSessionsRead(m, msg)
	case AgentSessionTick:
		return agentSessionTick(m, msg)
	case AgentHandedOff:
		m = agentHandedOff(m, msg)
	case AgentIssueRead:
		return agentIssueRead(m, msg)
	case TeamStarted:
		return teamStarted(m, msg)
	case TeamsDriven:
		return teamsDriven(m, msg)
	case TeamPausedSet:
		return teamPausedSet(m, msg)
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
	case AgentRecordForgotten:
		return agentRecordForgotten(m, msg), nil
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
		if next, cmds, ran := runChord(m, msg.Chord, m.CurrentMode()); ran {
			return next, cmds
		}
		if inputMode(m.CurrentMode()) {
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
		next, cmds, _ := runChord(selected, msg.Chord, msg.Mode)
		return next, cmds
	case ResizeMsg:
		m.Cols, m.Rows = msg.Cols, msg.Rows
	case PaneRowsMsg:
		m.Rows = msg.Rows
	default:
		return updateLive(m, msg)
	}
	return m, nil
}

// runChord runs the binding of chord in mode, if there is one; ran is false when none is bound and m comes back unchanged.
func runChord(m Model, chord string, mode Mode) (next Model, cmds []Cmd, ran bool) {
	binding, ok := LookupBindingForMode(chord, mode)
	if !ok || binding.entry == nil {
		return m, nil, false
	}
	next, cmds = binding.run(m)
	return next, cmds, true
}
