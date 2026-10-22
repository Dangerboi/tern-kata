package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	Index   int    // 1-based
	Kind    string // "prompt"
	Attempt int
	// State is running, held: <reason>, succeeded, failed or cancelled for the step.
	State     string
	SessionID string
	PID       int
	StartedAt time.Time
	EndedAt   time.Time
	ExitCode  *int
	Signal    string
	TimedOut  bool
}

// OutputTailBytes bounds RunDetail.OutputTail.
const OutputTailBytes = 4 << 10

// stepsOf is the journal's steps. A workflow run lists one entry per step here once the journal records them.
func stepsOf(j *Journal) []StepDetail {
	st := StepDetail{Index: 1, Kind: "prompt", Attempt: j.attempt(), State: runStatus(j).State}
	if l := j.Launch; l != nil {
		st.PID, st.SessionID, st.StartedAt = l.PID, l.SessionID, l.At
	}
	if e := j.Exit; e != nil {
		code := e.Code
		st.ExitCode, st.Signal, st.TimedOut, st.EndedAt = &code, e.Signal, e.TimedOut, e.At
	}
	return []StepDetail{st}
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
	dir := w.store.RunDir(uid)
	d := RunDetail{RunStatus: w.runStatus(j), Steps: stepsOf(j), OutputTail: outputTail(filepath.Join(dir, "output.jsonl"), OutputTailBytes), StderrTail: fileTail(filepath.Join(dir, "stderr.log"), 2048)}
	d.Cwd, d.SessionDir, d.SessionID = cmpOr(j.Checkout, dir), sessionFolder(dir), w.sessionID(j)
	if j.Outcome != nil {
		d.Summary = j.Outcome.Summary
	}
	return d, nil
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
