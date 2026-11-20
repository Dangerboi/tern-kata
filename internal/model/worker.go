package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// This machine's worker is a separate process the Pane reaches over its control socket. The Pane asks it for its status, for the
// local state of the runs it executes (held runs with their reason, steps, the tail of the agent's output) and sends it the
// verbs run now, resume and abandon. Kata holds a Run's record; the worker alone knows what kata does not: why a run is held and
// what the agent has said so far. Only runs whose executor label is the worker's are shown with local state.

const workerPollEvery = 2 * time.Second

// LoadWorker asks this machine's worker for its status, After a pause when After is set. Request numbers the asks: an answer to
// an older one is dropped.
type LoadWorker struct {
	Request int64
	After   time.Duration
}

func (LoadWorker) cmd() {}

type WorkerProject struct {
	Name      string
	Admitting bool
	// Why is what stops it admitting, in words: its lease is held by another worker, a pause, and so on.
	Why             string
	InUse, Capacity int
	// Unsupported is why the worker sets each job aside, by job uid: a profile, checkout or secret worker.toml does not map, a
	// workflow it does not run, and so on.
	Unsupported map[string]string
	// Paused is how many failed runs in a row made the worker stop admitting each job, by job uid, until it is resumed.
	Paused map[string]int
	// Skipped is each job's recent skips, one entry per job, as the worker lists them (Only when conditions).
	Skipped []JobSkips
}

// JobSkips is one job's recent skips as the worker reports them: how many occurrences it skipped and the latest, by the condition
// that said no and its reason. Skips are not kata Runs; only the worker knows them.
type JobSkips struct {
	UID, Name string
	Count     int
	// LatestCondition is the condition's name (runs_per_day, new_commits and so on) and LatestReason its words; LatestAt is when
	// the worker skipped the newest occurrence.
	LatestCondition, LatestReason string
	LatestAt                      time.Time
}

// SkipRows are the Runs Page's skip rows: the jobs this machine's worker skipped occurrences of, by job name. Empty when no worker
// status is readable for the Project, narrowed by the job filter and the page search like the Runs.
func (m Model) SkipRows() []JobSkips {
	p := m.workerProject()
	if p == nil {
		return nil
	}
	var rows []JobSkips
	for _, s := range p.Skipped {
		if m.Runs.JobUID != "" && s.UID != m.Runs.JobUID {
			continue
		}
		if m.searchingPage(PageRuns) && !m.matchesSearch(s.Name, s.LatestCondition, s.LatestReason) {
			continue
		}
		rows = append(rows, s)
	}
	return rows
}

type LocalRun struct {
	UID, Project, Job, Issue string
	// State is running, resuming, held: <reason>, succeeded, failed, cancelled; Pending is what is still unreported to kata.
	State, Pending string
	Attempt        int
	Held           bool
	// Hold is the reason of a held run and Detail what the worker knew when it held it.
	Hold, Detail string
	// Resumable and Abandonable say which verbs the worker would accept now; ResumeRefusal is why a resume would not.
	Resumable     bool
	ResumeRefusal string
	Abandonable   bool
}

func (r LocalRun) Settled() bool {
	switch r.State {
	case "succeeded", "failed", "cancelled", "unknown":
		return true
	}
	return false
}

type WorkerStatus struct {
	Executor string
	// PID is the worker's process id. Supervised says the plugin's host half started it and starts it again when it exits.
	PID        int
	Supervised bool
	// Stopping is "idle" while the worker drains toward an exit and "now" once it was told to end at once; InFlight is the runs
	// a drain waits for.
	Stopping string
	InFlight int
	Projects []WorkerProject
	Runs     []LocalRun
}

// WorkerLoaded answers LoadWorker. Running is false when nothing answers on the control socket; Err is any other failure.
type WorkerLoaded struct {
	Request int64
	Running bool
	Status  WorkerStatus
	Err     error
}

func (WorkerLoaded) msg() {}

type LoadLocalRun struct{ UID string }

func (LoadLocalRun) cmd() {}

type LocalStep struct {
	Index     int
	Kind      string
	Attempt   int
	State     string
	SessionID string
	ExitCode  *int
	Signal    string
	TimedOut  bool
}

