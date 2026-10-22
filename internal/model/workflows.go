package model

import (
	"cmp"
	"errors"
	"slices"
	"strings"
)

// The Workflows Page lists the Pane's Project's kata workflows beside an inspector of the selected one. Like the Board it takes the
// list column and hides the detail column. Its state is WorkflowsState; it is not saved.

// WorkflowDraftUID is the key of the $EDITOR round trip of the Workflow card's text.
const WorkflowDraftUID = "workflow-draft"

// WorkflowsState is the Workflows Page's state.
type WorkflowsState struct {
	// ProjectUID and ProjectID are the Project the list belongs to.
	ProjectUID string
	ProjectID  int64
	// List is every workflow the Project has, archived ones included; VisibleWorkflows leaves the archived out unless ShowArchived.
	List         []Workflow
	ShowArchived bool
	// Selected is the UID of the selected workflow, "" while there is none.
	Selected string
	// Inspecting says the inspector is open: beside the list, or in place of it in a Pane too narrow for both.
	Inspecting bool
	// Loading says a read is in flight; Err is why the last one failed.
	Loading bool
	Err     error
	Card    WorkflowCard
	// jobs are the Project's live Cron jobs as last read, to say which use each workflow; jobsRequest numbers the read (counting down
	// from zero, so no answer to the Jobs Page's reads can be taken for it).
	jobs        []CronJob
	jobsRequest int64
	// detailHidden is the detail column's hidden flag from before the Page took its place.
	detailHidden bool
}

// WorkflowCard is the Workflow card: the JSON text of a new workflow or of one being changed, edited in the card or in $EDITOR.
type WorkflowCard struct {
	Open bool
	// UID is the workflow being changed, "" for a new one; Name is what the card titles it with.
	UID, Name string
	// Base is the definition_event_uid the text started against; a save is guarded on it.
	Base string
	// Start is the text the card opened with, so a card left unchanged closes without asking.
	Start  string
	Editor TextEditor
	// Conflict is the workflow as it stands now when the save found it changed; the edited text is kept beside it.
	Conflict       *Workflow
	Saving         bool
	DiscardConfirm bool
	Err            error
}

// WorkflowOp is which write a WorkflowWritten answers.
type WorkflowOp uint8

const (
	WorkflowOpSave WorkflowOp = iota
	WorkflowOpArchive
	WorkflowOpRestore
)

// LoadWorkflows asks for every workflow of the Project, archived ones included.
type LoadWorkflows struct {
	Workspace  string
	ProjectID  int64
	ProjectUID string
}

func (LoadWorkflows) cmd() {}

// WorkflowsLoaded answers LoadWorkflows, in kata's order (name, then uid).
type WorkflowsLoaded struct {
	ProjectUID string
	Workflows  []Workflow
	Err        error
}

func (WorkflowsLoaded) msg() {}

// SaveWorkflow creates a workflow (UID "") or replaces one, guarded on Expected, the definition_event_uid the text started
// against. Definition is the checked definition as JSON.
type SaveWorkflow struct {
	Workspace, Actor, ProjectUID, UID, Expected, Name, Definition string
	ProjectID                                                     int64
}

func (SaveWorkflow) cmd() {}

// ArchiveWorkflow archives the workflow, guarded on Expected. Kata refuses while a live job uses it.
type ArchiveWorkflow struct {
	Workspace, Actor, ProjectUID, UID, Expected string
	ProjectID                                   int64
}

func (ArchiveWorkflow) cmd() {}

// RestoreWorkflow brings an archived workflow back, guarded on Expected.
type RestoreWorkflow struct {
	Workspace, Actor, ProjectUID, UID, Expected string
	ProjectID                                   int64
}

func (RestoreWorkflow) cmd() {}

// WorkflowWritten answers a save, an archive or a restore. Workflow is the workflow as written. Current, with no Workflow, says the
// guard refused: nothing was written and Current is the workflow as it stands now.
type WorkflowWritten struct {
	ProjectUID string
	Op         WorkflowOp
	UID        string
	Workflow   *Workflow
	Current    *Workflow
	Err        error
}

func (WorkflowWritten) msg() {}

