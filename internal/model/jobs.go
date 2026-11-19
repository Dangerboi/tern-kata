package model

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The Jobs Page lists one Project's Cron jobs, native and adapter, with the selected job's definition and parent beside them.
// It takes the list column's place like the Board and keeps the Issue detail column hidden: its second column is the inspector.

// LoadJobs asks for the Cron jobs of the Project with ProjectID. Archived adds the archived ones. Request numbers the asks: an
// answer to an older one is dropped.
type LoadJobs struct {
	Workspace string
	ProjectID int64
	Archived  bool
	Request   int64
	// Activity asks for what each job has done lately with the jobs: the Jobs Page's reads do, the ones that only need the
	// definitions do not.
	Activity bool
}

func (LoadJobs) cmd() {}

// JobsLoaded answers LoadJobs. ProjectArchived says the Project is archived: kata answers no job of it, so Jobs is empty and the
// jobs the Pane already holds are paused for that reason.
type JobsLoaded struct {
	Request         int64
	Jobs            []CronJob
	ProjectArchived bool
	Activity        map[string]JobActivity
	Err             error
}

// JobActivity is what a job has done lately, newest first: the Runs of an adapter job, the child Issues a native job made.
type JobActivity struct {
	Runs     []Run
	Children []CronChild
}

type CronChild struct {
	UID, ShortID, Title, Status string
	ScheduledAt, CreatedAt      time.Time
}

// When is the run the child was made for, else when it was made.
func (c CronChild) When() time.Time {
	if c.ScheduledAt.IsZero() {
		return c.CreatedAt
	}
	return c.ScheduledAt
}

// Words is the child as a row words it: "#ab12 Tick — 2026… · open".
func (c CronChild) Words() string {
	return "#" + c.ShortID + " " + c.Title + " · " + c.Status
}

// JobActivityShown is how many recent runs or children a job keeps and the inspector lists.
const JobActivityShown = 5

func (JobsLoaded) msg() {}

type JobSelectMsg struct{ UID string }

func (JobSelectMsg) msg() {}

type JobOpenMsg struct{ UID string }

func (JobOpenMsg) msg() {}

type JobsState struct {
	// ProjectUID is the Project the state was filled for; resetPagesForProject empties it when the Pane's Project differs.
	ProjectUID string
	Jobs       []CronJob
	// ProjectID is the Project Jobs belongs to; zero before a read was asked for.
	ProjectID       int64
	ProjectArchived bool
	ShowArchived    bool
	Selected        string
	Activity        map[string]JobActivity
	Loading         bool
	Err             error
	// Writing is the uid of the job a pause, resume, archive or restore is writing; the keys wait for its answer.
	Writing string
	// reads numbers the reads asked for; stale asks for another.
	reads Latest
	stale bool
	// pick is the job the next read selects: the one an Issue detail's Cron job property opened.
	pick string
	// cronRead is what the Issue detail last read the list for; rev counts the answers, for the detail's rows.
	cronRead cronRead
	rev      int64
	// rates are the listed jobs' IssuesPerDay, counted once per job revision. A cron trigger's count walks every occurrence
	// of a week (a minutely job's 10,080 take about 17ms), too slow for every draw of the Page; see withRates.
	rates map[jobRevision]int
}

// jobRevision is one revision of a job: what its rate is counted for.
type jobRevision struct {
	uid      string
	revision int64
}

// withRates counts the rate of every listed job whose revision has none yet, at m.Now, and forgets the rest. A rate counted
// over the week from an earlier read stays: the Page says "about" so many Issues a day, which a few hours do not move.
func (m Model) withRates() Model {
	rates := make(map[jobRevision]int, len(m.Jobs.Jobs))
	for _, job := range m.Jobs.Jobs {
		key := jobRevision{job.UID, job.Revision}
		if perDay, ok := m.Jobs.rates[key]; ok {
			rates[key] = perDay
		} else {
			rates[key] = job.IssuesPerDay(m.Now)
		}
	}
	m.Jobs.rates = rates
	return m
}