type LocalRunDetail struct {
	LocalRun
	Summary                    string
	Steps                      []LocalStep
	OutputTail, StderrTail     string
	Cwd, SessionDir, SessionID string
}

type LocalRunLoaded struct {
	UID    string
	Detail LocalRunDetail
	Err    error
}

func (LocalRunLoaded) msg() {}

type RunJobNow struct{ JobUID, JobName string }

func (RunJobNow) cmd() {}

// JobRunRequested answers RunJobNow. Err is the worker's refusal, in words for the user.
type JobRunRequested struct {
	JobUID, JobName, RunUID string
	Err                     error
}

func (JobRunRequested) msg() {}

type ResumeLocalRun struct{ UID string }

func (ResumeLocalRun) cmd() {}

type AbandonLocalRun struct{ UID string }

func (AbandonLocalRun) cmd() {}

// LocalRunMoved answers ResumeLocalRun and AbandonLocalRun. State is the run's state as the worker saved it.
type LocalRunMoved struct {
	UID     string
	Resumed bool
	State   string
	Err     error
}

func (LocalRunMoved) msg() {}

// OpenRunSession asks for a new Tern tab in the run's checkout with its omp session resumed:
// tern new tab --cwd Cwd -- omp --resume SessionID --session-dir SessionDir.
type OpenRunSession struct{ Cwd, SessionDir, SessionID string }

func (OpenRunSession) cmd() {}

type RunSessionOpened struct{ Err error }

func (RunSessionOpened) msg() {}

type WorkerState struct {
	// Read says an answer arrived; Running that something answered on the control socket.
	Read, Running bool
	Err           error
	WorkerStatus
	// Setup is the last read of this machine's worker.toml and the Pane folder's Binding; SetupRead says an answer arrived and
	// SetupErr is a read that failed.
	Setup     WorkerSetup
	SetupRead bool
	SetupErr  error
	// Detail is the detail of the run last asked for; it is shown only while that run is selected.
	Detail *LocalRunDetail
	// reads numbers the status asks; asked is the run whose detail was asked for last and detailStale says a fresh status wants
	// it read again.
	reads       Latest
	inflight    bool
	polling     bool
	asked       string
	detailStale bool
}

func (m Model) askWorker(after time.Duration) (Model, Cmd) {
	m.Worker.inflight, m.Worker.polling = true, after > 0
	return m, LoadWorker{Request: m.Worker.reads.Ask(), After: after}
}

// workerLoaded applies the answer to the latest ask. On the Runs Page it asks again while a run is running, and when a pause
// ended it has the newest Runs read so the Run's own record follows what the worker reports.
func workerLoaded(m Model, msg WorkerLoaded) (Model, []Cmd) {
	if !m.Worker.reads.IsCurrent(msg.Request) {
		return m, nil
	}
	w := &m.Worker
	polled := w.polling
	w.inflight, w.polling = false, false
	w.Read, w.Running, w.Err = true, msg.Running, msg.Err
	w.WorkerStatus = msg.Status
	if !msg.Running {
		w.WorkerStatus, w.Detail, w.asked = WorkerStatus{}, nil, ""
	}
	w.detailStale = true
	if !m.OnRuns() || !m.runsRunning() {
		return m, nil
	}
	if polled && !m.Runs.Loading {
		m.Runs.refresh, m.Runs.stale = true, true
	}
	m, cmd := m.askWorker(workerPollEvery)
	return m, []Cmd{cmd}
}

func (m Model) runsRunning() bool {
	if !m.Worker.Running {
		return false
	}
	for _, r := range m.Worker.Runs {
		if !r.Settled() && !r.Held {
			return true
		}
	}
	return slices.ContainsFunc(m.Runs.Runs, func(r Run) bool { return r.Status == cronjob.RunRunning && m.IsLocalRun(r) })
}

func (m Model) IsLocalRun(r Run) bool {
	return m.Worker.Running && r.ExecutorLabel != "" && r.ExecutorLabel == m.Worker.Executor
}