// OnWorkflows says the Workflows Page is current.
func (m Model) OnWorkflows() bool { return m.Page == PageWorkflows }

// VisibleWorkflows is the list the Page draws: the live workflows, and the archived ones too once they are asked for.
func (m Model) VisibleWorkflows() []Workflow {
	w := m.Workflows
	if w.ShowArchived {
		return w.List
	}
	return slices.DeleteFunc(slices.Clone(w.List), func(x Workflow) bool { return x.Archived })
}

// WorkflowJobs are the live Cron jobs that run the workflow with uid: those whose action names it. They are what keeps kata from
// archiving it. Empty until the Page's read of the Project's jobs answers.
func (m Model) WorkflowJobs(uid string) []CronJob {
	var jobs []CronJob
	for _, j := range m.Workflows.jobs {
		if !j.Archived() && j.Definition.Action.WorkflowUID == uid {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// SelectedWorkflow is the selected workflow, false when the list is empty.
func (m Model) SelectedWorkflow() (Workflow, bool) {
	for _, w := range m.VisibleWorkflows() {
		if w.UID == m.Workflows.Selected {
			return w, true
		}
	}
	return Workflow{}, false
}

// collectionProjectID is the numeric id of the Project the current Project Collection shows, 0 when the Pane has not read it.
func (m Model) collectionProjectID() int64 {
	uid := m.Collection.ProjectUID
	if uid == m.Project.UID && m.Project.ID != 0 {
		return m.Project.ID
	}
	if entry, ok := CatalogEntry(m.Snapshot, uid); ok {
		return entry.Project.ID
	}
	return 0
}

// enterWorkflows is the Workflows Page's entry: the Project's workflows are read again each time, and the ones held stay on
// screen until they answer.
func enterWorkflows(m Model) (Model, []Cmd) {
	m, cmds, ok := enterProjectPage(m, "Workflows Page")
	if !ok {
		return m, cmds
	}
	w := &m.Workflows
	if w.ProjectUID != m.Collection.ProjectUID {
		*w = WorkflowsState{ProjectUID: m.Collection.ProjectUID, jobsRequest: w.jobsRequest}
	}
	w.ProjectID = m.collectionProjectID()
	w.detailHidden = m.DetailHidden()
	w.Inspecting = false
	m.Page = PageWorkflows
	m.hidden = true
	m.Focus = ColumnList
	if w.ProjectID == 0 {
		w.Err = errors.New("kata has not told this Pane the Project's id yet; open the Page again in a moment.")
		return m, cmds
	}
	w.Err = nil
	w.Loading = true
	w.jobsRequest--
	return m, append(cmds,
		LoadWorkflows{Workspace: m.Workspace, ProjectID: w.ProjectID, ProjectUID: w.ProjectUID},
		LoadJobs{Workspace: m.Workspace, ProjectID: w.ProjectID, Request: w.jobsRequest})
}

// workflowsOff leaves the Workflows Page for the Issues Page and gives the detail column back, unless something already focused it.
func (m Model) workflowsOff() Model {
	m.Page = PageIssues
	m.Workflows.Inspecting = false
	if m.Focus != ColumnDetail {
		m.hidden = m.Workflows.detailHidden
	}
	return m.revealInList(m.SelectedUID).WithVisibleSelection()
}

// settleWorkflows ends the Page when the Pane no longer shows its Project or the list lost focus to the Menu.
func (m Model) settleWorkflows() Model {
	if m.Page != PageWorkflows {
		return m
	}
	if !m.Collection.IsProject() || m.Collection.ProjectUID != m.Workflows.ProjectUID || m.Focus != ColumnList {
		return m.workflowsOff()
	}
	return m
}

// selectWorkflow keeps the selection on a workflow the list shows: the one selected, else the one now at the old row, else the last.
func (m Model) selectWorkflow(row int) Model {
	visible := m.VisibleWorkflows()
	if len(visible) == 0 {
		m.Workflows.Selected = ""
		return m
	}
	if _, ok := m.SelectedWorkflow(); ok {
		return m
	}
	m.Workflows.Selected = visible[max(0, min(row, len(visible)-1))].UID
	return m
}

func (m Model) selectedRow() int {
	return slices.IndexFunc(m.VisibleWorkflows(), func(w Workflow) bool { return w.UID == m.Workflows.Selected })
}

// moveWorkflow moves the selection by a row, a page or to either end.
func moveWorkflow(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model {
		visible := m.VisibleWorkflows()
		if len(visible) == 0 {
			return m
		}
		page := max(m.Rows-2, 1)
		row := max(m.selectedRow(), 0)
		switch action {
		case MoveDown:
			row++
		case MoveUp:
			row--
		case PageDown:
			row += page
		case PageUp:
			row -= page
		case MoveFirst:
			row = 0
		case MoveLast:
			row = len(visible) - 1
		}
		m.Workflows.Selected = visible[max(0, min(row, len(visible)-1))].UID
		return m
	})
}

func inspectWorkflow(m Model) Model {
	if _, ok := m.SelectedWorkflow(); ok {
		m.Workflows.Inspecting = true
	}
	return m
}

// workflowsBack is esc on the Page: it closes the inspector, and with none open goes to the Menu.
func workflowsBack(m Model) (Model, []Cmd) {
	if m.Workflows.Inspecting {
		m.Workflows.Inspecting = false
		return m, nil
	}
	return drillOut(m)
}

func toggleArchivedWorkflows(m Model) Model {
	row := m.selectedRow()
	m.Workflows.ShowArchived = !m.Workflows.ShowArchived
	return m.selectWorkflow(row)
}

func newWorkflow(m Model) Model {
	if m.Workflows.ProjectID == 0 {
		return m
	}
	m.Workflows.Card = WorkflowCard{Open: true, Start: NewWorkflowText, Editor: NewTextEditor(NewWorkflowText)}
	return m
}

func editWorkflow(m Model) Model {
	w, ok := m.SelectedWorkflow()
	switch {
	case !ok:
	case w.Archived:
		m.Notice = w.Name + " is archived: restore it first."
	default:
		text := w.Text()
		m.Workflows.Card = WorkflowCard{Open: true, UID: w.UID, Name: w.Name, Base: w.EventUID, Start: text, Editor: NewTextEditor(text)}
	}
	return m
}

// changeWorkflowState is `x` and `r`: archive the selected workflow, or restore it, guarded on the version on screen.
func changeWorkflowState(restore bool) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		w, ok := m.SelectedWorkflow()
		switch {
		case !ok:
			return m, nil
		case m.Stale:
			m.Notice = errDraftReadOnly.Error()
			return m, nil
		case restore && !w.Archived:
			m.Notice = w.Name + " is not archived."
			return m, nil
		case !restore && w.Archived:
			m.Notice = w.Name + " is already archived."
			return m, nil
		}
		s := m.Workflows
		if restore {
			return m, []Cmd{RestoreWorkflow{Workspace: m.Workspace, Actor: m.Actor, ProjectID: s.ProjectID, ProjectUID: s.ProjectUID, UID: w.UID, Expected: w.EventUID}}
		}
		return m, []Cmd{ArchiveWorkflow{Workspace: m.Workspace, Actor: m.Actor, ProjectID: s.ProjectID, ProjectUID: s.ProjectUID, UID: w.UID, Expected: w.EventUID}}
	}
}