// highRateWarning is the line the Jobs Page shows for a job that makes more than 24 Issues a day, "" for any other. A job
// withRates has not counted is counted from now.
func (m Model) highRateWarning(job CronJob, now time.Time) string {
	perDay, ok := m.Jobs.rates[jobRevision{job.UID, job.Revision}]
	if !ok {
		perDay = job.IssuesPerDay(now)
	}
	return cronjob.HighRateWarning(perDay)
}

func (m Model) OnJobs() bool { return m.Page == PageJobs }

// JobRow is one job as the Jobs Page's list draws it. Schedule and Next are empty where they can't be worked out.
type JobRow struct {
	Job      CronJob
	Pause    PauseReason
	State    string
	Kind     string
	Schedule string
	Profile  string
	Next     string
	// Unsupported is why this machine's worker sets the job aside, empty when it runs it or serves another Project.
	Unsupported string
	// WorkerPause is why this machine's worker admits none of the job's runs, empty when it does.
	WorkerPause string
	// LastStatus is the status of the job's latest run, empty when its latest activity is a child Issue or there is none. Last is
	// the rest of the line: for a run its start, then its Brief; for a child the whole line, "Last Issue #ab12 Tick · open · 18:00".
	LastStatus cronjob.RunStatus
	Last       string
	Warning    string
}

// HiddenArchivedJobs is how many archived jobs the list holds but does not show.
func (m Model) HiddenArchivedJobs() int {
	if m.Jobs.ShowArchived {
		return 0
	}
	n := 0
	for _, job := range m.Jobs.Jobs {
		if job.Archived() {
			n++
		}
	}
	return n
}

func (m Model) visibleJobs() []CronJob {
	jobs := m.Jobs.Jobs
	if !m.Jobs.ShowArchived {
		jobs = slices.DeleteFunc(slices.Clone(jobs), CronJob.Archived)
	}
	if !m.searchingPage(PageJobs) {
		return jobs
	}
	return slices.DeleteFunc(slices.Clone(jobs), func(job CronJob) bool { return !m.matchesSearch(m.jobSearchWords(job)...) })
}

// JobRows are the Jobs Page's rows in kata's order, name then uid. now is when the next runs are counted from.
func (m Model) JobRows(now time.Time) []JobRow {
	jobs := m.visibleJobs()
	rows := make([]JobRow, len(jobs))
	for i, job := range jobs {
		pause := m.JobPause(job)
		row := JobRow{Job: job, Pause: pause, State: pause.StateText(), Kind: job.Kind(), Schedule: triggerWords(job.Definition.Trigger), Profile: job.Definition.Profile, Unsupported: m.workerUnsupported(job.UID), WorkerPause: m.workerPauseWords(job.UID)}
		if next, ok := job.NextRun(now, m.Jobs.ProjectArchived); ok && pause != PauseFailedRuns {
			row.Next = m.Calendar.Read(next, now)
		}
		row.LastStatus, row.Last = m.lastActivityParts(job, now)
		if !job.Archived() {
			row.Warning = m.highRateWarning(job, now)
		}
		rows[i] = row
	}
	return rows
}

// lastActivityParts is the job's latest run or child as a status and the rest of the line, both empty when none is on record.
func (m Model) lastActivityParts(job CronJob, now time.Time) (cronjob.RunStatus, string) {
	activity := m.Jobs.Activity[job.UID]
	switch {
	case len(activity.Runs) > 0:
		return activity.Runs[0].Status, m.runWords(activity.Runs[0], now)
	case len(activity.Children) > 0:
		c := activity.Children[0]
		return "", "Last Issue " + c.Words() + " · " + m.Calendar.Read(c.When(), now)
	}
	return "", ""
}

// lastActivity is the job's latest run or child in one line, "" when none is on record; the Page search reads it.
func (m Model) lastActivity(job CronJob, now time.Time) string {
	status, rest := m.lastActivityParts(job, now)
	if status == "" {
		return rest
	}
	return "last run " + string(status) + " · " + rest
}

