package model

import "slices"

// IssueDetail owns what the detail column shows: the Issue it was asked for, that Issue's loaded authority, loading and error
// state, the Activity tab, the row cursor, the jump history, the hidden flag and the Issue opened by reference. Only this file
// writes these fields; every other caller reports what happened (an Issue was written, a snapshot arrived, a tab was chosen).
type IssueDetail struct {
	hidden    bool
	authority *IssueDetailData
	// uid is the Issue the detail was last asked to show; it differs from the selection until the next step reloads.
	uid string
	// stale asks for a reload of uid even though the selection did not change.
	stale   bool
	loading bool
	err     error
	// linkOn is the comment `]` and `[` last stepped the link cursor on, and link where it stands; any other comment starts at its first link.
	linkOn rowID
	link    int
	tab     ActivityTab
	cursor  int // an index into rows.items
	rows    detailRows
	back    []jumpEntry
	// landing is where the cursor goes once the detail of the Issue it names arrives.
	landing *jumpEntry
	// landedOn is the Issue the last jump landed on; the history ends once the detail shows another one.
	landedOn string
	// openRef is a reference being opened; openedUID is the Issue it opened, which may be outside the Collection.
	openRef, openedUID string
	// recurrenceWanted is the Issue whose recurrence `O` waits for: the detail loads even while hidden, and the Recurrence card
	// opens once the loaded detail has it.
	recurrenceWanted string
	// follow is a ticket of another Project being followed: it waits for that Project's snapshot (and Lane list on the Board).
	follow *followState
	// listJump keeps the jump history alive while the list has focus, as on the Board after a follow lands on a card.
	listJump bool
}

func (d IssueDetail) DetailAuthority() *IssueDetailData { return d.authority }
func (d IssueDetail) DetailLoading() bool               { return d.loading }
func (d IssueDetail) DetailErr() error                  { return d.err }
func (d IssueDetail) DetailIndex() int                  { return d.cursor }
func (d IssueDetail) DetailHidden() bool                { return d.hidden }
func (d IssueDetail) ActiveTab() ActivityTab            { return d.tab }
func (d IssueDetail) OpenedIssueUID() string            { return d.openedUID }
func (d IssueDetail) OpenIssueRef() string              { return d.openRef }

// updateWithDetail runs one Model step and then settles the detail: tab and Issue changes become intents, the jump history ends
// whenever focus is not on the detail or it shows an Issue no jump landed on, and a changed selection or a stale detail loads.
func updateWithDetail(m Model, msg Msg) (Model, []Cmd) {
	shown, tab, boardWasOn := m.DetailIssueUID(), m.tab, m.OnBoard()
	m, cmds := update(m, msg)
	if _, ok := msg.(StreamEvent); ok {
		switch {
		case m.OnJobs():
			m.Jobs.stale = true
		case m.OnRuns():
			m.Runs.stale = true
		}
	}
	m = m.settleBoard()
	m = m.settleWorkflows().resetPagesForProject()
	m, flowRuns := m.settleWorkflowRuns()
	cmds = append(cmds, flowRuns...)
	m, reading := m.settleWorkflowReading()
	cmds = append(cmds, reading...)
	m, load := m.lanesLoad(boardWasOn)
	cmds = append(cmds, load...)
	m, jobs := m.settleJobs()
	cmds = append(cmds, jobs...)
	m, runs := m.settleRuns()
	cmds = append(cmds, runs...)
	m, local := m.settleLocalRun()
	cmds = append(cmds, local...)
	m, cron := m.settleCronJobs()
	cmds = append(cmds, cron...)
	if m.tab != tab {
		cmds = append(cmds, RevealActivityTab{})
	}
	if now := m.DetailIssueUID(); shown != "" && now != "" && now != shown {
		// The detail scroller keeps its offset across Issues; a different Issue starts at the top.
		cmds = append(cmds, Scroll{Column: ColumnDetail, By: ScrollToStart})
	}
	if (m.Focus != ColumnDetail && !m.listJump) || m.SelectedUID != m.landedOn {
		m = m.endJumpHistory()
	}
	m = m.settleWantedRecurrence()
	m, loadIssue := m.loadDetail()
	return m.withRows(), append(cmds, loadIssue...)
}