// withWorkflow puts w in the list, in kata's order, replacing the workflow with its uid.
func (s WorkflowsState) withWorkflow(w Workflow) WorkflowsState {
	list := slices.Clone(s.List)
	if i := slices.IndexFunc(list, func(x Workflow) bool { return x.UID == w.UID }); i >= 0 {
		list[i] = w
	} else {
		list = append(list, w)
	}
	slices.SortStableFunc(list, func(a, b Workflow) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.UID, b.UID)) })
	s.List = list
	return s
}

// updateWorkflows applies the Workflows Page's results and the $EDITOR round trip of its card.
func updateWorkflows(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case WorkflowsLoaded:
		if msg.ProjectUID != m.Workflows.ProjectUID {
			return m, nil, true
		}
		m.Workflows.Loading, m.Workflows.Err = false, msg.Err
		if msg.Err == nil {
			row := m.selectedRow()
			m.Workflows.List = msg.Workflows
			m = m.selectWorkflow(row)
		}
		return m, nil, true
	case WorkflowWritten:
		return workflowWritten(m, msg), nil, true
	case JobsLoaded:
		if msg.Request >= 0 || msg.Request != m.Workflows.jobsRequest {
			return m, nil, false
		}
		if msg.Err == nil && !msg.ProjectArchived {
			m.Workflows.jobs = msg.Jobs
		}
		return m, nil, true
	case WorkflowSelectMsg:
		if slices.ContainsFunc(m.VisibleWorkflows(), func(w Workflow) bool { return w.UID == msg.UID }) {
			m.Workflows.Selected = msg.UID
		}
		return m, nil, true
	case DraftEditorClosed:
		if msg.UID != WorkflowDraftUID {
			return m, nil, false
		}
		if c := &m.Workflows.Card; c.Open {
			c.Err = msg.Err
			if msg.Err == nil {
				text := normalizeLineBreaks(msg.Text)
				c.Editor = c.Editor.ApplyEdit(0, len16(c.Editor.Text), text, len16(text), len16(c.Editor.Text))
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

func workflowWritten(m Model, msg WorkflowWritten) Model {
	card := &m.Workflows.Card
	saving := msg.Op == WorkflowOpSave && card.Open && card.Saving
	if saving {
		card.Saving = false
	}
	if msg.ProjectUID != m.Workflows.ProjectUID {
		if saving && msg.Err == nil {
			*card = WorkflowCard{}
		}
		return m
	}
	switch {
	case msg.Err != nil && saving:
		card.Err = msg.Err
	case msg.Err != nil:
		m.Notice = msg.Err.Error()
	case msg.Workflow == nil && msg.Current != nil:
		if saving {
			card.Conflict, card.Err = msg.Current, nil
			return m
		}
		m.Workflows = m.Workflows.withWorkflow(*msg.Current)
		m.Notice = msg.Current.Name + " changed meanwhile: nothing was written."
		return m.selectWorkflow(0)
	case msg.Workflow != nil:
		row := m.selectedRow()
		m.Workflows = m.Workflows.withWorkflow(*msg.Workflow)
		if saving {
			m.Workflows.Card = WorkflowCard{}
			m.Workflows.Selected = msg.Workflow.UID
			return m
		}
		return m.selectWorkflow(row)
	}
	return m
}

// workflowCard is the Workflow card's key handler: the shared Draft keys for save, discard and $EDITOR, over the card's own
// text, and Overwrite and Discard Draft once a save found the workflow changed.
func workflowCardKey(m Model, msg KeyMsg) (Model, []Cmd) {
	c := m.Workflows.Card
	if c.Saving {
		return m, nil
	}
	binding, ok := LookupBindingForMode(msg.Chord, m.CurrentMode())
	switch {
	case c.Conflict != nil:
		switch {
		case ok && binding.Action == OverwriteDraft:
			c.Base, c.Conflict = c.Conflict.EventUID, nil
			m.Workflows.Card = c
			return saveWorkflowCard(m)
		case ok && binding.Action == DiscardDraft:
			m.Workflows = m.Workflows.withWorkflow(*c.Conflict)
			m.Workflows.Card = WorkflowCard{}
			return m.selectWorkflow(0), nil
		}
		return m, nil
	case c.DiscardConfirm:
		switch {
		case ok && binding.Action == DiscardDraft:
			m.Workflows.Card = WorkflowCard{}
		case ok && binding.Action == KeepDraft:
			m.Workflows.Card.DiscardConfirm = false
		}
		return m, nil
	}
	if ok {
		switch binding.Action {
		case SaveDraftAction:
			return saveWorkflowCard(m)
		case CancelDraft:
			if c.Editor.Text == c.Start || strings.TrimSpace(c.Editor.Text) == "" {
				m.Workflows.Card = WorkflowCard{}
			} else {
				m.Workflows.Card.DiscardConfirm = true
			}
			return m, nil
		case ExternalEditorAction:
			return m, []Cmd{OpenDraftEditor{UID: WorkflowDraftUID, Text: c.Editor.Text, Suffix: ".json"}}
		}
	}
	m.Workflows.Card.Editor = editorKey(c.Editor, binding, ok, msg)
	m.Workflows.Card.Err = nil
	return m, nil
}

// saveWorkflowCard checks the text against kata's rules and, when it passes, sends it. A text that fails stays in the card with
// the problem named.
func saveWorkflowCard(m Model) (Model, []Cmd) {
	c := &m.Workflows.Card
	if m.Stale {
		c.Err = errDraftReadOnly
		return m, nil
	}
	doc, err := ParseWorkflow(c.Editor.Text)
	if err != nil {
		c.Err = err
		return m, nil
	}
	c.Saving, c.Err = true, nil
	s := m.Workflows
	return m, []Cmd{SaveWorkflow{Workspace: m.Workspace, Actor: m.Actor, ProjectID: s.ProjectID, ProjectUID: s.ProjectUID, UID: c.UID, Expected: c.Base, Name: doc.Name, Definition: doc.Definition}}
}

// workflowMode is the keymap mode of the Workflow card.
func workflowMode(m Model) Mode {
	switch c := m.Workflows.Card; {
	case c.Conflict != nil:
		return ModeConflict
	case c.DiscardConfirm:
		return ModeDiscard
	}
	return ModeEditor
}

func workflowCardEdit(m Model, apply func(TextEditor) TextEditor) Model {
	if c := m.Workflows.Card; !c.Saving && !c.DiscardConfirm && c.Conflict == nil {
		m.Workflows.Card.Editor = apply(c.Editor)
	}
	return m
}

// workflowModes is where the Page's keys are bound.
var workflowModes = []Mode{ModeWorkflows}

// workflowActions are the Workflows Page's main-surface actions.
func workflowActions() []actionEntry {
	return []actionEntry{
		{action: MoveDown, modes: workflowModes, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "move", footer: workflowModes, footerKey: "↑↓", sheet: []sheetLine{{sheetWorkflows, 1, "next workflow"}}, handle: moveWorkflow(MoveDown)},
		{action: MoveUp, modes: workflowModes, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous workflow", sheet: []sheetLine{{sheetWorkflows, 2, "previous workflow"}}, handle: moveWorkflow(MoveUp)},
		{action: MoveFirst, modes: workflowModes, keys: keys("g", "g", "home", "home"), help: "first workflow", handle: moveWorkflow(MoveFirst)},
		{action: MoveLast, modes: workflowModes, keys: keys("shift+g", "G", "end", "end"), help: "last workflow", handle: moveWorkflow(MoveLast)},
		{action: PageUp, modes: workflowModes, keys: pageUpKeys, help: "page up", handle: moveWorkflow(PageUp)},
		{action: PageDown, modes: workflowModes, keys: pageDownKeys, help: "page down", handle: moveWorkflow(PageDown)},
		{action: OpenWorkflow, modes: workflowModes, keys: keys("enter", "↵"), help: "steps", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 3, "show the steps"}}, handle: pure(inspectWorkflow)},
		{action: NewWorkflow, modes: workflowModes, keys: keys("n", "n"), help: "new", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 4, "new workflow (Workflow card)"}}, handle: pure(newWorkflow)},
		{action: EditWorkflow, modes: workflowModes, keys: keys("e", "e"), help: "edit", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 5, "edit the workflow (Workflow card)"}}, handle: pure(editWorkflow)},
		{action: ArchiveWorkflowAction, modes: workflowModes, keys: keys("x", "x"), help: "archive", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 6, "archive"}}, handle: changeWorkflowState(false)},
		{action: RestoreWorkflowAction, modes: workflowModes, keys: keys("r", "r"), help: "restore", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 7, "restore"}}, handle: changeWorkflowState(true)},
		{action: ToggleArchivedWorkflows, modes: workflowModes, keys: keys("a", "a"), help: "archived", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 8, "show or hide archived workflows"}}, handle: pure(toggleArchivedWorkflows)},
		{action: DrillOut, modes: workflowModes, keys: keys("escape", "esc"), help: "back", footer: workflowModes, sheet: []sheetLine{{sheetWorkflows, 9, "close the steps, or go to the Menu"}}, handle: workflowsBack},
	}
}

// WorkflowSelectMsg is a click on a workflow's row.
type WorkflowSelectMsg struct{ UID string }

func (WorkflowSelectMsg) msg() {}
