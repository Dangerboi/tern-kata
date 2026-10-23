package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
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
	// Activity is what each job has done lately, by the job's uid, when the read asked for it.
	Activity map[string]JobActivity
	Err      error
}

// JobActivity is what a job has done lately, newest first: the Runs of an adapter job, the child Issues a native job made.
type JobActivity struct {
	Runs     []Run
	Children []CronChild
}

// CronChild is a child Issue a native job made, read from its parent's children by their cron.occurrence metadata.
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

// JobSelectMsg is a click on a job's row.
type JobSelectMsg struct{ UID string }

func (JobSelectMsg) msg() {}

// JobsState is the Jobs Page's state. It is not saved: a Pane starts on the Issues Page.
type JobsState struct {
	// ProjectUID is the Project the state was filled for; resetPagesForProject empties it when the Pane's Project differs.
	ProjectUID string
	// Jobs are the Project's jobs as last read, archived ones only when ShowArchived asked for them.
	Jobs []CronJob
	// ProjectID is the Project Jobs belongs to; zero before a read was asked for.
	ProjectID int64
	// ProjectArchived is what the last read found of the Project.
	ProjectArchived bool
	// ShowArchived says archived jobs are listed.
	ShowArchived bool
	// Selected is the uid of the job the inspector shows.
	Selected string
	// Activity is what each job has done lately, as the last read found it.
	Activity map[string]JobActivity
	Loading  bool
	Err      error
	// Writing is the uid of the job a pause, resume, archive or restore is writing; the keys wait for its answer.
	Writing string
	// request counts the reads asked for; stale asks for another.
	request int64
	stale   bool
	// detailHidden is the detail column's hidden flag from before the Jobs Page took its place.
	detailHidden bool
	// pick is the job the next read selects: the one an Issue detail's Cron job property opened.
	pick string
	// cronRead is what the Issue detail last read the list for; rev counts the answers, for the detail's rows.
	cronRead cronRead
	rev      int64
}

// OnJobs says the Jobs Page is current.
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
	// Last is the job's latest run or child in words, empty when it has none on record.
	Last string
	// Warning is the high-rate warning of a job that makes more than 24 Issues a day, empty for any other.
	Warning string
}

// visibleJobs are the jobs the list shows.
func (m Model) visibleJobs() []CronJob {
	if m.Jobs.ShowArchived {
		return m.Jobs.Jobs
	}
	return slices.DeleteFunc(slices.Clone(m.Jobs.Jobs), CronJob.Archived)
}

// JobRows are the Jobs Page's rows in kata's order, name then uid. now is when the next runs are counted from.
func (m Model) JobRows(now time.Time) []JobRow {
	jobs := m.visibleJobs()
	rows := make([]JobRow, len(jobs))
	for i, job := range jobs {
		pause := job.Pause(m.Jobs.ProjectArchived)
		row := JobRow{Job: job, Pause: pause, State: pause.StateText(), Kind: job.Kind(), Schedule: job.Definition.Trigger.Words(), Profile: job.Definition.Profile, Unsupported: m.workerUnsupported(job.UID)}
		if next, ok := job.NextRun(now, m.Jobs.ProjectArchived); ok {
			row.Next = m.Calendar.Read(next, now)
		}
		row.Last = m.lastActivity(job, now)
		if !job.Archived() {
			row.Warning = job.HighRateWarning(now)
		}
		rows[i] = row
	}
	return rows
}

// lastActivity is the job's latest run or child in words, "" when none is on record.
func (m Model) lastActivity(job CronJob, now time.Time) string {
	activity := m.Jobs.Activity[job.UID]
	switch {
	case len(activity.Runs) > 0:
		return "last run " + m.runWords(activity.Runs[0], now)
	case len(activity.Children) > 0:
		c := activity.Children[0]
		return "last child " + c.Words() + " · " + m.Calendar.Read(c.When(), now)
	}
	return ""
}

// runWords is a Run in a line, as the Jobs Page words it: its status, when it started, and the first line of its summary.
func (m Model) runWords(r Run, now time.Time) string {
	words := string(r.Status) + " · " + m.Calendar.Read(r.Started(), now)
	if first, _, _ := strings.Cut(r.Message, "\n"); strings.TrimSpace(first) != "" {
		words += " · " + strings.TrimSpace(first)
	}
	return words
}

