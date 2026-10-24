// Package driver is the Pane driver: one pure step from a Tern input or a
// daemon result to the next state, the view to draw, the Tern ops to send and
// the daemon commands to run. It decodes, calls the model, renders and routes;
// the view decides which focus, reveal and scroll targets are safe to name,
// because Tern answers an op on a node it does not draw with an error event
// that ends the program.
package driver

import (
	"fmt"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// State is the model plus the text focus target last sent to Tern.
type State struct {
	Model model.Model
	Focus string
}

// Fresh is the state after the surface was replaced: Tern holds no caret, so
// the next step sends the text focus again.
func (s State) Fresh() State {
	s.Focus = ""
	return s
}

// Output is what one step asks of the process, in this order: render View,
// send Ops, launch Cmds, then run Editor. A non-nil Err is fatal; Quit ends the
// loop without drawing.
type Output struct {
	State State
	// View is nil when the input changed nothing to draw.
	View *tern.View
	Ops  []tern.Op
	// Cmds are the daemon commands.
	Cmds []model.Cmd
	// Editor is the $EDITOR round trip, kept apart so every daemon command of the step runs before it.
	Editor *model.OpenDraftEditor
	Quit   bool
	Err    error
}

// Step applies in, a tern.Input or a model.Msg (a daemon result), to s. now is
// the time the view measures relative dates from.
func Step(s State, in any, now time.Time) Output {
	var msg model.Msg
	switch in := in.(type) {
	case model.Msg:
		msg = in
	case *tern.ErrorEvent:
		// The SDK queues raw focus/reveal ops even when a later Render replaces their nodes.
		if id, ok := strings.CutPrefix(in.Msg, "unknown id "); ok && view.ExpectedUnknownID(id) {
			return Output{State: s}
		}
		return Output{State: s, Err: fmt.Errorf("Tern surface: %s", in.Msg)}
	case tern.Input:
		msg = decode(in)
	}
	if msg == nil {
		return Output{State: s}
	}

	previousSelection := s.Model.CurrentSelection()
	s.Model.Now = now
	m, cmds := model.Update(s.Model, msg)
	s.Model = m
	if m.Quitting {
		return Output{State: s, Quit: true}
	}
	v := view.Render(m, now)
	out := Output{State: s, View: &v}

	if target := view.TextFocusID(m); target != s.Focus {
		out.Ops = append(out.Ops, tern.OpFocus(target))
		out.State.Focus = target
	}
	if reveal := view.SelectionRevealID(m, msg, previousSelection); reveal != "" {
		out.Ops = append(out.Ops, tern.OpReveal(reveal, "nearest"))
	}
	out.route(m, cmds)
	return out
}

// route sorts the model's commands: intents become Ops, the editor round trip its own field, the rest daemon Cmds.
func (out *Output) route(m model.Model, cmds []model.Cmd) {
	for _, cmd := range cmds {
		switch cmd := cmd.(type) {
		case model.Scroll:
			if target := view.ScrollTarget(m, cmd.Column); target != "" {
				out.Ops = append(out.Ops, tern.OpScroll(target, string(cmd.By)))
			}
		case model.RevealActivityTab:
			for _, id := range view.ActivityReveals(m) {
				out.Ops = append(out.Ops, tern.OpReveal(id, "nearest"))
			}
		case model.OpenDraftEditor:
			out.Editor = &cmd
		default:
			out.Cmds = append(out.Cmds, cmd)
		}
	}
}