// LocalRunOf is the worker's local state for the Run with uid, when the worker reports one: its detail if that is the run's, else
// its status line.
func (m Model) LocalRunOf(uid string) (LocalRun, bool) {
	if d := m.Worker.Detail; d != nil && d.UID == uid {
		return d.LocalRun, true
	}
	for _, r := range m.Worker.Runs {
		if r.UID == uid {
			return r, true
		}
	}
	return LocalRun{}, false
}

func (m Model) LocalDetail() *LocalRunDetail {
	if d := m.Worker.Detail; d != nil && d.UID == m.Runs.Selected {
		return d
	}
	return nil
}

// settleLocalRun reads the selected Run's local detail from the worker when it is this machine's and the detail is not held or a
// fresh status wants it read again.
func (m Model) settleLocalRun() (Model, []Cmd) {
	run := m.SelectedRun()
	if !m.OnRuns() || run == nil || !m.IsLocalRun(*run) {
		return m, nil
	}
	if m.Worker.asked == run.UID && !m.Worker.detailStale {
		return m, nil
	}
	m.Worker.asked, m.Worker.detailStale = run.UID, false
	return m, []Cmd{LoadLocalRun{UID: run.UID}}
}

func localRunLoaded(m Model, msg LocalRunLoaded) Model {
	if msg.Err != nil {
		if msg.UID == m.Runs.Selected {
			m = m.fail(Cannot("read", "the run from the worker", msg.Err))
		}
		return m
	}
	if msg.UID != m.Worker.asked {
		return m // a late answer for a run that is no longer the one asked for
	}
	m.Worker.Detail = &msg.Detail
	return m
}

func (m Model) workerProjectName() string {
	if m.Collection.IsProject() {
		return m.Collection.Title
	}
	return m.Project.Name
}

// workerProject is this Pane's Project in this machine's worker status, nil when the worker does not run or does not serve it.
func (m Model) workerProject() *WorkerProject {
	if !m.Worker.Running {
		return nil
	}
	name := m.workerProjectName()
	for i := range m.Worker.Projects {
		if m.Worker.Projects[i].Name == name {
			return &m.Worker.Projects[i]
		}
	}
	return nil
}

// workerUnsupported is why this machine's worker sets the job aside, empty when it runs it or does not serve the Project.
func (m Model) workerUnsupported(uid string) string {
	if p := m.workerProject(); p != nil {
		return p.Unsupported[uid]
	}
	return ""
}

// workerPausedJobs is how many failed runs in a row made this machine's worker pause the job, 0 when it did not or does not serve the Project.
func (m Model) workerPausedJobs(uid string) int {
	if p := m.workerProject(); p != nil {
		return p.Paused[uid]
	}
	return 0
}

// JobPause is why the job is not running as the Jobs Page words it: kata's own pause reason, else this machine's worker pausing
// it after repeated failed runs.
func (m Model) JobPause(job CronJob) PauseReason {
	if pause := job.Pause(m.Jobs.ProjectArchived); pause != NotPaused || m.workerPausedJobs(job.UID) == 0 {
		return pause
	}
	return PauseFailedRuns
}

// workerPauseWords is why this machine's worker admits none of the job's runs, empty when it does.
func (m Model) workerPauseWords(uid string) string {
	n := m.workerPausedJobs(uid)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("Paused after %d failed %s in a row", n, Noun(n, "run", ""))
}

// WorkerBanner is the Jobs Page's line about this machine's worker: a badge, then a muted detail. Good says the worker runs and
// serves this Pane's Project, Pending that one is about to start, Failed that the line is an error with no badge.
type WorkerBanner struct {
	Badge   string
	Good    bool
	Pending bool
	Failed  bool
	Detail  string
}

// Empty says there is nothing to show yet: the first answer has not arrived.
func (b WorkerBanner) Empty() bool { return b.Badge == "" && b.Detail == "" }

// Refusal is the banner as the reason a job cannot run now: a clause that ends the sentence "Cannot run "x" now: <reason>."; the clause holds no colon of its own.
func (b WorkerBanner) Refusal() string {
	switch {
	case b.Failed:
		return "reading the worker failed (" + strings.TrimSuffix(strings.TrimPrefix(b.Detail, "Cannot read the worker: "), ".") + ")"
	case b.Pending:
		return "the worker is starting"
	case b.Badge == "No worker":
		return "no worker (" + b.Detail + ")"
	}
	// A detail with its own reason ("admits nothing: paused") keeps it in brackets, so the sentence carries one colon.
	if what, why, ok := strings.Cut(b.Detail, ": "); ok {
		return "the worker " + what + " (" + why + ")"
	}
	return "the worker " + b.Detail
}

