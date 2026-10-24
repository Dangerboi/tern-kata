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
			if run.Status != "succeeded" || (!writeFails && strings.Contains(run.Summary, "noisy")) {
				t.Fatalf("run is %q: %q", run.Status, run.Summary)
			}
			if got := h.lines("made.txt"); len(got) != 1 {
				t.Fatalf("the command did not run to its end: %v", got)
			}
			h.wantUnrecorded(r, uid, log)
		})
	}
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
