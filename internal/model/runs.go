package model

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// The Runs Page lists one Project's Runs, newest first, with the selected Run's detail beside them. It takes the list column's
// place like the Jobs Page and keeps the Issue detail column hidden: its second column is the inspector.
//
// A Run is kata's record of one execution of a Cron job or workflow, written by whichever executor ran it: an adapter, this
// machine's worker or another machine's. The Pane shows what kata holds.

// RunStatus is where a Run stands, as kata records it.
type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	// RunUnknown is a Run an executor could not settle: held, or lost track of.
	RunUnknown RunStatus = "unknown"
)

// Run is one kata run observation as the model holds it. A Run names a Cron job, a workflow or both, and the Issue it ran for.
// JobName, WorkflowName and the Issue's short id and title are what the Daemon gateway could resolve from the uids; each is
// empty when kata no longer answers for it.
type Run struct {
	UID string
	// JobUID and WorkflowUID are what the Run belongs to; DefinitionEventUID and WorkflowDefinitionEventUID are the versions of
	// them it ran.
	JobUID, JobName                    string
	DefinitionEventUID                 string
	WorkflowUID, WorkflowName          string
	WorkflowDefinitionEventUID         string
	OccurrenceKey                      string
	IssueUID, IssueShortID, IssueTitle string
	// Actor is who observed the Run first; Teammate and ExecutorLabel say which agent and machine did.
	Actor, Teammate, ExecutorLabel string
	Status                         RunStatus
	// Message and the token counts are the summary. A count kata does not hold is nil.
	Message                   string
	InputTokens, OutputTokens *int64
	Revision                  int64
	CreatedAt, UpdatedAt      time.Time
	StartedAt, EndedAt        *time.Time
}

// Name is what the Run ran: the job's name, else the workflow's, else the uid of whichever kata holds.
func (r Run) Name() string {
	switch {
	case r.JobName != "":
		return r.JobName
	case r.WorkflowName != "":
		return r.WorkflowName
	case r.JobUID != "":
		return r.JobUID
	}
	return r.WorkflowUID
}

// Executor is who ran it: the machine's label, else the actor.
func (r Run) Executor() string {
	if r.ExecutorLabel != "" {
		return r.ExecutorLabel
	}
	return r.Actor
}

// Started is when the Run started, else when it was first observed.
func (r Run) Started() time.Time {
	if r.StartedAt != nil {
		return *r.StartedAt
	}
	return r.CreatedAt
}

// Duration is how long the Run took, or has taken so far while it runs; false when it has no start and no end to count from.
func (r Run) Duration(now time.Time) (time.Duration, bool) {
	if r.StartedAt == nil {
		return 0, false
	}
	end := now
	if r.EndedAt != nil {
		end = *r.EndedAt
	} else if r.Status != RunRunning {
		return 0, false
	}
	return max(0, end.Sub(*r.StartedAt)), true
}

// IssueRef is the Run's Issue as a row words it: "#short title", or the uid when kata no longer answers for the Issue.
func (r Run) IssueRef() string {
	switch {
	case r.IssueUID == "":
		return ""
	case r.IssueShortID == "":
		return r.IssueUID
	}
	return "#" + r.IssueShortID + " " + r.IssueTitle
}

// LoadRuns asks for the Project's Runs, newest first. JobUID narrows them to one Cron job's. Before is the cursor of the page
// before: empty for the newest page, else the NextBefore of the last answer. Request numbers the asks: an answer to an older
// one is dropped.
type LoadRuns struct {
	Workspace string
	ProjectID int64
	JobUID    string
	Before    string
	Request   int64
	// Refresh says the newest page is read again to follow runs in flight: the answer joins the Runs held instead of replacing them.
	Refresh bool
}

func (LoadRuns) cmd() {}

// RunsLoaded answers LoadRuns. NextBefore is the cursor of the next older page, empty when there is none. Before echoes the ask
// so the answer is appended to the Runs held rather than replacing them. ProjectArchived says the Project is archived: kata
// answers no Run of it.
type RunsLoaded struct {
	Request         int64
	Runs            []Run
	Before          string
	NextBefore      string
	ProjectArchived bool
	Refresh         bool
	Err             error
}

func (RunsLoaded) msg() {}

// ShowRun asks for one Run by uid.
type ShowRun struct {
	Workspace string
	ProjectID int64
	UID       string
}

func (ShowRun) cmd() {}

// RunShown answers ShowRun.
type RunShown struct {
	UID string
	Run Run
	Err error
}

func (RunShown) msg() {}

// RunSelectMsg is a click on a Run's row.
type RunSelectMsg struct{ UID string }

func (RunSelectMsg) msg() {}

