package model

import (
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
	Executor      string
	PID           int
	Supervised    bool
	Stopping      string
	InFlight      int
	Projects      []WorkerProject
	Runs          []LocalRun
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
	w.Executor, w.Projects, w.Runs = msg.Status.Executor, msg.Status.Projects, msg.Status.Runs
	w.PID, w.Supervised, w.Stopping, w.InFlight = msg.Status.PID, msg.Status.Supervised, msg.Status.Stopping, msg.Status.InFlight
	if !msg.Running {
		w.Executor, w.Projects, w.Runs, w.Detail, w.asked = "", nil, nil, nil, ""
		w.PID, w.Supervised, w.Stopping, w.InFlight = 0, false, "", 0
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
			m.Notice = "Reading the run from the worker failed: " + msg.Err.Error()
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

func (m Model) workerPauseWords(uid string) string {
	n := m.workerPausedJobs(uid)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("paused after %d failed run%s in a row", n, plural(n))
}

// WorkerLine is the Jobs Page's line about this machine's worker, and whether it is good news: whether the worker is running and
// serves this Pane's Project. Where worker.toml was read for this Project, each state points at W. It is empty until the first
// answer arrives.
func (m Model) WorkerLine() (text string, ok bool) {
	text, ok = m.workerLine()
	if w := m.Worker; w.Running && w.Err == nil && w.SetupRead && w.SetupErr == nil && w.Setup.Manual && text != "" {
		text += " Supervision is off (" + manualFile + "): the plugin does not restart it."
	}
	return text, ok
}

func (m Model) workerLine() (text string, ok bool) {
	w := m.Worker
	project := m.workerProjectName()
	known := w.SetupRead && w.SetupErr == nil && w.Setup.Project == project
	switch {
	case !w.Read:
		return "", false
	case w.Err != nil:
		return "Worker: " + w.Err.Error(), false
	case !w.Running && known:
		return m.stoppedWorkerLine(project), false
	case !w.Running:
		return "No worker is running on this machine; start one with `tern-kata worker`. Adapter jobs run only when one serves the Project.", false
	}
	for _, p := range w.Projects {
		if p.Name != project {
			continue
		}
		if !p.Admitting {
			return fmt.Sprintf("Worker %s serves %s but admits nothing: %s", w.Executor, project, p.Why), false
		}
		return fmt.Sprintf("Worker %s running and serving %s: %d of %d slots in use (W: worker)", w.Executor, project, p.InUse, p.Capacity), true
	}
	if known && !w.Setup.Lists {
		return fmt.Sprintf("Worker %s is running but does not serve %s; press W to serve it, then restart the worker.", w.Executor, project), false
	}
	if known {
		return fmt.Sprintf("Worker %s is running but does not serve %s, though worker.toml lists it; restart the worker to pick that up (W).", w.Executor, project), false
	}
	return fmt.Sprintf("Worker %s is running but does not serve %s; list the Project under [[projects]] in worker.toml.", w.Executor, project), false
}

// stoppedWorkerLine is the banner while no worker runs and worker.toml was read for the Project: what is missing, and W.
func (m Model) stoppedWorkerLine(project string) string {
	s := m.Worker.Setup
	switch {
	case !s.Exists:
		return fmt.Sprintf("No worker is set up on this machine: press W to set one up for %s. Adapter jobs run only when a worker serves the Project.", project)
	case s.LoadErr != "":
		return "worker.toml does not load, so no worker can start: press W to see why."
	case !s.Lists:
		return fmt.Sprintf("worker.toml does not list %s, so no worker serves it: press W to serve it.", project)
	case s.Manual:
		return "No worker is running on this machine and " + manualFile + " keeps the plugin from starting one: run `tern-kata worker` (W shows the setup)."
	}
	return "No worker is running on this machine yet; the plugin starts one within 10 seconds (W shows the setup)."
}

// runNowRefusal is why a job cannot be run now, from what the Pane already knows; empty when nothing is known against it.
func (m Model) runNowRefusal(job CronJob) string {
	if job.Native() {
		return fmt.Sprintf("%q is a native job: kata runs it, and nothing here can run it now. Only adapter jobs run on this machine's worker.", job.Name)
	}
	if text, ok := m.WorkerLine(); !ok && text != "" {
		return fmt.Sprintf("Cannot run %q now. %s", job.Name, text)
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
		m.Notice = fmt.Sprintf("%q is archived; restore it with x before running it.", job.Name)
		return m, nil
	}
	if reason := m.runNowRefusal(*job); reason != "" {
		m.Notice = reason
		return m, nil
	}
	m.Notice = fmt.Sprintf("Asking the worker to run %q…", job.Name)
	return m, []Cmd{RunJobNow{JobUID: job.UID, JobName: job.Name}}
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
		m.Notice = fmt.Sprintf("%q is not paused by this machine's worker; p pauses or resumes it at kata.", job.Name)
		return m, nil
	}
	m.Notice = fmt.Sprintf("Asking the worker to resume %q…", job.Name)
	return m, []Cmd{ResumeJob{JobUID: job.UID, JobName: job.Name}}
}

func jobResumed(m Model, msg JobResumed) (Model, []Cmd) {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot resume %q: %s", msg.JobName, msg.Err)
		return m, nil
	}
	m.Notice = fmt.Sprintf("Resumed %q: the worker admits its runs again and counts failed runs from none.", msg.JobName)
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

func jobRunRequested(m Model, msg JobRunRequested) (Model, []Cmd) {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot run %q now: %s", msg.JobName, msg.Err)
		return m, nil
	}
	m.Notice = fmt.Sprintf("Running %q now as %s; it is on the Runs Page (3).", msg.JobName, msg.RunUID)
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

// openRunSession is o on the Runs Page: the omp session of a finished or held run of this machine opens in a new Tern tab.
func openRunSession(m Model) (Model, []Cmd) {
	run := m.SelectedRun()
	if run == nil {
		return m, nil
	}
	if reason := m.notLocal(*run, "open its session"); reason != "" {
		m.Notice = reason
		return m, nil
	}
	d := m.LocalDetail()
	switch {
	case d == nil:
		m.Notice = "The worker has not read this run yet; try again in a moment."
		return m, nil
	case !d.Held && !d.Settled():
		m.Notice = "This run is still running; open its session once it has finished or is held."
		return m, nil
	case d.SessionID == "":
		m.Notice = "The agent recorded no omp session for this run."
		return m, nil
	}
	return m, []Cmd{OpenRunSession{Cwd: d.Cwd, SessionDir: d.SessionDir, SessionID: d.SessionID}}
}

func (m Model) notLocal(run Run, verb string) string {
	switch {
	case m.IsLocalRun(run):
		return ""
	case !m.Worker.Running:
		return "No worker is running on this machine, so its runs cannot " + verb + ". Start one with `tern-kata worker`."
	}
	return fmt.Sprintf("This run belongs to executor %q, not this machine's worker %q; only its own machine can %s.", run.Executor(), m.Worker.Executor, verb)
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
		if reason := m.notLocal(*run, verb+" it"); reason != "" {
			m.Notice = reason
			return m, nil
		}
		local, ok := m.LocalRunOf(run.UID)
		if !ok || !(local.Held || !resume && local.Abandonable) {
			m.Notice = "This run is not held, so there is nothing to " + verb + "."
			return m, nil
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
		m.Notice = "Cannot " + verb + " the run: " + msg.Err.Error()
		return m, nil
	}
	if msg.Resumed {
		m.Notice = "Resuming the run: its omp session continues as a new attempt."
	} else if msg.State == "cancelling" {
		m.Notice = "Cancelling the run: the worker stops its agent and records it cancelled at kata."
	} else {
		m.Notice = "Abandoned the run; the worker cancels it at kata."
	}
	m.Runs.stale = true
	m.Worker.detailStale = true
	m, cmd := m.askWorker(0)
	return m, []Cmd{cmd}
}

func runSessionOpened(m Model, msg RunSessionOpened) Model {
	if msg.Err != nil {
		m.Notice = "Opening the session failed: " + msg.Err.Error()
		return m
	}
	m.Notice = "Opened the run's omp session in a new Tern tab."
	return m
}

// localFacts are the inspector's lines that only the worker knows, for a Run of this machine: why it is held and which verbs
// would work, its steps, and the tail of what the agent said.
func (m Model) localFacts(run Run) (hold []RunFact, rest []RunFact) {
	local, ok := m.LocalRunOf(run.UID)
	if !ok {
		return nil, nil
	}
	add := func(into *[]RunFact, label, value string) {
		if value != "" {
			*into = append(*into, RunFact{label, value})
		}
	}
	add(&hold, "Worker state", local.State)
	if local.Held {
		add(&hold, "Held because", holdWords(local.Hold))
		add(&hold, "Hold detail", local.Detail)
		if local.Resumable {
			add(&hold, "Resume", "u continues the agent's session as a new attempt")
		} else {
			add(&hold, "Resume", "not now: "+local.ResumeRefusal)
		}
	}
	if local.Abandonable {
		add(&hold, "Abandon", "A cancels the run")
	}
	if local.Attempt > 1 {
		add(&hold, "Attempt", strconv.Itoa(local.Attempt))
	}
	add(&hold, "Unreported", local.Pending)
	d := m.LocalDetail()
	if d == nil || d.UID != run.UID {
		return hold, nil
	}
	for _, s := range d.Steps {
		add(&rest, "Step "+strconv.Itoa(s.Index), stepWords(s))
	}
	add(&rest, "Output", strings.TrimSpace(d.OutputTail))
	if !local.Settled() || local.Held || d.StderrTail != "" && run.Status != cronjob.RunSucceeded {
		add(&rest, "Stderr", strings.TrimSpace(d.StderrTail))
	}
	if d.SessionID != "" {
		add(&rest, "Session", d.SessionID+" (o opens it)")
	}
	return hold, rest
}

func holdWords(hold string) string { return strings.ReplaceAll(hold, "_", " ") }

func stepWords(s LocalStep) string {
	parts := []string{s.Kind, s.State}
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
