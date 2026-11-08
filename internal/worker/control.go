package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/oklog/ulid/v2"
)

// The control socket (ADR-0004): one JSON request per connection, one JSON reply. It replaces polling of request
// files; the socket reply is the acknowledgement, sent only after a run's journal is durable (ADR-0002).

// Request is one control request. Ops:
//
//	"run"        Job: a job uid; answers Run.
//	"status"     answers Status, whose Runs carry each run's hold, reason and which verbs would be accepted.
//	"run-detail" Run: a run uid; answers Detail (status line, hold reason, steps, output tail).
//	"resume"     Run: a held run's uid, Actor: who asks (optional); answers Transition.
//	"resume-job" Job: a paused job's uid; answers PausedJob. It lifts the pause the worker put on a job after repeated failed runs.
//	"stop"       Mode: "idle" (the default) or "now"; answers Stop. Idle refuses new work and exits once no run is in flight; now exits at once.
//	"abandon"    Run: a held or running run's uid (or one this machine lost the state of), Actor: who asks (optional); answers Transition.
//	"identity"   answers Identity: the worker's process, version and supervisor, without reading a journal (the host half asks it every poll).
type Request struct {
	Op    string `json:"op"`
	Job   string `json:"job,omitempty"`
	Run   string `json:"run,omitempty"`
	Actor string `json:"actor,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

// Response answers a Request; Error is set when it was refused, in words for the user.
type Response struct {
	OK         bool           `json:"ok"`
	Error      string         `json:"error,omitempty"`
	Run        *RunAccepted   `json:"run,omitempty"`
	Status     *Status        `json:"status,omitempty"`
	Detail     *RunDetail     `json:"detail,omitempty"`
	Transition *RunTransition `json:"transition,omitempty"`
	Stop       *StopAnswer    `json:"stop,omitempty"`
	Resumed    *PausedJob     `json:"resumed,omitempty"`
	Identity   *Identity      `json:"identity,omitempty"`
}

// Identity is which worker answers: its process, its version and who started it (SupervisorTern, or empty when started by hand).
type Identity struct {
	PID        int    `json:"pid"`
	Version    string `json:"version,omitempty"`
	Supervisor string `json:"supervisor,omitempty"`
}

// ErrNoWorker means nothing answers on the control socket.
var ErrNoWorker = errors.New("no tern-kata worker is running on this machine; start one with `tern-kata worker`")

func Ask(ctx context.Context, dirs Dirs, req Request) (Response, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", dirs.Socket())
	if err != nil {
		return Response{}, ErrNoWorker
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return Response{}, errors.New("the worker closed the connection without answering")
	}
	return resp, nil
}

func (w *Worker) serveControl(ctx context.Context, ln interface {
	Accept() (net.Conn, error)
}) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		if !peerAllowed(conn) {
			_ = conn.Close()
			continue
		}
		w.runs.Go(func() {
			defer conn.Close()
			unhook := context.AfterFunc(ctx, func() { _ = conn.Close() }) // a client that says nothing must not outlive the worker
			defer unhook()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
			var req Request
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = json.NewEncoder(conn).Encode(Response{Error: "unreadable request"})
				return
			}
			if req.Op == OpStop {
				unhook() // the last run may end before the answer is written, and that cancels ctx; the deadline still bounds it
			}
			_ = json.NewEncoder(conn).Encode(w.handle(ctx, req))
			if req.Op == OpStop {
				w.finishDrain()
			}
		})
	}
}

func (w *Worker) handle(ctx context.Context, req Request) Response {
	switch req.Op {
	case OpRunDetail, OpResume, OpAbandon:
		if _, err := ulid.ParseStrict(req.Run); err != nil {
			return Response{Error: fmt.Sprintf("%q%s", req.Run, notARunUIDSuffix)}
		}
	}
	if _, known := whileStopping[req.Op]; known {
		if err := w.stopRefusal(req.Op); err != nil {
			return Response{Error: err.Error()}
		}
	}
	switch req.Op {
	case OpStatus:
		s := w.Status()
		return Response{OK: true, Status: &s}
	case OpIdentity:
		return Response{OK: true, Identity: &Identity{PID: w.pid, Version: w.opt.Version, Supervisor: w.opt.Supervisor}}
	case OpRun:
		accepted, err := w.RequestRun(ctx, req.Job)
		return answer(Response{Run: &accepted}, err)
	case OpResumeJob:
		p, err := w.ResumeJob(req.Job)
		return answer(Response{Resumed: &p}, err)
	case OpStop:
		a, err := w.Stop(req.Mode)
		return answer(Response{Stop: &a}, err)
	case OpRunDetail:
		d, err := w.RunDetail(req.Run)
		return answer(Response{Detail: &d}, err)
	case OpResume, OpAbandon:
		verb := w.Resume
		if req.Op == OpAbandon {
			verb = w.Abandon
		}
		t, err := verb(ctx, req.Run, req.Actor)
		return answer(Response{Transition: &t}, err)
	}
	return Response{Error: "unknown request " + req.Op}
}

func answer(ok Response, err error) Response {
	if err != nil {
		return Response{Error: err.Error()}
	}
	ok.OK = true
	return ok
}