// runWords is a Run as the Jobs Page words it after its status: when it started, then its summary's Brief.
func (m Model) runWords(r Run, now time.Time) string {
	words := m.Calendar.Read(r.Started(), now)
	if brief := r.Brief(); brief != "" {
		words += " · " + brief
	}
	return words
}

func (m Model) SelectedJob() *CronJob {
	for _, job := range m.visibleJobs() {
		if job.UID == m.Jobs.Selected {
			return &job
		}
	}
	return nil
}

// CurrentSelection is the uid of the selected row of the current Page's list: a job on the Jobs Page, else an Issue.
func (m Model) CurrentSelection() string {
	if m.OnConfig() {
		if f := m.SelectedConfigFile(); f != nil {
			return string(f.Key)
		}
		return ""
	}
	if m.OnDaemons() {
		if e := m.SelectedDaemon(); e != nil {
			return e.Name
		}
		return ""
	}
	if m.OnFederation() {
		switch m.Federation.Screen {
		case FederationPreview, FederationRecovery, FederationEnrollDone:
			// These screens draw no row of a list.
			return ""
		}
		if m.FederationChoosing() {
			if _, n := m.federationCursorCount(); n > 0 {
				return "choice" + strconv.Itoa(m.FederationChoiceCursor())
			}
			return ""
		}
		if _, noticed := m.Federation.Notice(); noticed && m.Federation.Screen == FederationList {
			// The list is not drawn while a notice stands in its place.
			return ""
		}
		if s := m.SelectedSpoke(); s != nil {
			return s.ProjectName
		}
		return ""
	}
	if m.OnAttention() {
		return m.Attention.Selected
	}
	if m.OnCredentials() {
		if c := m.SelectedCredential(); c != nil {
			return strconv.FormatInt(c.ID, 10)
		}
		return ""
	}
	if m.OnRuns() {
		return m.Runs.Selected
	}
	if m.OnJobs() {
		return m.Jobs.Selected
	}
	if m.OnWorkflows() {
		return m.Workflows.Selected
	}
	return m.SelectedUID
}

// JobFact is one line of the inspector. Status, when set, is a run status drawn in its tone before Value.
type JobFact struct {
	Label, Value string
	Parent       bool
	Status       cronjob.RunStatus
}