// wantRecurrence is `O` on an Issue that recurs: the Recurrence card opens at once when the shown detail has the Issue's
// recurrence as of now, else once the detail has been loaded again. A hidden detail is not refreshed by snapshots, so it
// always loads again.
func (m Model) wantRecurrence(uid string) Model {
	m.recurrenceWanted = uid
	if m.hidden || !m.detailCurrent(uid) {
		m.stale, m.err = true, nil
	}
	return m
}

// detailCurrent says the detail shows uid, loaded and not known to be out of date.
func (m Model) detailCurrent(uid string) bool {
	return m.authority != nil && m.authority.Issue != nil && m.authority.Issue.UID == uid && !m.stale && !m.loading
}

// settleWantedRecurrence opens the card the wanted recurrence waits for, or drops the wish when the selection moved on or the
// detail failed to load.
func (m Model) settleWantedRecurrence() Model {
	uid := m.recurrenceWanted
	switch {
	case uid == "":
	case uid != m.SelectedUID || m.err != nil:
		m.recurrenceWanted = ""
	case m.detailCurrent(uid):
		m.recurrenceWanted = ""
		m = openWantedRecurrence(m)
	}
	return m
}

// shownIssue is the loaded Issue of the detail when it is the selected one, else nil.
func (m Model) shownIssue() *Issue {
	if m.authority != nil && m.authority.Issue != nil && m.authority.Issue.UID == m.SelectedUID {
		return m.authority.Issue
	}
	return nil
}

// endJumpHistory is the one way the jump history ends.
func (m Model) endJumpHistory() Model {
	m.back, m.landing, m.listJump = nil, nil, false
	return m
}

// loadDetail loads the selected Issue's detail when the selection changed or the detail went stale.
func (m Model) loadDetail() (Model, []Cmd) {
	if m.Quitting || (m.hidden && m.recurrenceWanted == "") || (m.SelectedUID == m.uid && !m.stale) {
		return m, nil
	}
	m.uid = m.SelectedUID
	m.stale = false
	if m.authority == nil || m.authority.Issue == nil || m.authority.Issue.UID != m.SelectedUID {
		// A different Issue starts at its first row; the Activity tab stays. The previous detail
		// stays on screen until the new one arrives, unless nothing is selected.
		if m.SelectedUID == "" || m.DetailIssueUID() == "" {
			m.authority = nil
		}
		m.cursor = 0
	}
	m.err = nil
	m.loading = m.SelectedUID != ""
	if !m.loading {
		return m, nil
	}
	return m, []Cmd{LoadDetail{Workspace: m.Workspace, UID: m.SelectedUID}}
}

// focusDetail gives the detail the keys and shows it.
func (m Model) focusDetail() Model {
	m.Focus = ColumnDetail
	m.hidden = false
	return m
}

// toggleDetail hides or shows the column; hiding it takes focus back to the list.
func (m Model) toggleDetail() Model {
	m.hidden = !m.hidden
	if m.hidden && m.Focus == ColumnDetail {
		m.Focus = ColumnList
	}
	return m
}

// issueWritten says the Issue was changed in kata (a Draft saved, closed, reopened, a field written); the shown detail reloads.
func (m Model) issueWritten(uid string) Model {
	if uid == m.uid {
		m.stale = true
	}
	return m
}

// conflictResolved shows the stored Issue a Draft conflicted with.
func (m Model) conflictResolved(uid string, stored *Issue) Model {
	if uid == m.uid && m.authority != nil {
		detail := *m.authority
		detail.Issue = stored
		m.authority = &detail
	}
	return m
}

// detailLoaded and detailFailed apply the result of a LoadDetail; one for an Issue no longer asked for is dropped.
func (m Model) detailLoaded(msg DetailLoaded) Model {
	if msg.UID == m.SelectedUID && msg.UID == m.uid {
		anchor, anchored := m.cursorRowID()
		m.authority = msg.Selected
		m.loading = false
		m.err = nil
		m = m.arrived(anchor, anchored)
	}
	return m
}

