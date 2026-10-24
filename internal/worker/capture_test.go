//go:build linux

package worker_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// logLines collects the Worker's log.
type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (l *logLines) count(sub string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.lines {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

// spoil makes the capture file of a run (relative to its folder) impossible to create: a directory in its place.
func spoil(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Error(err)
	}
}

// failWritesAfter lets 3 bytes into any capture file, then fails every write.
func failWritesAfter(path string) (int, bool) { return 3, true }

func (h *harness) wantUnrecorded(r *running, uid string, log *logLines) {
	h.t.Helper()
	var status worker.RunStatus
	for _, s := range r.w.Status().Runs {
		if s.UID == uid {
			status = s
		}
	}
	if !strings.Contains(status.Unrecorded, "output") {
		h.t.Fatalf("run status carries no loss: %+v", status)
	}
	if text := r.w.Status().String(); !strings.Contains(text, "output not recorded: "+status.Unrecorded) {
		h.t.Fatalf("worker status does not say it:\n%s", text)
	}
	detail, err := r.w.RunDetail(uid)
	if err != nil || !strings.Contains(detail.OutputTail, "output not recorded: "+status.Unrecorded) {
		h.t.Fatalf("run detail says %q (%v)", detail.OutputTail, err)
	}
	if n := log.count("output not recorded"); n != 1 {
		h.t.Fatalf("the Worker's log says it %d times", n)
	}
}

func TestAnAgentRunWhoseOutputLogCannotBeCreatedStillSettlesWithTheAgentsResult(t *testing.T) {
	for name, writeFails := range map[string]bool{"cannot be created": false, "cannot be written": true} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, "--stub-text", "All done.")
			job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
			log := &logLines{}
			hooks := worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
				if point == "admitted" && !writeFails {
					spoil(t, filepath.Join(h.dirs.Runs(), j.RunUID, "output.jsonl"))
				}
				return nil
			}}
			if writeFails {
				hooks.CaptureWriteLimit = failWritesAfter
			}
			r := h.start(hooks, func(o *worker.Options) { o.Log = log.add })
			h.admitting(r)
			uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
			run := h.settled(uid)
			if run.Status != "succeeded" || !strings.Contains(run.Summary, "All done.") {
				t.Fatalf("run is %q: %q", run.Status, run.Summary)
			}
			h.wantUnrecorded(r, uid, log)
		})
	}
}

func TestACommandStepWhoseOutputLogCannotBeWrittenIsNotHeldAndSettlesFromItsExitStatus(t *testing.T) {
	for name, writeFails := range map[string]bool{"cannot be created": false, "cannot be written": true} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			_, job := h.workflowJob("build", workertest.Step{Key: "make", Command: "echo noisy; echo done >> made.txt"})
			log := &logLines{}
			hooks := worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
				if point == "step-started" && !writeFails {
					dir := filepath.Join(h.dirs.Runs(), j.RunUID, j.Steps[0].Dir)
					if err := os.MkdirAll(dir, 0o700); err != nil {
						t.Error(err)
					}
					spoil(t, filepath.Join(dir, "output.log"))
				}
				return nil
			}}
			if writeFails {
				hooks.CaptureWriteLimit = failWritesAfter
			}
			r := h.start(hooks, func(o *worker.Options) { o.Log = log.add })
			h.admitting(r)
			uid := h.runWorkflow(r, job)
			run := h.settled(uid)
			if run.Status != "succeeded" || strings.Contains(run.Summary, "noisy") {
				t.Fatalf("run is %q: %q", run.Status, run.Summary)
			}
			if got := h.lines("made.txt"); len(got) != 1 {
				t.Fatalf("the command did not run to its end: %v", got)
			}
			h.wantUnrecorded(r, uid, log)
		})
	}
}

// A step that lost its capture is named in status and the run detail for as long as the run shows, whether a later step
// is still running or has recorded fine.
func TestAnEarlierStepsLostCaptureStaysShownWhileAndAfterALaterStepRecords(t *testing.T) {
	h := newHarness(t)
	_, job := h.workflowJob("build",
		workertest.Step{Key: "first", Command: "echo noisy"},
		workertest.Step{Key: "second", After: []string{"first"}, Command: "while [ ! -f go.txt ]; do sleep 0.1; done; echo fine"})
	spoiled := false
	r := h.start(worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point == "step-started" && !spoiled {
			spoiled = true
			dir := filepath.Join(h.dirs.Runs(), j.RunUID, j.Steps[0].Dir)
			spoil(t, filepath.Join(dir, "output.log"))
		}
		return nil
	}})
	h.admitting(r)
	uid := h.runWorkflow(r, job)

	shown := func(when string) {
		t.Helper()
		status, _ := h.runStatus(r, uid)
		if !strings.Contains(status.Unrecorded, `step "first"`) || strings.Contains(status.Unrecorded, `step "second"`) {
			t.Errorf("%s: run status says %q", when, status.Unrecorded)
		}
		if text := r.w.Status().String(); !strings.Contains(text, "output not recorded: "+status.Unrecorded) {
			t.Errorf("%s: worker status does not say it:\n%s", when, text)
		}
		detail, err := r.w.RunDetail(uid)
		if err != nil || !strings.Contains(detail.OutputTail, "output not recorded: "+status.Unrecorded) {
			t.Errorf("%s: run detail says %q (%v)", when, detail.OutputTail, err)
		}
		if len(detail.Steps) != 2 || detail.Steps[0].Unrecorded == "" || detail.Steps[1].Unrecorded != "" {
			t.Errorf("%s: the steps show %+v", when, detail.Steps)
		}
	}
	h.within("the second step to run", func() bool {
		d, err := r.w.RunDetail(uid)
		return err == nil && len(d.Steps) == 2 && d.Steps[1].State == "running"
	})
	shown("while the later step runs")
	if err := os.WriteFile(filepath.Join(h.repo, "go.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if run := h.settled(uid); run.Status != "succeeded" {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	shown("after the later step recorded")
}

func TestARunWithNoCaptureTroubleShowsNoNote(t *testing.T) {
	h := newHarness(t, "--stub-text", "Fine.")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.settled(uid)
	detail, err := r.w.RunDetail(uid)
	if err != nil || detail.Unrecorded != "" || strings.Contains(detail.OutputTail, "not recorded") || strings.Contains(r.w.Status().String(), "not recorded") {
		t.Fatalf("a note shows with no trouble: %+v (%v)", detail, err)
	}
}