// JobFacts are the inspector's lines for the selected job: its state, what it does and when, the Issue policy, and for a
// native job its parent with the delivery progress. Lines that don't apply to the job are left out. Ids are JobIDs.
func (m Model) JobFacts(now time.Time) []JobFact {
	job := m.SelectedJob()
	if job == nil {
		return nil
	}
	d, t := job.Definition, job.Definition.Trigger
	var facts []JobFact
	add := func(label, value string) {
		if value != "" {
			facts = append(facts, JobFact{Label: label, Value: value})
		}
	}
	addList := func(label string, values []string, status func(i int) cronjob.RunStatus) {
		for i, v := range values {
			if i > 0 {
				label = ""
			}
			fact := JobFact{Label: label, Value: v}
			if status != nil {
				fact.Status = status(i)
			}
			facts = append(facts, fact)
		}
	}
	pause := m.JobPause(*job)
	add("State", pause.StateText())
	add("Worker", m.workerPauseWords(job.UID))
	add("Set aside", m.workerUnsupported(job.UID))
	add("Kind", job.Kind())
	schedule := triggerWords(t)
	add("Schedule", schedule)
	if runs, reason := m.NextRuns(*job, now); reason != "" {
		add("Next runs", strings.TrimPrefix(reason, "none: "))
	} else {
		addList("Next runs", runs, nil)
	}
	if !job.Archived() {
		add("Warning", m.highRateWarning(*job, now))
	}
	activity := m.Jobs.Activity[job.UID]
	runs := activity.Runs[:min(len(activity.Runs), JobActivityShown)]
	recent := make([]string, len(runs))
	for i, r := range runs {
		recent[i] = m.runWords(r, now)
	}
	addList("Recent runs", recent, func(i int) cronjob.RunStatus { return runs[i].Status })
	recent = nil
	for _, c := range activity.Children[:min(len(activity.Children), JobActivityShown)] {
		recent = append(recent, c.Words()+" · "+m.Calendar.Read(c.When(), now))
	}
	addList("Recent children", recent, nil)
	if schedule == "" { // a schedule that cannot be worded shows the fields it is made of
		switch t.Kind {
		case cronjob.TriggerCron:
			zone, expression := cronjob.Zone(t.Cron, t.Timezone)
			add("Cron", expression)
			add("Time zone", zone)
		case cronjob.TriggerInterval:
			add("Interval", strconv.FormatInt(t.IntervalSeconds, 10)+" s")
			add("Time zone", t.Timezone)
		case cronjob.TriggerOnce:
			add("At", t.At)
		}
	}
	add("Prompt", d.Action.Prompt)
	if d.Action.WorkflowUID != "" {
		add("Workflow", m.workflowName(d.Action.WorkflowUID))
	}
	recipient := d.Action.Recipient
	if recipient == recipientDefault {
		recipient = "The Issue's owner or author"
	}
	add("Recipient", recipient)
	add("Message", d.Action.Message)
	add("Adapter", job.Adapter())
	add("Profile", d.Profile)
	add("Checkout", d.CheckoutKey)
	if len(d.SecretRefs) > 0 {
		names := make([]string, 0, len(d.SecretRefs))
		for name := range d.SecretRefs {
			names = append(names, name)
		}
		slices.Sort(names)
		add("Secrets", strings.Join(names, ", "))
	}
	if p := d.Issue; p != nil {
		add("Issue", map[cronjob.IssuePolicyKind]string{cronjob.IssuePerRun: "New Issue per run", cronjob.IssueExisting: "Existing Issue"}[p.Kind])
		add("Child title", p.Title)
		add("Child body", p.Body)
		if p.ScheduledOffsetSeconds != nil {
			add("Scheduled offset", durationWords(*p.ScheduledOffsetSeconds, false))
		}
		if p.DeadlineOffsetSeconds != nil {
			add("Deadline offset", durationWords(*p.DeadlineOffsetSeconds, false))
		}
	}
	if d.TimeoutSeconds > 0 {
		add("Timeout", durationWords(d.TimeoutSeconds, false))
	}
	add("Overlap", OverlapLabel(d.Overlap))
	catchup := CatchupLabel(d.Catchup)
	if d.GraceSeconds > 0 {
		catchup += " · grace " + durationWords(d.GraceSeconds, false)
	}
	add("Catch up", catchup)
	if p := job.Parent; p != nil {
		ref, status := p.UID, p.Status
		if p.ShortID != "" {
			ref = "#" + p.ShortID + " " + p.Title
		}
		if p.Deleted {
			status = "deleted"
		}
		facts = append(facts, JobFact{Label: "Parent", Value: ref + " · " + status, Parent: true})
		if p.Progress != nil {
			add("Delivered through", m.Calendar.Stamp(p.Progress.Through))
		}
		if missed := job.Missed(now); missed > 0 {
			add("Missed", strconv.Itoa(missed))
		}
	}
	created := m.Calendar.Read(job.CreatedAt, now)
	if job.Author != "" {
		created += " · " + job.Author
	}
	add("Created", created)
	add("Updated", m.Calendar.Read(job.UpdatedAt, now))
	return facts
}

// Unreachable says err is a transport failure: kata did not answer, as opposed to answering with a refusal.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	for _, marker := range []string{"error sending request", "error executing request", "connection refused", "kata daemon start", "dial tcp", "no such host", "i/o timeout"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// LoadFailure is the line for a failed read of thing: a transport failure reads "kata is not reachable", else kata's own words.
func LoadFailure(thing string, err error) string {
	if Unreachable(err) {
		return "Cannot load " + thing + ": kata is not reachable."
	}
	return Cannot("load", thing, err)
}

