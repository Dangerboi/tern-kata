package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"
)

// The control socket (ADR-0004): one JSON request per connection, one JSON reply. It replaces polling of request
// files; the socket reply is the acknowledgement, sent only after a run's journal is durable (ADR-0002).

// Request is one control request. Ops:
//
//	"run"        Job: a job uid; answers Run.
//	"status"     answers Status, whose Runs carry each run's hold, reason and which verbs would be accepted.
//	"run-detail" Run: a run uid; answers Detail (status line, hold reason, steps, output tail).
//	"resume"     Run: a held run's uid, Actor: who asks (optional); answers Transition.
//	"abandon"    Run: a held or running run's uid (or one this machine lost the state of), Actor: who asks (optional); answers Transition.
type Request struct {
	Op    string `json:"op"`
	Job   string `json:"job,omitempty"`
	Run   string `json:"run,omitempty"`
	Actor string `json:"actor,omitempty"`
}

// Response answers a Request; Error is set when it was refused, in words for the user.
type Response struct {
	OK         bool           `json:"ok"`
	Error      string         `json:"error,omitempty"`
	Run        *RunAccepted   `json:"run,omitempty"`
	Status     *Status        `json:"status,omitempty"`
	Detail     *RunDetail     `json:"detail,omitempty"`
	Transition *RunTransition `json:"transition,omitempty"`
}

// ErrNoWorker means nothing answers on the control socket.
var ErrNoWorker = errors.New("no tern-kata worker is running on this machine; start one with `tern-kata worker`")

// Ask sends req to the running worker of dirs. It returns ErrNoWorker when none answers.
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
		w.runs.Add(1)
		go func() {
			defer w.runs.Done()
			defer conn.Close()
			defer context.AfterFunc(ctx, func() { _ = conn.Close() })() // a client that says nothing must not outlive the worker
			_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
			var req Request
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = json.NewEncoder(conn).Encode(Response{Error: "unreadable request"})
				return
			}
			_ = json.NewEncoder(conn).Encode(w.handle(ctx, req))
		}()
	}
}

func (w *Worker) handle(ctx context.Context, req Request) Response {
	switch req.Op {
	case "status":
		s := w.Status()
		return Response{OK: true, Status: &s}
	case "run":
		if w.halted.Load() {
			return Response{Error: "the worker is stopping"}
		}
		accepted, err := w.RequestRun(ctx, req.Job)
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{OK: true, Run: &accepted}
	case "run-detail":
		d, err := w.RunDetail(req.Run)
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{OK: true, Detail: &d}
	case "resume", "abandon":
		var t RunTransition
		var err error
		if req.Op == "resume" {
			t, err = w.Resume(ctx, req.Run, req.Actor)
		} else {
			t, err = w.Abandon(ctx, req.Run, req.Actor)
		}
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{OK: true, Transition: &t}
	}
	return Response{Error: "unknown request " + req.Op}
}
