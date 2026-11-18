// Package driver is the Pane driver: one pure step from a Tern input or a
// daemon result to the next state, the view to draw, the Tern ops to send and
// the daemon commands to run. It decodes, calls the model, renders and routes;
// the view decides which focus, reveal and scroll targets are safe to name,
// because Tern answers an op on a node it does not draw with an error event
// that ends the program.
package driver

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// frameRace is how long before an editing on a frame the Pane sent still counts as one sent after it: Tern emitted the editing on
// before it got that frame, and the frame may wait for a credit the on's ack returns.
const frameRace = 100 * time.Millisecond

type drawn struct {
	view *tern.View
	at   time.Time
}

type State struct {
	Model model.Model
	Focus string
	// standing and recent are the views the Pane drew: recent those of the last frameRace, standing the one before them (nil after the
	// surface was replaced), so an editing on can tell whether a frame was on its way.
	standing *tern.View
	recent   []drawn
	// HeldOps are the ops the Pane kept back while it held its frames (the SDK sends an op as a frame, which ends a doc edit), in
	// order and with only the last focus; the step that releases the hold sends them after its view.
	HeldOps []tern.Op
}

// Fresh is the state after the surface was replaced: Tern holds no caret, so
// the next step sends the text focus again.
func (s State) Fresh() State {
	s.Focus, s.standing, s.recent, s.HeldOps = "", nil, nil, nil
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
	editingOn := false
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
		if s.Model.HoldsFrames() && leavesDoc(in) {
			s.Model = s.Model.ReleaseFrames()
		}
		if userInput(in) {
			s.Model = s.Model.EndDocGuard() // the pointer acted, or the user changed a value or the body: they know where they are
		}
		msg = decode(in)
		e, ok := in.(*tern.Editing)
		editingOn = ok && e.On
	}
	if msg == nil {
		return Output{State: s}
	}

	previousSelection := s.Model.CurrentSelection()
	s.Model.Now = now
	m, cmds := model.Update(s.Model, msg)
	if editingOn && m.HoldsFrames() && s.frameWithinRace(now) {
		m = m.FramedBeforeEditing()
	}
	s.Model = m
	if m.Quitting {
		return Output{State: s, Quit: true}
	}
	out := Output{State: s}
	if !m.HoldsFrames() {
		v := view.Render(m, now)
		out.View = &v
		out.State.noteDrawn(&v, now)
	}
	out.route(m, cmds)
	if target := view.TextFocusID(m); target != s.Focus {
		out.Ops = append(out.Ops, tern.OpFocus(target))
		out.State.Focus = target
	}
	if reveal := view.SelectionRevealID(m, msg, previousSelection); reveal != "" {
		out.Ops = append(out.Ops, tern.OpReveal(reveal, "nearest"))
	}
	pending := withLastFocus(append(slices.Clone(s.HeldOps), out.Ops...))
	if m.HoldsFrames() {
		out.State.HeldOps, out.Ops = pending, nil
	} else {
		out.State.HeldOps, out.Ops = nil, pending
	}
	return out
}

// noteDrawn records that the Pane drew v at at, and moves the views older than frameRace into standing.
func (s *State) noteDrawn(v *tern.View, at time.Time) {
	cut := at.Add(-frameRace)
	keep := 0
	for keep < len(s.recent) && s.recent[keep].at.Before(cut) {
		s.standing = s.recent[keep].view
		keep++
	}
	s.recent = append(slices.Clone(s.recent[keep:]), drawn{view: v, at: at})
}

// frameWithinRace reports whether a view the Pane drew in the last frameRace differs from the one before it: a frame went out, or
// is waiting for a credit. Only the editing on asks, so the views are compared only then.
func (s State) frameWithinRace(now time.Time) bool {
	cut := now.Add(-frameRace)
	before := s.standing
	for _, d := range s.recent {
		if d.at.Before(cut) {
			before = d.view
			continue
		}
		if before == nil || !reflect.DeepEqual(*before, *d.view) {
			return true
		}
		before = d.view
	}
	return false
}

// withLastFocus drops every focus op but the last: the caret ends where the last one puts it.
func withLastFocus(ops []tern.Op) []tern.Op {
	last := -1
	for i, op := range ops {
		if isFocusOp(op) {
			last = i
		}
	}
	kept := ops[:0]
	for i, op := range ops {
		if isFocusOp(op) && i != last {
			continue
		}
		kept = append(kept, op)
	}
	return kept
}

func isFocusOp(op tern.Op) bool { return len(op) > 0 && op[0] == "focus" }

// leavesDoc reports whether in shows the user is outside the doc they edit: a key reaches the Pane only when the doc does not have
// it, and a pointer action, a focus, a comment or a change elsewhere is not the doc's own typing.
func leavesDoc(in tern.Input) bool {
	switch in := in.(type) {
	case tern.Key:
		return true
	case *tern.Change:
		return !view.IsBodyDoc(in.ID)
	case *tern.Select:
		return !view.IsBodyDoc(in.ID)
	case *tern.Focus:
		return !view.IsBodyDoc(in.ID)
	}
	return userInput(in)
}

// userInput reports whether in is the user acting, not the window half relaying what it saw: the agent-states relay comes on every
// agent state event, whatever the user does.
func userInput(in tern.Input) bool {
	switch in := in.(type) {
	case *tern.Select, *tern.Focus, *tern.Toggle, *tern.Comment, *tern.Change:
		return true
	case *tern.Action:
		return in.Act != string(model.AgentStatesAction)
	}
	return false
}

// route turns the step's Cmds into ops and the Output's Cmds and Editor. Its ops go ahead of the step's focus op: a menu the
// pointer opened stayed open when its close came behind a focus op (tab moving to the next field).
func (out *Output) route(m model.Model, cmds []model.Cmd) {
	for _, cmd := range cmds {
		switch cmd := cmd.(type) {
		case model.Scroll:
			if target := view.ScrollTarget(m, cmd.Column); target != "" {
				out.Ops = append(out.Ops, tern.OpScroll(target, string(cmd.By)))
			}
		case model.RevealActivityTab:
			if id := view.ActivityRevealID(m); id != "" {
				out.Ops = append(out.Ops, tern.OpReveal(id, "start"))
			}
		case model.CloseSelectMenu:
			// The Pane drew the menu closed (no open prop) while the pointer had it open, so drawing it closed again changes
			// nothing Tern would apply; setting the prop and deleting it again is a close Tern applies.
			if id := view.CloseSelectTarget(m, cmd.Card, cmd.Field); id != "" {
				out.Ops = append(out.Ops, tern.OpSet(id, map[string]any{"open": true}), tern.OpSet(id, map[string]any{"open": nil}))
			}
		case model.OpenDraftEditor:
			out.Editor = &cmd
		default:
			out.Cmds = append(out.Cmds, cmd)
		}
	}
}
