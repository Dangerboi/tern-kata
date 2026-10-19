package main

import (
	"context"

	"github.com/stencil-hq/tern-sdk/go/tern"
)

type paneSession interface {
	Next(ctx context.Context) (tern.Input, error)
	Caps() tern.Caps
	Close() error
}

type paneSurface interface {
	Render(tern.View) error
	Send(...tern.Op) error
	Close() error
}

// reconnect opens a fresh Tern session and the Pane's surface on it.
type reconnect func(ctx context.Context) (paneSession, paneSurface, error)

// terminal owns the Tern session, the surface and the input reader, and lets the process run a function outside Tern.
type terminal struct {
	ctx         context.Context
	session     paneSession
	surface     paneSurface
	reconnect   reconnect
	stopInput   func()
	inputs      chan any
	inputErrors chan error
}

func newTerminal(ctx context.Context, session paneSession, surface paneSurface, reconnect reconnect) *terminal {
	t := &terminal{ctx: ctx, session: session, surface: surface, reconnect: reconnect, inputs: make(chan any), inputErrors: make(chan error, 1)}
	t.stopInput = startInputReader(ctx, session, t.inputs, t.inputErrors)
	return t
}

func (t *terminal) Render(v tern.View) error { return t.surface.Render(v) }

// Send sends one op per frame: Tern needs the first tab reveal laid out before the second.
func (t *terminal) Send(op tern.Op) error { return t.surface.Send(op) }

func (t *terminal) Caps() tern.Caps { return t.session.Caps() }

// Inputs delivers every Tern input; InputErrors the reader's failure.
func (t *terminal) Inputs() <-chan any        { return t.inputs }
func (t *terminal) InputErrors() <-chan error { return t.inputErrors }

func (t *terminal) Close() {
	t.stopInput()
	t.surface.Close()
	t.session.Close()
}

// Suspend hands the terminal to run and resumes the Pane afterwards through the injected reconnect.
// When resuming fails the terminal is unusable and the error is returned.
func (t *terminal) Suspend(run func()) error {
	t.stopInput()
	t.surface.Close()
	// The Session must be closed first: Suspend leaves its stdin pump and raw mode active.
	t.session.Close()
	run()
	session, surface, err := t.reconnect(t.ctx)
	if err != nil {
		return err
	}
	t.session, t.surface = session, surface
	t.stopInput = startInputReader(t.ctx, session, t.inputs, t.inputErrors)
	return nil
}

// connectPane is the production reconnect: Tern's connect plus the Pane's surface.
func connectPane(ctx context.Context) (paneSession, paneSurface, error) {
	session, err := tern.Connect(ctx, editorOptions())
	if err != nil {
		return nil, nil, err
	}
	surface, err := openSurface(session)
	if err != nil {
		session.Close()
		return nil, nil, err
	}
	return session, surface, nil
}