// SelectedJob is the job the inspector shows: the selected one if the list shows it, else nil.
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
	if m.OnRuns() {
		return m.Runs.Selected
	}
	if m.OnJobs() {
		return m.Jobs.Selected
	}
	return m.SelectedUID
}

// JobFact is one line of the inspector. Parent marks the line Enter opens.
type JobFact struct {
	Label, Value string
	Parent       bool
}

// JobFacts are the inspector's lines for the selected job: its state, what it does and when, the Issue policy, and for a
// native job its parent with the delivery progress. Lines that don't apply to the job are left out.
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
	pause := job.Pause(m.Jobs.ProjectArchived)
	add("Name", job.Name)
	add("State", pause.StateText())
	add("Set aside", m.workerUnsupported(job.UID))
	add("Kind", job.Kind())
	add("Schedule", t.Words())
	if next, ok := job.NextRun(now, m.Jobs.ProjectArchived); ok {
		add("Next run", m.Calendar.Stamp(next))
	}
	if !job.Archived() {
		add("Warning", job.HighRateWarning(now))
	}
	activity := m.Jobs.Activity[job.UID]
	for i, r := range activity.Runs[:min(len(activity.Runs), JobActivityShown)] {
		label := ""
		if i == 0 {
			label = "Recent runs"
		}
		facts = append(facts, JobFact{Label: label, Value: m.runWords(r, now)})
	}
	for i, c := range activity.Children[:min(len(activity.Children), JobActivityShown)] {
		label := ""
		if i == 0 {
			label = "Recent children"
		}
		facts = append(facts, JobFact{Label: label, Value: c.Words() + " · " + m.Calendar.Read(c.When(), now)})
	}
	switch t.Kind {
	case TriggerCron:
		zone, expression := t.zoneAndExpression()
		add("Cron", expression)
		add("Time zone", zone)
	case TriggerInterval:
		add("Interval", strconv.FormatInt(t.IntervalSeconds, 10)+" s")
		add("Time zone", t.Timezone)
	case TriggerOnce:
		add("At", t.At)
	case TriggerIssueScheduled, TriggerIssueDeadline:
		add("Trigger Issue", t.IssueUID)
	}
	add("Prompt", d.Action.Prompt)
	add("Workflow", d.Action.WorkflowUID)
	add("Recipient", d.Action.Recipient)
	add("Message", d.Action.Message)
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
		add("Issue", map[string]string{IssuePerRun: "a new Issue per run", IssueExisting: "an existing Issue"}[p.Kind])
		add("Existing Issue", p.UID)
		add("Child title", p.Title)
		add("Child body", p.Body)
		if p.ScheduledOffsetSeconds != nil {
			add("Scheduled offset", strconv.FormatInt(*p.ScheduledOffsetSeconds, 10)+" s")
		}
		if p.DeadlineOffsetSeconds != nil {
			add("Deadline offset", strconv.FormatInt(*p.DeadlineOffsetSeconds, 10)+" s")
		}
	}
	add("Overlap", d.Overlap)
	catchup := d.Catchup
	if d.GraceSeconds > 0 {
		catchup += fmt.Sprintf(", grace %d s", d.GraceSeconds)
	}
	add("Catchup", catchup)
	if d.TimeoutSeconds > 0 {
		add("Timeout", strconv.FormatInt(d.TimeoutSeconds, 10)+" s")
	}
	if p := job.Parent; p != nil {
		ref, status := p.UID, p.Status
		if p.ShortID != "" {
			ref = "#" + p.ShortID + " " + p.Title
		}
		if p.Deleted {
			status = "deleted"
		}
		value := ref + " · " + status
		facts = append(facts, JobFact{Label: "Parent", Value: value, Parent: true})
		if p.Progress != nil {
			add("Anchor", m.Calendar.Stamp(p.Progress.Anchor))
			add("Delivered through", m.Calendar.Stamp(p.Progress.Through))
		}
		if missed := job.Missed(now); missed > 0 {
			add("Missed", strconv.Itoa(missed))
		}
	}
	add("Author", job.Author)
	add("Revision", strconv.FormatInt(job.Revision, 10))
	add("Created", m.Calendar.Stamp(job.CreatedAt))
	add("Updated", m.Calendar.Stamp(job.UpdatedAt))
	add("UID", job.UID)
	return facts
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
		m.Notice = "The " + name + " needs a Project: choose one in the Menu."
		return openProjects(m), nil, false
	}
	return m.resetPagesForProject(), cmds, true
}