// RunsState is the Runs Page's state. It is not saved: a Pane starts on the Issues Page.
type RunsState struct {
	// Runs are the Project's Runs as last read, newest first: the newest page and any older pages asked for.
	Runs []Run
	// ProjectID is the Project Runs belongs to; zero before a read was asked for.
	ProjectID       int64
	ProjectArchived bool
	// JobUID narrows the Runs to one Cron job's; JobName is how the heading calls it.
	JobUID, JobName string
	// More is the cursor of the next older page; empty when Runs reaches back as far as kata holds them.
	More string
	// Selected is the uid of the Run the inspector shows.
	Selected string
	Loading  bool
	Err      error
	// request counts the reads asked for; stale asks for another.
	request int64
	stale   bool
	// refresh makes the next read of the newest page join the Runs held.
	refresh bool
	// detailHidden is the detail column's hidden flag from before the Runs Page took its place.
	detailHidden bool
}

// OnRuns says the Runs Page is current.
func (m Model) OnRuns() bool { return m.Page == PageRuns }

// RunRow is one Run as the Runs Page's list draws it. A part that can't be worked out is empty.
type RunRow struct {
	Run      Run
	Name     string
	Issue    string
	Executor string
	Start    string
	Duration string
	Summary  string
	// Held says the worker of this machine holds the Run for the user; Hold is why, in words. State is the status the row shows:
	// "held" for a held Run, else kata's.
	Held  bool
	Hold  string
	State string
}

// RunRows are the Runs Page's rows, newest first as kata lists them. now is when the start times are worded from and a running
// Run's duration is counted to.
func (m Model) RunRows(now time.Time) []RunRow {
	rows := make([]RunRow, len(m.Runs.Runs))
	for i, run := range m.Runs.Runs {
		row := RunRow{Run: run, Name: run.Name(), Issue: run.IssueRef(), Executor: run.Executor(), Start: m.Calendar.Read(run.Started(), now), State: string(run.Status)}
		if local, ok := m.LocalRunOf(run.UID); ok && m.IsLocalRun(run) && local.Held {
			row.Held, row.Hold, row.State = true, holdWords(local.Hold), "held"
		}
		if d, ok := run.Duration(now); ok {
			row.Duration = ShortDuration(d)
		}
		row.Summary, _, _ = strings.Cut(strings.TrimSpace(run.Message), "\n")
		rows[i] = row
	}
	return rows
}

// ShortDuration words a duration in its two largest units: "45s", "2m 05s", "1h 02m".
func ShortDuration(d time.Duration) string {
	secs := int64(d / time.Second)
	switch {
	case secs < 60:
		return strconv.FormatInt(secs, 10) + "s"
	case secs < 3600:
		return strconv.FormatInt(secs/60, 10) + "m " + twoDigits(secs%60) + "s"
	case secs < 86400:
		return strconv.FormatInt(secs/3600, 10) + "h " + twoDigits(secs%3600/60) + "m"
	}
	return strconv.FormatInt(secs/86400, 10) + "d " + twoDigits(secs%86400/3600) + "h"
}

func twoDigits(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// SelectedRun is the Run the inspector shows: the selected one if the list holds it, else nil.
func (m Model) SelectedRun() *Run {
	for _, run := range m.Runs.Runs {
		if run.UID == m.Runs.Selected {
			return &run
		}
	}
	return nil
}

// RunFact is one line of the inspector.
type RunFact struct{ Label, Value string }

// RunFacts are the inspector's lines for the selected Run: its status, what it ran and for which Issue, who ran it and when,
// the summary's message and token counts, and the references kata holds. Lines the Run has no value for are left out.
func (m Model) RunFacts(now time.Time) []RunFact {
	run := m.SelectedRun()
	if run == nil {
		return nil
	}
	var facts []RunFact
	add := func(label, value string) {
		if value != "" {
			facts = append(facts, RunFact{label, value})
		}
	}
	count := func(n *int64) string {
		if n == nil {
			return ""
		}
		return strconv.FormatInt(*n, 10)
	}
	stamp := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return m.Calendar.Stamp(*t)
	}
	add("Status", string(run.Status))
	hold, local := m.localFacts(*run)
	facts = append(facts, hold...)
	add("Job", run.JobName)
	add("Workflow", run.WorkflowName)
	add("Issue", run.IssueRef())
	add("Executor", run.ExecutorLabel)
	add("Actor", run.Actor)
	add("Teammate", run.Teammate)
	add("Started", stamp(run.StartedAt))
	add("Ended", stamp(run.EndedAt))
	if d, ok := run.Duration(now); ok {
		add("Duration", ShortDuration(d))
	}
	add("Summary", strings.TrimSpace(run.Message))
	facts = append(facts, local...)
	add("Input tokens", count(run.InputTokens))
	add("Output tokens", count(run.OutputTokens))
	add("Occurrence", run.OccurrenceKey)
	add("Job UID", run.JobUID)
	add("Job version", run.DefinitionEventUID)
	add("Workflow UID", run.WorkflowUID)
	add("Workflow version", run.WorkflowDefinitionEventUID)
	add("Issue UID", run.IssueUID)
	add("Observed", m.Calendar.Stamp(run.CreatedAt))
	add("Updated", m.Calendar.Stamp(run.UpdatedAt))
	add("Revision", strconv.FormatInt(run.Revision, 10))
	add("UID", run.UID)
	return facts
}

