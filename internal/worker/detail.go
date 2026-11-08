package worker

import (
	"cmp"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// RunDetail is everything a Pane needs to show one run, held or not: its status line, why it is held, its steps and
// the tail of what the agent said. Answered to the control op "run-detail".
type RunDetail struct {
	RunStatus
	// Summary is what kata records for the run: the outcome's summary, or empty while it runs.
	Summary string
	// Steps are the run's steps in order; a prompt job has one.
	Steps []StepDetail
	// OutputTail is the end of the agent's assistant messages (at most OutputTailBytes), StderrTail the end of its stderr.
	OutputTail string
	StderrTail string
	// Cwd is where the agent ran (the job's checkout, else the run's own folder), SessionDir the folder omp keeps the
	// run's sessions in and SessionID the session a resume continues; the last is empty until omp has written one.
	Cwd, SessionDir, SessionID string
}

type StepDetail struct {
	Index   int              // 1-based
	Kind    cronjob.StepKind // prompt or command
	Key     string           // a workflow step's key; empty for a prompt job's one step
	Attempt int
	Summary string
	// State is running, held: <reason>, succeeded, failed or cancelled for the step.
	State      string
	SessionID  string
	PID        int
	StartedAt  time.Time
	EndedAt    time.Time
	ExitCode   *int
	Signal     string
	TimedOut   bool
	Unrecorded string
}

const OutputTailBytes = 4 << 10

func stepKind(j *Journal, i int) cronjob.StepKind {
	if j.Workflow == nil || i < 0 || i >= len(j.Workflow.Steps) {
		return ""
	}
	return j.Workflow.Steps[i].Kind
}

// stepsOf is the journal's steps: one for a prompt job, one per step of a workflow's plan in order, a step not yet
// reached showing as pending.
func stepsOf(j *Journal) []StepDetail {
	if j.Workflow == nil {
		st := StepDetail{Index: 1, Kind: cronjob.StepPrompt, Attempt: j.attempt(), State: j.stateName()}
		st.fill(j.Launch, j.Exit)
		return []StepDetail{st}
	}
	steps := make([]StepDetail, len(j.Steps))
	active := j.activeStep()
	for i, r := range j.Steps {
		st := StepDetail{Index: i + 1, Kind: stepKind(j, i), Key: r.Key, Attempt: r.Tries, State: stepStateText(j, i), Summary: r.Summary, StartedAt: r.StartedAt, EndedAt: r.EndedAt}
		launch, exit := r.Launch, r.Exit
		if i == active { // the attempt in flight keeps its records on the run until it concludes
			launch, exit = j.Launch, j.Exit
		}
		st.fill(launch, exit)
		steps[i] = st
	}
	return steps
}

// fill copies what an attempt's launch and exit records say; either may be nil.
func (st *StepDetail) fill(launch *LaunchRecord, exit *ExitRecord) {
	if launch != nil {
		st.PID, st.SessionID, st.StartedAt = launch.PID, launch.SessionID, launch.At
	}
	if exit != nil {
		code := exit.Code
		st.ExitCode, st.Signal, st.TimedOut, st.EndedAt, st.Unrecorded = &code, exit.Signal, exit.TimedOut, exit.At, exit.Unrecorded
	}
}

func (w *Worker) RunDetail(uid string) (RunDetail, error) {
	j, err := w.ownRun(uid)
	if err != nil {
		return RunDetail{}, err
	}
	d := RunDetail{RunStatus: w.runStatus(j), Steps: stepsOf(j), OutputTail: outputTail(w.heldFile(j, "output.jsonl"), OutputTailBytes)}
	d.StderrTail = stderrFallback(w.heldFile(j, "stderr.log"))
	if t := shownStderrTail(j); t != "" { // stderr.log keeps only the first bytes; the journal holds the real end
		d.StderrTail = t
	}
	d.Cwd = cmp.Or(j.Checkout, w.store.RunDir(uid))
	d.SessionDir, d.SessionID = w.sessionOf(j)
	if d.OutputTail == "" { // a command step's output is its own log
		d.OutputTail = fileTail(w.heldFile(j, "output.log"), OutputTailBytes)
	}
	if note := j.outputLoss(); note != "" { // the tail is what was recorded; say why it may be short or empty
		d.OutputTail = strings.TrimSpace(outputNote(note) + "\n\n" + d.OutputTail)
	}
	if j.Outcome != nil {
		d.Summary = j.Outcome.Summary
	}
	return d, nil
}

// shownStderrTail is the journal's stderr tail for the attempt RunDetail shows: the run's own for a prompt job or an
// attempt in flight, else the step attemptDir picks (the active one, else the latest with a folder).
func shownStderrTail(j *Journal) string {
	if j.Result != nil && j.Result.StderrTail != "" {
		return j.Result.StderrTail
	}
	if j.Workflow == nil {
		return ""
	}
	// A running step's stored tail is an earlier attempt's; the live stderr.log shows this one.
	if i := j.activeStep(); i >= 0 && j.Steps[i].Dir != "" {
		return ""
	}
	for i := len(j.Steps) - 1; i >= 0; i-- {
		if j.Steps[i].Dir != "" {
			return j.Steps[i].StderrTail
		}
	}
	return ""
}

// sessionOf is the omp session a Pane opens for the run and the folder holding it: a prompt job's, or for a workflow
// the latest prompt step's that has one (each step keeps its own session folder). Empty when there is none, as for a
// workflow of command steps alone.
func (w *Worker) sessionOf(j *Journal) (folder, id string) {
	runDir := w.store.RunDir(j.RunUID)
	if j.Workflow == nil {
		return sessionFolder(runDir), w.sessionID(j)
	}
	active := j.activeStep()
	for i := len(j.Steps) - 1; i >= 0; i-- {
		r := j.Steps[i]
		if stepKind(j, i) != cronjob.StepPrompt || r.Dir == "" {
			continue
		}
		launch := r.Launch
		if i == active {
			launch = j.Launch
		}
		dir := sessionFolder(filepath.Join(runDir, r.Dir))
		if id := sessionIn(dir, launch.session()); id != "" {
			return dir, id
		}
	}
	return "", ""
}