// enterJobs is the Jobs Page's entry. The jobs load as soon as the Project's id is known.
func enterJobs(m Model) (Model, []Cmd) {
	m, cmds, ok := enterProjectPage(m, "Jobs Page")
	if !ok {
		return m, cmds
	}
	m.Page = PageJobs
	m.Jobs.detailHidden = m.DetailHidden()
	m.hidden = true
	m.Focus = ColumnList
	// The Page keeps its selection while another Page is shown; the jobs are read again.
	m.Jobs.stale = true
	m, ask := m.askWorker(0)
	return m, append(cmds, ask, LoadWorkerSetup{Workspace: m.Workspace})
}

// jobsOff leaves the Jobs Page for the Issues Page, the keys on the list, and gives the detail column back.
func (m Model) jobsOff() Model {
	m.Page = PageIssues
	if m.Focus == ColumnDetail {
		m.Focus = ColumnList
	}
	m.hidden = m.Jobs.detailHidden
	m.Jobs.Loading = false
	m.Jobs.cronRead = cronRead{}
	return m.revealInList(m.SelectedUID).WithVisibleSelection()
}

// settleJobs keeps the Jobs Page consistent: it ends without a Project Collection, and it reads the Project's jobs when it has
// none for the Project in view or the last read went stale.
func (m Model) settleJobs() (Model, []Cmd) {
	if !m.OnJobs() {
		return m, nil
	}
	if !m.Collection.IsProject() {
		return m.jobsOff(), nil
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
	m.Jobs.request++
	return m, []Cmd{LoadJobs{Workspace: m.Workspace, ProjectID: id, Archived: m.Jobs.ShowArchived, Request: m.Jobs.request, Activity: true}}
}

// jobsLoaded applies the answer to the latest read. A Project that is archived keeps the jobs already held: they are what is
// paused.
func jobsLoaded(m Model, msg JobsLoaded) Model {
	if msg.Request != m.Jobs.request {
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

// settleJobSelection keeps the selection on a listed job, the first when it is gone.
func (m Model) settleJobSelection() Model {
	jobs := m.visibleJobs()
	if slices.ContainsFunc(jobs, func(j CronJob) bool { return j.UID == m.Jobs.Selected }) {
		return m
	}
	m.Jobs.Selected = ""
	if len(jobs) > 0 {
		m.Jobs.Selected = jobs[0].UID
	}
	return m
}

// moveJob moves the selection by action: one row, to either end, or a page.
func moveJob(m Model, action Action) Model {
	jobs := m.visibleJobs()
	if len(jobs) == 0 {
		return m
	}
	at := slices.IndexFunc(jobs, func(j CronJob) bool { return j.UID == m.Jobs.Selected })
	m.Jobs.Selected = jobs[movedIndex(at, len(jobs), listPage, action)].UID
	return m
}

// toggleArchivedJobs shows or hides the archived jobs; the list is read again because kata lists them only when asked.
func toggleArchivedJobs(m Model) Model {
	m.Jobs.ShowArchived = !m.Jobs.ShowArchived
	m.Jobs.stale = true
	return m.settleJobSelection()
}

// reloadJobs reads the jobs again.
func reloadJobs(m Model) (Model, []Cmd) {
	m.Jobs.stale = true
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd, LoadWorkerSetup{Workspace: m.Workspace}}
}

// focusInspector is enter on the job list: the inspector takes the keys.
func focusInspector(m Model) Model {
	if m.SelectedJob() != nil {
		m.Focus = ColumnDetail
	}
	return m
}

// inspectorBack is esc in the inspector: the keys go back to the list.
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
		m.Notice = "The parent Issue is deleted."
		return m
	}
	uid := job.Parent.UID
	m = m.jobsOff()
	if m.SelectedUID == uid {
		// The detail was not kept fresh while the Jobs Page hid it.
		m.stale = true
		return m.focusDetail()
	}
	return m.jumpWithin(uid, nil)
}
