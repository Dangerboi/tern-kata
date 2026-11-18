package model

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The Runs Page lists one Project's Runs, newest first, with the selected Run's detail beside them. It takes the list column's
// place like the Jobs Page and keeps the Issue detail column hidden: its second column is the inspector.
//
// A Run is kata's record of one execution of a Cron job or workflow, written by whichever executor ran it: an adapter, this
// machine's worker or another machine's. The Pane shows what kata holds.

// Run is the cronjob package's Run with what only the Pane reads: the names the Daemon gateway could resolve from its uids (the
// job's, the workflow's, and the Issue's short id and title), each empty when kata no longer answers for it.
type Run struct {
	cronjob.Run
	JobName, WorkflowName    string
	IssueShortID, IssueTitle string
}

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

func (r Run) Executor() string {
	if r.ExecutorLabel != "" {
		return r.ExecutorLabel
	}
	return r.Actor
}

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
	} else if r.Status != cronjob.RunRunning {
		return 0, false
	}
	return max(0, end.Sub(*r.StartedAt)), true
}

// IssueRef is the Run's Issue as a row words it: "#short title", or the uid when kata no longer answers for the Issue.
func (r Run) IssueRef() string {
	ref, title := r.IssueRefTitle()
	if title == "" {
		return ref
	}
	return ref + " " + title
}

// IssueRefTitle is the Run's Issue as its ref ("#short", or the uid when kata no longer answers for it) and its title.
func (r Run) IssueRefTitle() (ref, title string) {
	switch {
	case r.IssueUID == "":
		return "", ""
	case r.IssueShortID == "":
		return r.IssueUID, ""
	}
	return "#" + r.IssueShortID, r.IssueTitle
}