// WorkerBanner words this machine's worker for the Jobs Page: whether it runs and serves this Pane's Project, and what is missing
// when it does not. It is empty until the first answer arrives.
func (m Model) WorkerBanner() WorkerBanner {
	w := m.Worker
	project := m.workerProjectName()
	known := w.SetupRead && w.SetupErr == nil && w.Setup.Project == project
	switch {
	case !w.Read:
		return WorkerBanner{}
	case w.Err != nil:
		return WorkerBanner{Failed: true, Detail: Cannot("read", "the worker", w.Err)}
	case !w.Running && known:
		return m.stoppedWorkerBanner(project)
	case !w.Running:
		return WorkerBanner{Badge: "No worker", Detail: "none is running on this machine"}
	}
	badge := "Worker " + w.Executor
	manual := ""
	if w.SetupRead && w.SetupErr == nil && w.Setup.Manual {
		manual = " · supervision is off"
	}
	for _, p := range w.Projects {
		if p.Name != project {
			continue
		}
		if !p.Admitting {
			return WorkerBanner{Badge: badge, Detail: "admits nothing: " + p.Why + manual}
		}
		return WorkerBanner{Badge: badge, Good: true, Detail: fmt.Sprintf("%d of %d slots in use", p.InUse, p.Capacity) + manual}
	}
	if known && w.Setup.Lists {
		return WorkerBanner{Badge: badge, Detail: "does not serve " + project + "; restart it to pick up worker.toml" + manual}
	}
	return WorkerBanner{Badge: badge, Detail: "does not serve " + project + "; not listed in worker.toml" + manual}
}

// stoppedWorkerBanner is the banner while no worker runs and worker.toml was read for the Project: what is missing.
func (m Model) stoppedWorkerBanner(project string) WorkerBanner {
	s := m.Worker.Setup
	switch {
	case !s.Exists:
		return WorkerBanner{Badge: "No worker", Detail: "none set up for " + project}
	case s.LoadErr != "":
		return WorkerBanner{Badge: "No worker", Detail: "worker.toml does not load"}
	case !s.Lists:
		return WorkerBanner{Badge: "No worker", Detail: "worker.toml does not list " + project}
	case s.Manual:
		return WorkerBanner{Badge: "No worker", Detail: "supervision is off, so none starts by itself"}
	}
	return WorkerBanner{Badge: "Starting", Pending: true, Detail: "the plugin starts one within 10 seconds"}
}

// runNowRefusal is why a job cannot be run now, from what the Pane already knows; empty when nothing is known against it.
func (m Model) runNowRefusal(job CronJob) string {
	if job.Native() {
		return fmt.Sprintf("Cannot run %q now: kata runs a native job itself.", job.Name)
	}
	if b := m.WorkerBanner(); !b.Empty() && !b.Good {
		return fmt.Sprintf("Cannot run %q now: %s.", job.Name, b.Refusal())
	}
	return ""
}

// runJobNow is R on the Jobs Page: the worker is asked to run the selected adapter job now, under its usual admission and
// capacity rules.
func runJobNow(m Model) (Model, []Cmd) {
	job := m.SelectedJob()
	if job == nil {
		return m, nil
	}
	if job.Archived() {
		return m.fail(Cannot("run", fmt.Sprintf("%q", job.Name), errors.New("it is archived; restore it first"))), nil
	}
	if notice := m.runNowRefusal(*job); notice != "" {
		return m.fail(notice), nil
	}
	return m.say(Progress("Asking the worker to run", fmt.Sprintf("%q", job.Name))), []Cmd{RunJobNow{JobUID: job.UID, JobName: job.Name}}
}

type ResumeJob struct{ JobUID, JobName string }

func (ResumeJob) cmd() {}

// JobResumed answers ResumeJob. Err is the worker's refusal, in words for the user.
type JobResumed struct {
	JobUID, JobName string
	Err             error
}