// enterRunsPage is the Runs Page's entry from its key: all the Project's Runs. The jobs page's filter key enters through
// enterRuns with a filter set after.
func enterRunsPage(m Model) (Model, []Cmd) {
	m.Runs.JobUID, m.Runs.JobName = "", ""
	return enterRuns(m)
}

// enterRuns is the Runs Page's entry on the Pane's Project. The Runs load as soon as the Project's id is known.
func enterRuns(m Model) (Model, []Cmd) {
	m, cmds, ok := enterProjectPage(m, "Runs Page")
	if !ok {
		return m, cmds
	}
	m.Page = PageRuns
	m.Runs.detailHidden = m.DetailHidden()
	m.hidden = true
	m.Focus = ColumnList
	m.Runs.ProjectID, m.Runs.Runs, m.Runs.More, m.Runs.Selected, m.Runs.Err = 0, nil, "", "", nil
	m.Runs.ProjectArchived = false
	m, ask := m.askWorker(0)
	return m, append(cmds, ask)
}

// runsOff leaves the Runs Page for the Issues Page, the keys on the list, and gives the detail column back.
func (m Model) runsOff() Model {
	m.Page = PageIssues
	if m.Focus == ColumnDetail {
		m.Focus = ColumnList
	}
	m.hidden = m.Runs.detailHidden
	m.Runs.Loading = false
	m.Runs.JobUID, m.Runs.JobName = "", ""
	return m.revealInList(m.SelectedUID).WithVisibleSelection()
}

// settleRuns keeps the Runs Page consistent: it ends without a Project Collection, and it reads the Project's Runs when it has
// none for the Project in view or the last read went stale.
func (m Model) settleRuns() (Model, []Cmd) {
	if !m.OnRuns() {
		return m, nil
	}
	if !m.Collection.IsProject() {
		return m.runsOff(), nil
	}
	id, ok := m.jobsProjectID()
	if !ok {
		return m, nil
	}
	if id != m.Runs.ProjectID {
		m.Runs.ProjectID, m.Runs.Runs, m.Runs.More, m.Runs.Selected, m.Runs.Err, m.Runs.ProjectArchived = id, nil, "", "", nil, false
		m.Runs.stale = true
	}
	if !m.Runs.stale {
		return m, nil
	}
	m.Runs.stale, m.Runs.Loading = false, true
	m.Runs.request++
	refresh := m.Runs.refresh
	m.Runs.refresh = false
	return m, []Cmd{LoadRuns{Workspace: m.Workspace, ProjectID: id, JobUID: m.Runs.JobUID, Request: m.Runs.request, Refresh: refresh}}
}

// runsLoaded applies the answer to the latest read: an older page is appended to the Runs held, the newest replaces them. A
// Project that is archived keeps the Runs already held.
func runsLoaded(m Model, msg RunsLoaded) Model {
	if msg.Request != m.Runs.request {
		return m
	}
	m.Runs.Loading = false
	m.Runs.Err = msg.Err
	if msg.Err != nil {
		return m.settleRunSelection()
	}
	m.Runs.ProjectArchived = msg.ProjectArchived
	if msg.ProjectArchived {
		return m.settleRunSelection()
	}
	if msg.Refresh {
		// The newest page, read again: its Runs replace the ones held of the same uid and the older pages already read stay.
		held := slices.DeleteFunc(slices.Clone(m.Runs.Runs), func(r Run) bool {
			return slices.ContainsFunc(msg.Runs, func(n Run) bool { return n.UID == r.UID })
		})
		if len(held) == 0 {
			m.Runs.More = msg.NextBefore
		}
		m.Runs.Runs = append(slices.Clone(msg.Runs), held...)
		return m.settleRunSelection()
	}
	m.Runs.More = msg.NextBefore
	if msg.Before == "" {
		m.Runs.Runs = msg.Runs
		return m.settleRunSelection()
	}
	held := slices.Clone(m.Runs.Runs)
	for _, run := range msg.Runs {
		if !slices.ContainsFunc(held, func(r Run) bool { return r.UID == run.UID }) {
			held = append(held, run)
		}
	}
	m.Runs.Runs = held
	return m.settleRunSelection()
}