func (m Model) detailFailed(msg DetailFailed) Model {
	if msg.UID == m.SelectedUID && msg.UID == m.uid {
		m.loading = false
		m.err = msg.Err
		if m.DetailIssueUID() != msg.UID {
			m.authority = nil // the error replaces the previous Issue's detail on screen
		}
	}
	return m
}

// snapshotArrived refreshes the shown Issue in place from a snapshot's selected authority, keeping the tab and the cursor where
// its rows still exist. selected and anchor are the selection and the row under the cursor before the snapshot applied.
func (m Model) snapshotArrived(authority *IssueDetailData, selected string, anchor rowID, anchored bool) Model {
	if m.Focus == ColumnDetail && len(m.back) > 0 {
		m.SelectedUID = selected // the Issue jumped to stays shown
	}
	if authority != nil && authority.Issue != nil && authority.Issue.UID == m.SelectedUID {
		m.authority = authority
		m.uid = m.SelectedUID
		m.stale = false
		m.loading = false
		m.err = nil
		m = m.arrived(anchor, anchored)
	} else if m.uid == m.SelectedUID {
		m.stale = true
	}
	return m
}

// openByReference says the Issue named by the reference being opened arrived; the detail shows it even when the Collection
// does not list it.
func (m Model) openByReference(ref string, authority *IssueDetailData) (Model, []Cmd) {
	if ref != m.openRef || authority == nil || authority.Issue == nil {
		return m, nil
	}
	m.openRef = ""
	if target, across := m.otherProjectNamed(authority.Issue.ProjectName); across {
		m, cmds := m.crossJump(authority.Issue.UID, target, nil)
		if !m.OnBoard() {
			m.uid, m.stale, m.authority, m.loading, m.err = m.SelectedUID, false, authority, false, nil
		}
		return m, cmds
	}
	m.SelectedUID = authority.Issue.UID
	m.openedUID = m.SelectedUID
	m.uid = m.SelectedUID
	m.stale = false
	m.authority = authority
	m.loading = false
	m.err = nil
	m.hidden = false
	m.cursor = 0
	m.Focus = ColumnDetail
	return m, nil
}

// openRequested records a reference to open; openFailed drops it.
func (m Model) openRequested(ref string) Model {
	m.openRef = ref
	return m
}

func (m Model) openFailed(ref string) Model {
	if ref == m.openRef {
		m.openRef = ""
		return m
	}
	return m
}

// jumpEntry is a place in an Issue's detail: the Issue, its Activity tab and a row. A jump history entry records where the
// jump left from; a landing is where the detail puts the cursor once the Issue arrives.
type jumpEntry struct {
	uid    string
	tab    ActivityTab
	row    rowID
	hasRow bool
	// cross is set when the jump left this Issue for another Project: where the Pane stood, so going back returns there.
	cross *crossPlace
}

// arrived puts the cursor where a pending landing asks, else keeps it on its row (anchor) by reanchoring. A landing row that is
// gone leaves the cursor on the tab's first row.
func (m Model) arrived(anchor rowID, anchored bool) Model {
	land := m.landing
	if land == nil || land.uid != m.SelectedUID {
		return m.reanchor(anchor, anchored)
	}
	m.landing = nil
	m.cursor = 0
	m = m.withRows()
	if land.hasRow {
		m = m.reanchor(land.row, true)
		if row, ok := m.cursorRow(); ok && row.Kind == land.row.kind && row.Key == land.row.key {
			return m
		}
	}
	m.cursor = m.firstActivityIndex()
	return m
}

// jumpTo shows another Issue and remembers where it came from. A landing, when given, is the tab and row the Issue opens on;
// without one the Issue opens at its first row on the tab already shown.
func (m Model) jumpTo(uid string, landing *jumpEntry) (Model, []Cmd) {
	if uid == "" || uid == m.SelectedUID {
		return m, nil
	}
	if target, across := m.otherProject(uid); across {
		return m.crossJump(uid, target, landing)
	}
	return m.jumpWithin(uid, landing), nil
}

