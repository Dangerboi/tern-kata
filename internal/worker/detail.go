package worker

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
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

// StepDetail is one step of a run as the journal knows it.
type StepDetail struct {
	Index   int              // 1-based
	Kind    cronjob.StepKind // prompt or command
	Key     string           // a workflow step's key; empty for a prompt job's one step
	Attempt int
	// Summary is what the step said or why it ended as it did; empty while it runs.
	Summary string
	// State is running, held: <reason>, succeeded, failed or cancelled for the step.
	State     string
	SessionID string
	PID       int
	StartedAt time.Time
	EndedAt   time.Time
	ExitCode  *int
	Signal    string
	TimedOut  bool
	// Unrecorded is why the step's output was not recorded, empty when it was.
	Unrecorded string
}

// OutputTailBytes bounds RunDetail.OutputTail.
const OutputTailBytes = 4 << 10

// stepsOf is the journal's steps: one for a prompt job, one per step of a workflow's plan in order, a step not yet
// reached showing as pending.
func stepsOf(j *Journal) []StepDetail {
	if j.Workflow == nil {
		st := StepDetail{Index: 1, Kind: cronjob.StepPrompt, Attempt: j.attempt(), State: j.stateName()}
		if l := j.Launch; l != nil {
			st.PID, st.SessionID, st.StartedAt = l.PID, l.SessionID, l.At
		}
		if e := j.Exit; e != nil {
			code := e.Code
			st.ExitCode, st.Signal, st.TimedOut, st.EndedAt, st.Unrecorded = &code, e.Signal, e.TimedOut, e.At, e.Unrecorded
		}
		return []StepDetail{st}
	}
	steps := make([]StepDetail, len(j.Steps))
	active := j.activeStep()
	for i, r := range j.Steps {
		st := StepDetail{Index: i + 1, Kind: j.Workflow.Steps[i].Kind, Key: r.Key, Attempt: r.Tries, State: stepStateText(j, i), Summary: r.Summary, StartedAt: r.StartedAt, EndedAt: r.EndedAt}
		launch, exit := r.Launch, r.Exit
		if i == active { // the attempt in flight keeps its records on the run until it concludes
			launch, exit = j.Launch, j.Exit
		}
		if launch != nil {
			st.PID, st.SessionID, st.StartedAt = launch.PID, launch.SessionID, launch.At
		}
		if exit != nil {
			code := exit.Code
			st.ExitCode, st.Signal, st.TimedOut, st.EndedAt, st.Unrecorded = &code, exit.Signal, exit.TimedOut, exit.At, exit.Unrecorded
		}
		steps[i] = st
	}
	return steps
}

// RunDetail reads one run of this worker for a Pane.
func (w *Worker) RunDetail(uid string) (RunDetail, error) {
	j, err := w.store.Load(uid)
	switch {
	case err == ErrNoJournal && w.store.IsQuarantined(uid):
		return RunDetail{}, fmt.Errorf("run %s has a damaged journal, set aside in quarantine; run `tern-kata worker doctor`", uid)
	case err == ErrNoJournal:
		return RunDetail{}, fmt.Errorf("this worker has no run %s", uid)
	case err != nil:
		return RunDetail{}, err
	case j.Executor != w.opt.Config.Executor:
		return RunDetail{}, fmt.Errorf("run %s belongs to worker %q, not %q", uid, j.Executor, w.opt.Config.Executor)
	}
	dir := w.attemptDir(j) // a workflow shows its latest step's output
	d := RunDetail{RunStatus: w.runStatus(j), Steps: stepsOf(j), OutputTail: outputTail(filepath.Join(dir, "output.jsonl"), OutputTailBytes), StderrTail: fileTail(filepath.Join(dir, "stderr.log"), 2048)}
	d.Cwd = cmp.Or(j.Checkout, w.store.RunDir(uid))
	d.SessionDir, d.SessionID = w.sessionOf(j)
	if d.OutputTail == "" { // a command step's output is its own log
		d.OutputTail = fileTail(filepath.Join(dir, "output.log"), OutputTailBytes)
	}
	if note := j.outputLoss(); note != "" { // the tail is what was recorded; say why it may be short or empty
		d.OutputTail = strings.TrimSpace(outputNote(note) + "\n\n" + d.OutputTail)
	}
	if j.Outcome != nil {
		d.Summary = j.Outcome.Summary
	}
	return d, nil
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
		if j.Workflow.Steps[i].Kind != cronjob.StepPrompt || r.Dir == "" {
			continue
		}
		launch := r.Launch
		if i == active {
			launch = j.Launch
		}
		var launched string
		if launch != nil {
			launched = launch.SessionID
		}
		dir := sessionFolder(filepath.Join(runDir, r.Dir))
		if id := sessionIn(dir, launched); id != "" {
			return dir, id
		}
	}
	return "", ""
}

// outputTail joins the assistant messages of an output.jsonl and keeps the last max bytes.
func outputTail(path string, max int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var texts []string
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 && line[0] == '{' {
			var ev struct {
				Type    string      `json:"type"`
				Message *ompMessage `json:"message"`
			}
			if json.Unmarshal(line, &ev) == nil && ev.Type == "message_end" && ev.Message != nil && ev.Message.Role == "assistant" {
				if t := strings.TrimSpace(ev.Message.text()); t != "" {
					texts = append(texts, t)
				}
			}
		}
		if err != nil {
			break
		}
	}
	out := strings.Join(texts, "\n\n")
	if len(out) > max {
		out = strings.ToValidUTF8(out[len(out)-max:], "")
	}
	return out
}

// fileTail is the last max bytes of a text file, empty when it cannot be read.
func fileTail(path string, max int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(raw) > max {
		raw = raw[len(raw)-max:]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(raw), ""))
}