// runShown replaces the Run held with the one kata answered with, when the list holds it.
func runShown(m Model, msg RunShown) Model {
	if msg.Err != nil {
		m.Notice = "Reading the run failed: " + msg.Err.Error()
		return m
	}
	runs := slices.Clone(m.Runs.Runs)
	if i := slices.IndexFunc(runs, func(r Run) bool { return r.UID == msg.UID }); i >= 0 {
		runs[i] = msg.Run
		m.Runs.Runs = runs
	}
	return m
}

// settleRunSelection keeps the selection on a listed Run, the first when it is gone.
func (m Model) settleRunSelection() Model {
	if slices.ContainsFunc(m.Runs.Runs, func(r Run) bool { return r.UID == m.Runs.Selected }) {
		return m
	}
	m.Runs.Selected = ""
	if len(m.Runs.Runs) > 0 {
		m.Runs.Selected = m.Runs.Runs[0].UID
	}
	return m
}

// moveRun moves the selection by action: one row, to either end, or a page.
func moveRun(m Model, action Action) Model {
	runs := m.Runs.Runs
	if len(runs) == 0 {
		return m
	}
	at := slices.IndexFunc(runs, func(r Run) bool { return r.UID == m.Runs.Selected })
	switch action {
	case MoveDown:
		at++
	case MoveUp:
		at--
	case PageDown:
		at += 10
	case PageUp:
		at -= 10
	case MoveFirst:
		at = 0
	case MoveLast:
		at = len(runs) - 1
	}
	m.Runs.Selected = runs[max(0, min(at, len(runs)-1))].UID
	return m
}

// reloadRuns reads the newest Runs again.
func reloadRuns(m Model) (Model, []Cmd) {
	m.Runs.stale = true
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

// olderRuns asks for the next older page of Runs, when kata holds more.
func olderRuns(m Model) (Model, []Cmd) {
	if m.Runs.More == "" || m.Runs.Loading {
		return m, nil
	}
	m.Runs.Loading = true
	m.Runs.request++
	return m, []Cmd{LoadRuns{Workspace: m.Workspace, ProjectID: m.Runs.ProjectID, JobUID: m.Runs.JobUID, Before: m.Runs.More, Request: m.Runs.request}}
}

// inspectRun is enter on the Run list: the inspector takes the keys and the Run is read again for the detail.
func inspectRun(m Model) (Model, []Cmd) {
	run := m.SelectedRun()
	if run == nil {
		return m, nil
	}
	m.Focus = ColumnDetail
	return m, []Cmd{ShowRun{Workspace: m.Workspace, ProjectID: m.Runs.ProjectID, UID: run.UID}}
}

// runInspectorBack is esc in the inspector: the keys go back to the list.
func runInspectorBack(m Model) Model {
	m.Focus = ColumnList
	return m
}

// toggleRunJobFilter is f: on the Runs of every job it narrows them to the selected Run's job, on a narrowed list it lifts the
// filter. A Run of a workflow alone has no job to narrow to.
func toggleRunJobFilter(m Model) Model {
	switch run := m.SelectedRun(); {
	case m.Runs.JobUID != "":
		m.Runs.JobUID, m.Runs.JobName = "", ""
	case run == nil:
		return m
	case run.JobUID == "":
		m.Notice = "This run belongs to a workflow, not a Cron job."
		return m
	default:
		m.Runs.JobUID, m.Runs.JobName = run.JobUID, run.JobName
	}
	return m.refilteredRuns()
}

// refilteredRuns drops the Runs held and asks for the filter's.
func (m Model) refilteredRuns() Model {
	m.Runs.Runs, m.Runs.More, m.Runs.Selected = nil, "", ""
	m.Runs.stale = true
	m.Focus = ColumnList
	return m
}

// runsOfJob is r on the Jobs Page: the Runs Page filtered to the selected job.
func runsOfJob(m Model) (Model, []Cmd) {
	job := m.SelectedJob()
	if job == nil {
		return m, nil
	}
	m = m.jobsOff()
	m, cmds := enterRuns(m)
	if m.OnRuns() {
		m.Runs.JobUID, m.Runs.JobName = job.UID, job.Name
	}
	return m, cmds
}

// openRunIssue is i: the Run's Issue opens on the Issues Page, with its detail focused.
func openRunIssue(m Model) Model {
	run := m.SelectedRun()
	switch {
	case run == nil:
		return m
	case run.IssueUID == "":
		m.Notice = "This run has no Issue."
		return m
	}
	uid := run.IssueUID
	m = m.runsOff()
	if m.SelectedUID == uid {
		// The detail was not kept fresh while the Runs Page hid it.
		m.stale = true
		return m.focusDetail()
	}
	return m.jumpTo(uid, nil)
}