// jumpWithin is jumpTo inside the Pane's Collection: the Collection never changes.
func (m Model) jumpWithin(uid string, landing *jumpEntry) Model {
	if uid == "" || uid == m.SelectedUID {
		return m
	}
	row, hasRow := m.cursorRowID()
	m.back = append(m.back[:len(m.back):len(m.back)], jumpEntry{uid: m.SelectedUID, tab: m.tab, row: row, hasRow: hasRow})
	m.SelectedUID, m.landedOn = uid, uid
	m.landing = nil
	if landing != nil {
		landing.uid = uid
		m.tab, m.landing = landing.tab, landing
	}
	return m.focusDetail()
}

// jumpBack returns to the Issue, tab and row the last jump came from; a jump across Projects returns to its Project, Page,
// Lane view and focus too, and reads that Project's snapshot.
func (m Model) jumpBack() (Model, []Cmd, bool) {
	if len(m.back) == 0 {
		return m, nil, false
	}
	entry := m.back[len(m.back)-1]
	m.back = m.back[:len(m.back)-1]
	m.SelectedUID, m.landedOn = entry.uid, entry.uid
	m.tab, m.landing = entry.tab, &entry
	if entry.cross != nil {
		m, cmds := m.restorePlace(entry)
		return m, cmds, true
	}
	return m, nil, true
}

// chooseTab shows an Activity tab with the cursor on its first row.
func (m Model) chooseTab(tab ActivityTab) Model {
	m.Focus = ColumnDetail
	m.tab = tab
	m = m.withRows()
	m.cursor = m.firstActivityIndex()
	return m
}

// cycleTab moves to the next or previous Activity tab.
func (m Model) cycleTab(next bool) Model {
	if next {
		return m.chooseTab((m.tab + 1) % TabCount)
	}
	return m.chooseTab((m.tab + TabCount - 1) % TabCount)
}

// chooseRow puts the cursor on the row with the key in a tab; an unknown key leaves it where it was.
func (m Model) chooseRow(tab ActivityTab, key string) Model {
	m.Focus = ColumnDetail
	m.tab = tab
	m = m.withRows()
	if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return r.Kind == tab.kind() && r.Key == key }); i >= 0 {
		m.cursor = i
	}
	return m
}

// openCursorRow follows the row under the cursor: a child, a referenced Issue, a Cron job (on the Jobs Page) or, for a reply,
// the comment it answers.
func (m Model) openCursorRow() (Model, []Cmd) {
	if row, ok := m.withRows().cursorRow(); ok {
		if row.Kind == RowCron {
			return openJob(m, row.JobUID)
		}
		if row.Kind == RowComment {
			if moved, cmds, ok := m.openCommentLink(row); ok {
				return moved, cmds
			}
		}
		if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return row.ReplyKey != "" && r.Key == row.ReplyKey }); i >= 0 {
			m.cursor = i
			return m, nil
		}
		var landing *jumpEntry
		if row.ReferenceKey != "" {
			landing = &jumpEntry{tab: TabComments, row: rowID{RowComment, row.ReferenceKey}, hasRow: true}
		} else if row.Kind == RowComment {
			landing = &jumpEntry{tab: TabComments}
		}
		return m.jumpTo(row.ReferenceUID, landing)
	}
	return m, nil
}

// moveCursor walks the cursor over the children and the active tab's rows.
func (m Model) moveCursor(action Action) Model {
	m.linkOn = rowID{}
	m = m.withRows()
	count := len(m.rows.items)
	if count == 0 {
		return m
	}
	switch action {
	case MoveDown:
		if m.cursor < count-1 {
			m.cursor++
		}
	case MoveUp:
		m.cursor = max(m.cursor-1, 0)
	case MoveFirst:
		m.cursor = 0
	case MoveLast:
		m.cursor = count - 1
	}
	return m
}
