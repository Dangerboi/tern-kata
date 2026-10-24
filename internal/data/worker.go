package data

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// The worker is reached over its control socket, not through kata: one Cmd in, one Msg out, like every gateway command. Nothing
// here retries; the Pane asks again at its own pace.

// workerAskTimeout bounds one ask of the worker. A run request answers after the run's journal is durable, which takes a moment
// and no more.
const workerAskTimeout = 30 * time.Second

// ask sends one request to this machine's worker.
func (g *Gateway) ask(ctx context.Context, req worker.Request) (worker.Response, error) {
	dirs, err := g.workerDirs()
	if err != nil {
		return worker.Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, workerAskTimeout)
	defer cancel()
	resp, err := worker.Ask(ctx, dirs, req)
	if err != nil {
		return worker.Response{}, err
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func (g *Gateway) loadWorker(ctx context.Context, cmd model.LoadWorker) model.WorkerLoaded {
	if cmd.After > 0 {
		select {
		case <-time.After(cmd.After):
		case <-ctx.Done():
		}
	}
	resp, err := g.ask(ctx, worker.Request{Op: "status"})
	switch {
	case errors.Is(err, worker.ErrNoWorker):
		return model.WorkerLoaded{Request: cmd.Request}
	case err != nil:
		return model.WorkerLoaded{Request: cmd.Request, Err: err}
	case resp.Status == nil:
		return model.WorkerLoaded{Request: cmd.Request, Err: errors.New("the worker answered no status")}
	}
	return model.WorkerLoaded{Request: cmd.Request, Running: true, Status: translateWorkerStatus(*resp.Status)}
}

func (g *Gateway) loadLocalRun(ctx context.Context, cmd model.LoadLocalRun) model.LocalRunLoaded {
	resp, err := g.ask(ctx, worker.Request{Op: "run-detail", Run: cmd.UID})
	if err == nil && resp.Detail == nil {
		err = errors.New("the worker answered no detail")
	}
	if err != nil {
		return model.LocalRunLoaded{UID: cmd.UID, Err: err}
	}
	return model.LocalRunLoaded{UID: cmd.UID, Detail: translateRunDetail(*resp.Detail)}
}

func (g *Gateway) runJobNow(ctx context.Context, cmd model.RunJobNow) model.JobRunRequested {
	resp, err := g.ask(ctx, worker.Request{Op: "run", Job: cmd.JobUID})
	if err == nil && resp.Run == nil {
		err = errors.New("the worker accepted the request but named no run")
	}
	if err != nil {
		return model.JobRunRequested{JobUID: cmd.JobUID, JobName: cmd.JobName, Err: err}
	}
	return model.JobRunRequested{JobUID: cmd.JobUID, JobName: cmd.JobName, RunUID: resp.Run.RunUID}
}

func (g *Gateway) moveRun(ctx context.Context, op, uid string) model.LocalRunMoved {
	resp, err := g.ask(ctx, worker.Request{Op: op, Run: uid, Actor: "tern-kata pane"})
	moved := model.LocalRunMoved{UID: uid, Resumed: op == "resume", Err: err}
	if err == nil && resp.Transition != nil {
		moved.State = resp.Transition.State
	}
	return moved
}

// openRunSession opens the run's omp session in a new tab of Tern's session daemon.
func (g *Gateway) openRunSession(ctx context.Context, cmd model.OpenRunSession) model.RunSessionOpened {
	out, err := g.runTern(ctx, SessionTabArgs(cmd))
	if err != nil {
		if text := strings.TrimSpace(string(out)); text != "" {
			err = fmt.Errorf("%w: %s", err, text)
		}
		return model.RunSessionOpened{Err: err}
	}
	return model.RunSessionOpened{}
}

// SessionTabArgs is the tern command line that opens a run's omp session: a new tab in the run's working folder that continues the
// session. Tern quotes each word after -- for the login shell, so the words go as they are.
func SessionTabArgs(cmd model.OpenRunSession) []string {
	return []string{"new", "tab", "--cwd", cmd.Cwd, "--", "omp", "--resume", cmd.SessionID, "--session-dir", cmd.SessionDir}
}

// execTern runs the tern program with args, returning its combined output.
func execTern(ctx context.Context, args []string) ([]byte, error) {
	return exec.CommandContext(ctx, "tern", args...).CombinedOutput()
}

func translateWorkerStatus(s worker.Status) model.WorkerStatus {
	out := model.WorkerStatus{Executor: s.Executor, PID: s.PID, Supervised: s.Supervisor != "", Stopping: s.Stopping, InFlight: s.InFlight}
	for _, p := range s.Projects {
		project := model.WorkerProject{Name: p.Name, Admitting: p.Admitting, Why: p.Why, InUse: p.InUse, Capacity: p.Capacity}
		if len(p.Aside) > 0 {
			project.Unsupported = make(map[string]string, len(p.Aside))
			for _, n := range p.Aside {
				project.Unsupported[n.UID] = n.Text
			}
		}
		out.Projects = append(out.Projects, project)
	}
	for _, r := range s.Runs {
		out.Runs = append(out.Runs, translateLocalRun(r))
	}
	return out
}

func translateLocalRun(r worker.RunStatus) model.LocalRun {
	return model.LocalRun{UID: r.UID, Project: r.Project, Job: r.Job, Issue: r.Issue, State: r.State, Pending: r.Pending, Attempt: r.Attempt,
		Held: r.Held, Hold: r.Hold, Detail: r.Detail, Resumable: r.Resumable, ResumeRefusal: r.ResumeRefusal, Abandonable: r.Abandonable}
}

func translateRunDetail(d worker.RunDetail) model.LocalRunDetail {
	out := model.LocalRunDetail{LocalRun: translateLocalRun(d.RunStatus), Summary: d.Summary, OutputTail: d.OutputTail, StderrTail: d.StderrTail,
		Cwd: d.Cwd, SessionDir: d.SessionDir, SessionID: d.SessionID}
	for _, s := range d.Steps {
		out.Steps = append(out.Steps, model.LocalStep{Index: s.Index, Kind: string(s.Kind), Attempt: s.Attempt, State: s.State, SessionID: s.SessionID,
			ExitCode: s.ExitCode, Signal: s.Signal, TimedOut: s.TimedOut})
	}
	return out
}