// workflowName is the name of the workflow with uid when the Workflows Page has read it, else empty.
func (m Model) workflowName(uid string) string {
	for _, w := range m.Workflows.List {
		if w.UID == uid {
			return w.Name
		}
	}
	return ""
}

// JobIDs are the ids of the selected job, for the hovercard of the inspector's title.
func (m Model) JobIDs() []IDFact {
	job := m.SelectedJob()
	if job == nil {
		return nil
	}
	var ids []IDFact
	add := func(label, value string) {
		if value != "" {
			ids = append(ids, IDFact{label, value})
		}
	}
	add("Job", job.UID)
	add("Version", job.DefinitionEventUID)
	add("Workflow", job.Definition.Action.WorkflowUID)
	add("Trigger Issue", job.Definition.Trigger.IssueUID)
	if p := job.Definition.Issue; p != nil && p.Kind == cronjob.IssueExisting {
		add("Existing Issue", p.UID)
	}
	return ids
}

// jobsProjectID is the id of the Project the Jobs Page shows: the Project Collection's, from the catalog, else the Binding's.
func (m Model) jobsProjectID() (int64, bool) {
	if !m.Collection.IsProject() {
		return 0, false
	}
	if entry, ok := CatalogEntry(m.Snapshot, m.Collection.ProjectUID); ok {
		return entry.Project.ID, true
	}
	if m.Bound && m.Project.UID == m.Collection.ProjectUID {
		return m.Project.ID, true
	}
	return 0, false
}

// enterProjectPage is the entry rule of a Page that acts on the Pane's Project: the Project Collection if that is current,
// else the Binding's Project, which becomes the current Collection. With neither it says so and opens the Menu's Projects
// entry, and reports false. name is the Page as the notice words it.
func enterProjectPage(m Model, name string) (Model, []Cmd, bool) {
	var cmds []Cmd
	switch {
	case m.Collection.IsProject():
	case m.Bound:
		m.Collection = ProjectCollection(m.Project.UID, m.Project.Name)
		m = m.syncSidebarCursor()
		m.Loading, m.Err = true, nil
		m, cmds = collectionSwitched(m)
	default:
		m = m.fail("Cannot open the " + name + ": it needs a Project.")
		m, cmds = openProjects(m)
		return m, cmds, false
	}
	return m.resetPagesForProject(), cmds, true
}

// clearJobs drops the Jobs Page's loading marker and the Issue detail's read of its job list when the Page is left.
func clearJobs(m Model) Model {
	m.Jobs.Loading = false
	m.Jobs.cronRead = cronRead{}
	return m
}

// settleJobs keeps the Jobs Page consistent: it ends without a Project Collection, and it reads the Project's jobs when it has
// none for the Project in view or the last read went stale.
func (m Model) settleJobs() (Model, []Cmd) {
	if !m.OnJobs() {
		return m, nil
	}
	if !m.Collection.IsProject() {
		return m.leavePage(), nil
	}
	id, ok := m.jobsProjectID()
	if !ok {
		return m, nil
	}
	if id != m.Jobs.ProjectID {
		m.Jobs.ProjectID, m.Jobs.stale = id, true
	}
	if !m.Jobs.stale {
		return m, nil
	}
	m.Jobs.stale, m.Jobs.Loading = false, true
	return m, []Cmd{LoadJobs{Workspace: m.Workspace, ProjectID: id, Archived: m.Jobs.ShowArchived, Request: m.Jobs.reads.Ask(), Activity: true}}
}