func (JobResumed) msg() {}

// resumePausedJob is u on the Jobs Page: the worker admits the selected job's runs again and counts failures from none.
func resumePausedJob(m Model) (Model, []Cmd) {
	job := m.SelectedJob()
	if job == nil {
		return m, nil
	}
	if m.workerPausedJobs(job.UID) == 0 {
		return m.fail(Cannot("resume", fmt.Sprintf("%q", job.Name), errors.New("this machine's worker has not paused it"))), nil
	}
	return m.say(Progress("Asking the worker to resume", fmt.Sprintf("%q", job.Name))), []Cmd{ResumeJob{JobUID: job.UID, JobName: job.Name}}
}

func jobResumed(m Model, msg JobResumed) (Model, []Cmd) {
	if msg.Err != nil {
		return m.fail(Cannot("resume", fmt.Sprintf("%q", msg.JobName), msg.Err)), nil
	}
	m = m.say(fmt.Sprintf("Resumed %q: the worker admits its runs again.", msg.JobName))
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

func jobRunRequested(m Model, msg JobRunRequested) (Model, []Cmd) {
	if msg.Err != nil {
		return m.fail(Cannot("run", fmt.Sprintf("%q now", msg.JobName), msg.Err)), nil
	}
	m = m.say(fmt.Sprintf("Started %q. It is on the Runs Page.", msg.JobName))
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

// openRunSession is o on the Runs Page: the omp session of a finished or held run of this machine opens in a new Tern tab.
func openRunSession(m Model) (Model, []Cmd) {
	run := m.SelectedRun()
	if run == nil {
		return m, nil
	}
	if notice := m.notLocal(*run, "open", "the session"); notice != "" {
		return m.fail(notice), nil
	}
	d := m.LocalDetail()
	switch {
	case d == nil:
		return m.fail("Cannot open the session: the worker has not read this run yet."), nil
	case !d.Held && !d.Settled():
		return m.fail("Cannot open the session: the run is still running."), nil
	case d.SessionID == "":
		return m.fail("Cannot open the session: the agent recorded none for this run."), nil
	}
	return m, []Cmd{OpenRunSession{Cwd: d.Cwd, SessionDir: d.SessionDir, SessionID: d.SessionID}}
}

func (m Model) notLocal(run Run, verb, thing string) string {
	switch {
	case m.IsLocalRun(run):
		return ""
	case !m.Worker.Running:
		return Cannot(verb, thing, errors.New("no worker is running on this machine; start one with `tern-kata worker`"))
	}
	return Cannot(verb, thing, fmt.Errorf("it belongs to executor %q, not this machine's worker %q, and only its own machine can %s it", run.Executor(), m.Worker.Executor, verb))
}

// moveHeldRun is u and A on the Runs Page: the worker resumes the selected held run, or abandons it (cancels it when it is still
// running). A run the worker would not take the verb for is refused here; whatever else stands against it (a lease held elsewhere, no free slot) is the worker's answer to give.
func moveHeldRun(resume bool) func(Model) (Model, []Cmd) {
	verb := "abandon"
	if resume {
		verb = "resume"
	}
	return func(m Model) (Model, []Cmd) {
		run := m.SelectedRun()
		if run == nil {
			return m, nil
		}
		if notice := m.notLocal(*run, verb, "the run"); notice != "" {
			return m.fail(notice), nil
		}
		local, ok := m.LocalRunOf(run.UID)
		if !ok || !(local.Held || !resume && local.Abandonable) {
			return m.fail(Cannot(verb, "the run", errors.New("it is not held"))), nil
		}
		if resume {
			return m, []Cmd{ResumeLocalRun{UID: run.UID}}
		}
		return m, []Cmd{AbandonLocalRun{UID: run.UID}}
	}
}

func localRunMoved(m Model, msg LocalRunMoved) (Model, []Cmd) {
	verb := "abandon"
	if msg.Resumed {
		verb = "resume"
	}
	if msg.Err != nil {
		return m.fail(Cannot(verb, "the run", msg.Err)), nil
	}
	switch {
	case msg.Resumed:
		m = m.say(Progress("Resuming", "the run"))
	case msg.State == "cancelling":
		m = m.say(Progress("Cancelling", "the run"))
	default:
		m = m.say("Abandoned the run.")
	}
	m.Runs.stale = true
	m.Worker.detailStale = true
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

func runSessionOpened(m Model, msg RunSessionOpened) Model {
	if msg.Err != nil {
		return m.fail(Cannot("open", "the session", msg.Err))
	}
	return m.say("Opened the run's session in a new Tern tab.")
}

// RunNotice is why the selected held Run cannot be resumed now, as a sentence; empty when it can, or when it is not held here.
func (m Model) RunNotice() string {
	run := m.SelectedRun()
	if run == nil {
		return ""
	}
	local, ok := m.LocalRunOf(run.UID)
	if !ok || !local.Held || local.Resumable || local.ResumeRefusal == "" {
		return ""
	}
	if strings.Contains(local.ResumeRefusal, "being reported") {
		return "Cannot resume yet: its hold is still being reported to kata."
	}
	return "Cannot resume: " + strings.TrimSuffix(local.ResumeRefusal, ".") + "."
}

// pendingWords is what a Run's worker has not yet told kata, in words; an unknown value is shown as it is.
func pendingWords(pending string) string {
	if pending == "notify" {
		return "the notification"
	}
	return pending
}

// localFacts are the inspector's lines that only the worker knows, for a Run of this machine: the worker's own state when it
// differs from kata's, what it has not yet reported, its steps, and the tail of what the agent said.
func (m Model) localFacts(run Run) (state, rest runFacts) {
	local, ok := m.LocalRunOf(run.UID)
	if !ok {
		return nil, nil
	}
	if !local.Held && local.State != string(run.Status) {
		state.add("Worker", local.State)
	}
	if local.Attempt > 1 {
		state.add("Attempt", strconv.Itoa(local.Attempt))
	}
	state.add("Not yet reported to kata", pendingWords(local.Pending))
	d := m.LocalDetail()
	if d == nil || d.UID != run.UID {
		return state, nil
	}
	for _, s := range d.Steps {
		rest.add("Step "+strconv.Itoa(s.Index), stepWords(s))
	}
	rest.add("Output", strings.TrimSpace(d.OutputTail))
	if !local.Settled() || local.Held || d.StderrTail != "" && run.Status != cronjob.RunSucceeded {
		rest.add("Stderr", strings.TrimSpace(d.StderrTail))
	}
	return state, rest
}

// IDFact is one id a person may need to copy, such as a run's UID for `tern-kata worker resume`. The inspector keeps them in the
// hovercard of its title, never as rows.
type IDFact struct{ Label, Value string }

// RunIDs are the ids of the selected Run, for the hovercard of the inspector's title.
func (m Model) RunIDs() []IDFact {
	run := m.SelectedRun()
	if run == nil {
		return nil
	}
	var ids []IDFact
	add := func(label, value string) {
		if value != "" {
			ids = append(ids, IDFact{label, value})
		}
	}
	add("Run", run.UID)
	add("Occurrence", run.OccurrenceKey)
	add("Job", run.JobUID)
	add("Job version", run.DefinitionEventUID)
	add("Workflow", run.WorkflowUID)
	add("Workflow version", run.WorkflowDefinitionEventUID)
	add("Issue", run.IssueUID)
	if d := m.LocalDetail(); d != nil {
		add("Session", d.SessionID)
	}
	return ids
}

// stepWords is a step in a line: its kind, state and how it ended. A held state reads "held" alone: the Why row names the reason.
func stepWords(s LocalStep) string {
	state, _, _ := strings.Cut(s.State, ":")
	parts := []string{s.Kind, state}
	if s.Attempt > 1 {
		parts = append(parts[:1], append([]string{"attempt " + strconv.Itoa(s.Attempt)}, parts[1:]...)...)
	}
	switch {
	case s.TimedOut:
		parts = append(parts, "timed out")
	case s.Signal != "":
		parts = append(parts, "signal "+s.Signal)
	case s.ExitCode != nil:
		parts = append(parts, "exit "+strconv.Itoa(*s.ExitCode))
	}
	return strings.Join(parts, " · ")
}