// Brief is the Run's summary in one line: its reason in words and the first line after it, as "<Reason> · <line>", else the
// summary's first line.
func (r Run) Brief() string {
	code, detail := r.Reason()
	first, _, _ := strings.Cut(strings.TrimSpace(detail), "\n")
	switch {
	case code == "":
		return first
	case first == "":
		return HoldWords(code)
	}
	return HoldWords(code) + " · " + first
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

type ShowRun struct {
	Workspace string
	ProjectID int64
	UID       string
}

func (ShowRun) cmd() {}

type RunShown struct {
	UID string
	Run Run
	Err error
}

func (RunShown) msg() {}

type RunSelectMsg struct{ UID string }

func (RunSelectMsg) msg() {}

type RunOpenMsg struct{ UID string }

func (RunOpenMsg) msg() {}

type RunsState struct {
	ProjectUID      string
	Runs            []Run
	ProjectID       int64
	ProjectArchived bool
	// JobUID narrows the Runs to one Cron job's; JobName is how the heading calls it.
	JobUID, JobName string
	// More is the cursor of the next older page; empty when Runs reaches back as far as kata holds them.
	More     string
	Selected string
	Loading  bool
	Err      error
	// reads numbers the reads asked for; stale asks for another.
	reads   Latest
	stale   bool
	refresh bool
	// Timeline says the Page shows the since-you-last-looked mode (see Model.Timeline); Seen is when this machine last marked the
	// Project's Runs seen, zero for never; SeenErr is why seen.toml could not be read.
	Timeline bool
	Seen     time.Time
	SeenErr  error
	// seenRead says seen.toml was read for the Project, seenAsked that the read is on its way; timelineSet says the mode was
	// toggled here, which an answer to the read does not undo.
	seenRead, seenAsked, timelineSet bool
	// jobsStale asks for the Project's jobs again, for the timeline's next runs.
	jobsStale bool
}

func (m Model) OnRuns() bool { return m.Page == PageRuns }

type RunRow struct {
	Run  Run
	Name string
	// IssueRef and IssueTitle are the Run's Issue: its ref, which the card draws as a link, and its title.
	IssueRef, IssueTitle string
	Executor             string
	Start                string
	Duration             string
	// Summary is the first line of the summary after its reason line; it is empty while the Run runs, when the worker's own line
	// would only repeat what the badge says.
	Summary string
	// Held says the worker of this machine holds the Run for the user. Hold is why, in words. Reason is the reason the summary
	// leads with, in words (kata's `reason: <code>` line), empty for a held Run, which shows Hold instead. State is the status
	// the row shows: "held" for a held Run, else kata's.
	Held   bool
	Hold   string
	Reason string
	State  string
}

// RunRows are the Runs Page's rows, newest first as kata lists them. now is when the start times are worded from and a running
// Run's duration is counted to.
func (m Model) RunRows(now time.Time) []RunRow {
	rows := m.allRunRows(now)
	if !m.searchingPage(PageRuns) {
		return rows
	}
	return slices.DeleteFunc(rows, func(row RunRow) bool { return !m.matchesSearch(runSearchWords(row)...) })
}

func (m Model) allRunRows(now time.Time) []RunRow {
	rows := make([]RunRow, len(m.Runs.Runs))
	for i, run := range m.Runs.Runs {
		row := RunRow{Run: run, Name: run.Name(), Executor: run.Executor(), Start: m.Calendar.Read(run.Started(), now), State: string(run.Status)}
		row.IssueRef, row.IssueTitle = run.IssueRefTitle()
		code, detail := run.Reason()
		if local, ok := m.LocalRunOf(run.UID); ok && m.IsLocalRun(run) && local.Held {
			row.Held, row.Hold, row.State = true, HoldWords(local.Hold), "held"
		} else if code != "" {
			row.Reason = HoldWords(code)
		}
		if d, ok := run.Duration(now); ok {
			row.Duration = ShortDuration(d)
		}
		if run.Status != cronjob.RunRunning {
			row.Summary, _, _ = strings.Cut(strings.TrimSpace(detail), "\n")
		}
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

func (m Model) SelectedRun() *Run {
	for _, run := range m.searchedRuns() {
		if run.UID == m.Runs.Selected {
			return &run
		}
	}
	return nil
}

// RunFact is one line of the inspector. Detail is a muted second part after Value, such as the explanation after a reason.
type RunFact struct{ Label, Value, Detail string }

type runFacts []RunFact

// add appends the fact unless its value is empty.
func (f *runFacts) add(label, value string) {
	if value != "" {
		*f = append(*f, RunFact{Label: label, Value: value})
	}
}

// RunFacts are the inspector's lines for the selected Run: its status and why, what it ran and for which Issue, who ran it and
// when, what the worker has not reported, the summary's message and token counts. Ids live in RunIDs. Lines the Run has no value
// for are left out.
func (m Model) RunFacts(now time.Time) []RunFact {
	run := m.SelectedRun()
	if run == nil {
		return nil
	}
	var facts runFacts
	add := facts.add
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
	code, detail := run.Reason()
	summary := strings.TrimSpace(detail)
	status := string(run.Status)
	local, isLocal := m.LocalRunOf(run.UID)
	held := isLocal && m.IsLocalRun(*run) && local.Held
	if held {
		status = "held"
	}
	add("Status", status)
	switch {
	case held:
		facts = append(facts, RunFact{Label: "Why", Value: HoldWords(local.Hold), Detail: strings.TrimSpace(local.Detail)})
		if summary == strings.TrimSpace(local.Detail) {
			summary = ""
		}
	case code != "":
		add("Why", HoldWords(code))
	}
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
	state, rest := m.localFacts(*run)
	facts = append(facts, state...)
	add("Summary", summary)
	facts = append(facts, rest...)
	add("Tokens in", count(run.InputTokens))
	add("Tokens out", count(run.OutputTokens))
	return facts
}

// enterRuns is the Runs Page's own work on entry (see switchPage): the Runs as the Page left them. The Jobs Page's filter key enters
// here too, with a filter set after. The Runs load as soon as the Project's id is known.
func enterRuns(m Model) (Model, []Cmd) {
	// The Page keeps its selection, loaded pages and filter while another Page is shown; the newest page is read again to join
	// what is held.
	m.Runs.refresh, m.Runs.stale, m.Runs.jobsStale = m.Runs.ProjectID != 0, true, true
	m, ask := m.askWorker(0)
	return m, []Cmd{ask}
}

// clearRuns drops the Runs Page's loading marker when the Page is left. The since-you-last-looked mode may have replaced the
// Jobs list with one that leaves out archived jobs, so the Issue detail's record of its own read is dropped to read again.
func clearRuns(m Model) Model {
	m.Runs.Loading = false
	m.Jobs.cronRead = cronRead{}
	return m
}

// settleRuns keeps the Runs Page consistent: it ends without a Project Collection, and it reads the Project's Runs when it has
// none for the Project in view or the last read went stale.
func (m Model) settleRuns() (Model, []Cmd) {
	if !m.OnRuns() {
		return m, nil
	}
	if !m.Collection.IsProject() {
		return m.leavePage(), nil
	}
	id, ok := m.jobsProjectID()
	if !ok {
		return m, nil
	}
	if id != m.Runs.ProjectID {
		m.Runs.ProjectID, m.Runs.stale = id, true
	}
	var cmds []Cmd
	if !m.Runs.seenAsked {
		m.Runs.seenAsked = true
		cmds = append(cmds, LoadSeen{ProjectUID: m.Runs.ProjectUID})
	}
	if m.Runs.Timeline && m.Runs.jobsStale {
		m.Runs.jobsStale = false
		if m.Jobs.ProjectID != id {
			m.Jobs.Jobs, m.Jobs.ProjectArchived = nil, false
		}
		m.Jobs.ProjectID, m.Jobs.Err, m.Jobs.Loading = id, nil, true
		cmds = append(cmds, LoadJobs{Workspace: m.Workspace, ProjectID: id, Archived: m.Jobs.ShowArchived, Request: m.Jobs.reads.Ask()})
	}
	if !m.Runs.stale {
		return m, cmds
	}
	m.Runs.stale, m.Runs.Loading = false, true
	refresh := m.Runs.refresh
	m.Runs.refresh = false
	return m, append(cmds, LoadRuns{Workspace: m.Workspace, ProjectID: id, JobUID: m.Runs.JobUID, Request: m.Runs.reads.Ask(), Refresh: refresh})
}

// runsLoaded applies the answer to the latest read: an older page is appended to the Runs held, the newest replaces them. A
// Project that is archived keeps the Runs already held.
func runsLoaded(m Model, msg RunsLoaded) Model {
	if !m.Runs.reads.IsCurrent(msg.Request) {
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

func runShown(m Model, msg RunShown) Model {
	if msg.Err != nil {
		return m.fail(Cannot("read", "the run", msg.Err))
	}
	runs := slices.Clone(m.Runs.Runs)
	if i := slices.IndexFunc(runs, func(r Run) bool { return r.UID == msg.UID }); i >= 0 {
		runs[i] = msg.Run
		m.Runs.Runs = runs
	}
	return m.settleRunSelection() // the Run may no longer match the search
}

func (m Model) settleRunSelection() Model {
	keepSelected(m.searchedRuns(), func(r Run) string { return r.UID }, &m.Runs.Selected)
	return m
}

func moveRun(m Model, action Action) Model {
	moveSelected(m.searchedRuns(), func(r Run) string { return r.UID }, &m.Runs.Selected, listPage, action)
	return m
}

func reloadRuns(m Model) (Model, []Cmd) {
	m.Runs.stale, m.Runs.jobsStale = true, true
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

func olderRuns(m Model) (Model, []Cmd) {
	if m.Runs.More == "" || m.Runs.Loading {
		return m, nil
	}
	m.Runs.Loading = true
	return m, []Cmd{LoadRuns{Workspace: m.Workspace, ProjectID: m.Runs.ProjectID, JobUID: m.Runs.JobUID, Before: m.Runs.More, Request: m.Runs.reads.Ask()}}
}

func inspectRun(m Model) (Model, []Cmd) {
	run := m.SelectedRun()
	if run == nil {
		return m, nil
	}
	m.Focus = ColumnDetail
	return m, []Cmd{ShowRun{Workspace: m.Workspace, ProjectID: m.Runs.ProjectID, UID: run.UID}}
}

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
		return m.fail("This run belongs to a workflow, not a job.")
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

func runsOfJob(m Model) (Model, []Cmd) {
	job := m.SelectedJob()
	if job == nil {
		return m, nil
	}
	m, cmds := switchPage(m, PageRuns)
	if m.OnRuns() {
		if m.Runs.JobUID != job.UID {
			m = m.refilteredRuns()
			m.Runs.refresh = false
		}
		m.Runs.JobUID, m.Runs.JobName = job.UID, job.Name
	}
	return m, cmds
}

func openRunIssue(m Model) Model {
	run := m.SelectedRun()
	switch {
	case run == nil:
		return m
	case run.IssueUID == "":
		return m.fail("This run has no Issue.")
	}
	uid := run.IssueUID
	m = m.leavePage()
	if m.SelectedUID == uid {
		return m.focusDetail()
	}
	return m.jumpWithin(uid, nil)
}

// updateRuns applies the Runs Page's results and selections. A read numbered below zero is the Workflows Page's, which that Page
// takes.
func updateRuns(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case RunsLoaded:
		if msg.Request < 0 {
			return m, nil, false
		}
		m = runsLoaded(m, msg)
		if m.OnRuns() && !m.Worker.inflight && m.runsRunning() {
			next, cmd := m.askWorker(workerPollEvery)
			return next, []Cmd{cmd}, true
		}
		return m, nil, true
	case SeenLoaded:
		return seenLoaded(m, msg), nil, true
	case SeenSaved:
		return seenSaved(m, msg), nil, true
	case RunShown:
		return runShown(m, msg), nil, true
	case RunSessionOpened:
		return runSessionOpened(m, msg), nil, true
	case RunSelectMsg:
		m.Runs.Selected, m.Focus = msg.UID, ColumnList
		return m, nil, true
	case RunOpenMsg:
		if mode := m.CurrentMode(); (mode == ModeRuns || mode == ModeRunInspector) && slices.ContainsFunc(m.Runs.Runs, func(r Run) bool { return r.UID == msg.UID }) {
			m.Runs.Selected, m.Focus = msg.UID, ColumnList
			next, cmds := inspectRun(m)
			return next, cmds, true
		}
		return m, nil, true
	}
	return m, nil, false
}