// jobsLoaded applies the answer to the latest read. A Project that is archived keeps the jobs already held: they are what is
// paused.
func jobsLoaded(m Model, msg JobsLoaded) Model {
	if !m.Jobs.reads.IsCurrent(msg.Request) {
		return m
	}
	m.Jobs.Loading = false
	m.Jobs.rev++
	m.Jobs.Err = msg.Err
	if msg.Err != nil {
		m.Jobs.pick = ""
		return m.settleJobSelection()
	}
	m.Jobs.ProjectArchived = msg.ProjectArchived
	if !msg.ProjectArchived {
		m.Jobs.Jobs = msg.Jobs
		m = m.withRates()
		if msg.Activity != nil {
			m.Jobs.Activity = msg.Activity
		}
	}
	if m.Jobs.pick != "" && slices.ContainsFunc(m.visibleJobs(), func(j CronJob) bool { return j.UID == m.Jobs.pick }) {
		m.Jobs.Selected = m.Jobs.pick
	}
	m.Jobs.pick = ""
	return m.settleJobSelection()
}

func (m Model) settleJobSelection() Model {
	keepSelected(m.visibleJobs(), func(j CronJob) string { return j.UID }, &m.Jobs.Selected)
	return m
}

func moveJob(m Model, action Action) Model {
	moveSelected(m.visibleJobs(), func(j CronJob) string { return j.UID }, &m.Jobs.Selected, listPage, action)
	return m
}

// toggleArchivedJobs shows or hides the archived jobs; the list is read again because kata lists them only when asked.
func toggleArchivedJobs(m Model) Model {
	m.Jobs.ShowArchived = !m.Jobs.ShowArchived
	m.Jobs.stale = true
	return m.settleJobSelection()
}

// reloadJobs is also the Jobs Page's own work on entry (see switchPage): the Page keeps its selection while another Page is
// shown, and the jobs are read again as soon as the Project's id is known.
func reloadJobs(m Model) (Model, []Cmd) {
	m.Jobs.stale = true
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd, LoadWorkerSetup{Workspace: m.Workspace}}
}

func focusInspector(m Model) Model {
	if m.SelectedJob() != nil {
		m.Focus = ColumnDetail
	}
	return m
}

func inspectorBack(m Model) Model {
	m.Focus = ColumnList
	return m
}

// openJobParent is enter in the inspector: a native job's parent Issue opens on the Issues Page, with its detail focused. A
// parent that is deleted can't be opened, and an adapter job has none.
func openJobParent(m Model) Model {
	job := m.SelectedJob()
	switch {
	case job == nil || job.Parent == nil:
		return m
	case job.Parent.Deleted:
		return m.fail("Cannot open the parent Issue: it is deleted.")
	}
	uid := job.Parent.UID
	m = m.leavePage()
	if m.SelectedUID == uid {
		return m.focusDetail()
	}
	return m.jumpWithin(uid, nil)
}

// updateJobs applies the Jobs Page's results and selections, and those of its card. A read numbered below zero is the Workflows
// Page's, which that Page takes.
func updateJobs(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case JobsLoaded:
		if msg.Request < 0 {
			return m, nil, false
		}
		next, cmds := backfillJobsRead(jobsLoaded(m, msg), msg)
		return next, cmds, true
	case JobWritten:
		if next, cmds, ok := backfillWritten(m, msg); ok {
			return next, cmds, true
		}
		return jobWritten(m, msg), nil, true
	case WorkerPickersLoaded:
		return workerPickersLoaded(m, msg), nil, true
	case JobWorkflowsLoaded:
		return jobWorkflowsLoaded(m, msg), nil, true
	case OpenJobMsg:
		next, cmds := openJob(m, msg.UID)
		return next, cmds, true
	case JobSelectMsg:
		m.Jobs.Selected, m.Focus = msg.UID, ColumnList
		return m, nil, true
	case JobOpenMsg:
		if mode := m.CurrentMode(); (mode == ModeJobs || mode == ModeJobInspector) && slices.ContainsFunc(m.visibleJobs(), func(j CronJob) bool { return j.UID == msg.UID }) {
			m.Jobs.Selected, m.Focus = msg.UID, ColumnList
			m = focusInspector(m)
		}
		return m, nil, true
	}
	return m, nil, false
}
