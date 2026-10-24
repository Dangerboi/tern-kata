package driver_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// editingCard is a popup card that has fields: how to open it, which field it has focused, and the fields one lap of tab
// visits from the opening, in order. hidden are the fields of the card's list that the card does not show as it opens.
type editingCard struct {
	name    string
	card    model.Card
	open    func(*testing.T) driver.State
	focused func(model.Model) model.Field
	lap     []model.Field
	hidden  []model.Field
}

func popupOpener(name string) func(*testing.T) driver.State {
	return func(t *testing.T) driver.State {
		t.Helper()
		for _, c := range popups() {
			if c.name == name {
				return step(t, driver.State{Model: c.open(t, browsing(t, 160))}, resize(160)).State
			}
		}
		t.Fatalf("no popup %q", name)
		return driver.State{}
	}
}

func editingCards() []editingCard {
	return []editingCard{
		{name: "Create", card: model.CardCreate, open: popupOpener("Create"),
			focused: func(m model.Model) model.Field { return m.Create.Field },
			lap:     []model.Field{model.CreateBody, model.CreateLabels, model.CreateOwner, model.CreatePriority, model.CreateScheduled, model.CreateDeadline, model.CreateTitle}},
		{name: "Close", card: model.CardClose, open: popupOpener("Close"),
			focused: func(m model.Model) model.Field { return m.CloseForm.Field },
			lap:     []model.Field{model.CloseMessage, model.CloseCommit, model.ClosePR, model.CloseTest, model.CloseReference, model.CloseRationale, model.CloseReason}},
		{name: "Dates", card: model.CardDates, open: popupOpener("Dates"),
			focused: func(m model.Model) model.Field { return m.DatesForm.Field },
			lap:     []model.Field{model.DatesDeadline, model.DatesScheduled}},
		{name: "Recurrence", card: model.CardRecurrence, open: popupOpener("Recurrence"),
			focused: func(m model.Model) model.Field { return m.RecurrenceForm.Field },
			hidden:  []model.Field{model.RecurrenceDay, model.RecurrenceMonth, model.RecurrenceEnd},
			lap:     []model.Field{model.RecurrenceFrequency, model.RecurrenceEvery, model.RecurrenceWeekdays, model.RecurrenceStart, model.RecurrenceEnds, model.RecurrenceZone}},
		{name: "Job", card: model.CardJob, open: popupOpener("Job"),
			focused: func(m model.Model) model.Field { return m.JobForm.Field },
			hidden:  []model.Field{model.JobFieldInterval, model.JobFieldAt, model.JobFieldIssue, model.JobFieldReport, model.JobFieldPrompt},
			lap: []model.Field{model.JobFieldTrigger, model.JobFieldCron, model.JobFieldZone, model.JobFieldParent, model.JobFieldTitle, model.JobFieldBody,
				model.JobFieldRecipient, model.JobFieldMessage, model.JobFieldCatchup, model.JobFieldEnabled, model.JobFieldKind, model.JobFieldName}},
		{name: "Lanes", card: model.CardLanes, open: popupOpener("Lanes"),
			focused: func(m model.Model) model.Field { return m.LanesForm.Field },
			lap:     []model.Field{model.LanesDays, model.LanesCards, model.LanesNames}},
		{name: "Worker", card: model.CardWorker,
			open:    func(t *testing.T) driver.State { return workerCardOpen(t, nil, noWorkerFile) },
			focused: func(m model.Model) model.Field { return m.WorkerCard.Field },
			lap:     []model.Field{model.WorkerFieldActor, model.WorkerFieldCommand, model.WorkerFieldModel, model.WorkerFieldThinking, model.WorkerFieldTrust, model.WorkerFieldExecutor}},
		{name: "Filter", card: model.CardFilter, open: popupOpener("Filter form"),
			focused: func(m model.Model) model.Field { return m.FilterForm.Field },
			lap:     []model.Field{model.FilterLabel, model.FilterRelationship, model.FilterProject, model.FilterStatus, model.FilterOwner}},
	}
}

// takesText says the card's field is a single-line text field.
func (c editingCard) takesText(field model.Field) bool {
	specs, _ := model.CardFields(c.card)
	return slices.ContainsFunc(specs, func(f model.FieldSpec) bool { return f.Name == field && f.Text })
}

// caretID is the node Tern is told to focus while the card has field focused: the field's own when it takes text, none for a
// choice field.
func (c editingCard) caretID(field model.Field) string {
	if c.takesText(field) {
		return view.FieldID(c.card, field)
	}
	return ""
}

// wantFocusOps is what one key sends Tern: the focus op when the focused node changed, nothing otherwise.
func wantFocusOps(from, to driver.State) []tern.Op {
	if from.Focus == to.Focus {
		return nil
	}
	return []tern.Op{tern.OpFocus(to.Focus)}
}

func opsOf(out driver.Output) []tern.Op {
	if len(out.Ops) == 0 {
		return nil
	}
	return out.Ops
}

// seekField tabs until the card has field focused.
func (c editingCard) seekField(t *testing.T, s driver.State, field model.Field) driver.State {
	t.Helper()
	for range len(c.lap) + 1 {
		if c.focused(s.Model) == field {
			return s
		}
		s = step(t, s, key("tab")).State
	}
	t.Fatalf("%s: tab never reaches %s", c.name, field)
	return s
}

func drawnNodes(t *testing.T, s driver.State) []tern.WireNode {
	return wireNodes(t, view.Render(s.Model, now))
}

// Tab and shift+tab walk the card's fields in order, skip the fields the card hides, and every move sends Tern the focus op for
// the rendered focused field: its own node when it takes text, none for a choice field.
func TestTabMovesTheFocusedFieldAndSendsTheFocusOp(t *testing.T) {
	for _, c := range editingCards() {
		t.Run(c.name, func(t *testing.T) {
			s := c.open(t)
			start := c.focused(s.Model)
			if s.Focus != c.caretID(start) {
				t.Fatalf("opened with focus %q, want %q", s.Focus, c.caretID(start))
			}
			for _, want := range c.lap {
				out := step(t, s, key("tab"))
				if got := c.focused(out.State.Model); got != want {
					t.Fatalf("tab from %s reached %s, want %s", c.focused(s.Model), got, want)
				}
				if slices.Contains(c.hidden, want) {
					t.Fatalf("tab reached the hidden field %s", want)
				}
				if out.State.Focus != c.caretID(want) {
					t.Fatalf("on %s the focus is %q, want %q", want, out.State.Focus, c.caretID(want))
				}
				if got, want := opsOf(out), wantFocusOps(s, out.State); !reflect.DeepEqual(got, want) {
					t.Fatalf("tab onto %s sent %v, want %v", want, got, want)
				}
				s = out.State
			}
			if c.focused(s.Model) != start {
				t.Fatalf("one lap of tab ends on %s, want the start %s", c.focused(s.Model), start)
			}
			for i := len(c.lap) - 1; i >= 0; i-- { // shift+tab walks the lap backwards
				want := start
				if i > 0 {
					want = c.lap[i-1]
				}
				out := step(t, s, key("shift+tab"))
				if got := c.focused(out.State.Model); got != want {
					t.Fatalf("shift+tab from %s reached %s, want %s", c.focused(s.Model), got, want)
				}
				if out.State.Focus != c.caretID(want) {
					t.Fatalf("on %s the focus is %q, want %q", want, out.State.Focus, c.caretID(want))
				}
				if got, wantOps := opsOf(out), wantFocusOps(s, out.State); !reflect.DeepEqual(got, wantOps) {
					t.Fatalf("shift+tab onto %s sent %v, want %v", want, got, wantOps)
				}
				s = out.State
			}
		})
	}
}

// In a text field typing, space, backspace and ctrl+u change the rendered text and send nothing: the caret stays in the field.
func TestTextKeysEditTheRenderedFocusedField(t *testing.T) {
	for _, c := range editingCards() {
		for _, field := range append([]model.Field{c.focused(c.open(t).Model)}, c.lap...) {
			if !c.takesText(field) || (c.card == model.CardCreate && field == model.CreateBody) {
				continue
			}
			t.Run(c.name+"/"+string(field), func(t *testing.T) {
				s := c.seekField(t, c.open(t), field)
				id := view.FieldID(c.card, field)
				steps := []struct {
					key  string
					want string
				}{{"ctrl+u", ""}, {"a", "a"}, {"b", "ab"}, {"c", "abc"}, {"backspace", "ab"}, {"space", "ab "}, {"backspace", "ab"}, {"ctrl+u", ""}, {"backspace", ""}}
				for _, st := range steps {
					out := step(t, s, key(st.key))
					text, ok := nodeText(t, out.State, id)
					if !ok || text != st.want {
						t.Fatalf("after %s the field draws %q (drawn %v), want %q", st.key, text, ok, st.want)
					}
					if out.State.Focus != id || len(out.Ops) != 0 {
						t.Fatalf("after %s focus %q ops %v, want the caret to stay on %s", st.key, out.State.Focus, out.Ops, id)
					}
					s = out.State
				}
			})
		}
	}
}

// In a choice field right, left and space step the choice and send nothing; backspace and ctrl+u leave the card's focus where it is.
func TestChoiceKeysStepTheRenderedChoice(t *testing.T) {
	for _, c := range editingCards() {
		for _, field := range c.lap {
			if c.takesText(field) {
				continue
			}
			t.Run(c.name+"/"+string(field), func(t *testing.T) {
				s := c.seekField(t, c.open(t), field)
				before := drawnNodes(t, s)
				next := step(t, s, key("right"))
				if reflect.DeepEqual(drawnNodes(t, next.State), before) {
					t.Errorf("right left the %s choice as it was", field)
				}
				back := step(t, next.State, key("left"))
				if !reflect.DeepEqual(drawnNodes(t, back.State), before) {
					t.Errorf("left after right did not restore the %s choice", field)
				}
				space := step(t, s, key("space"))
				switch {
				case field == model.RecurrenceWeekdays: // right moves along the days; space toggles the day under it
					if reflect.DeepEqual(drawnNodes(t, space.State), before) {
						t.Errorf("space left the %s choice as it was", field)
					}
				case !reflect.DeepEqual(drawnNodes(t, space.State), drawnNodes(t, next.State)):
					t.Errorf("space and right step the %s choice differently", field)
				}
				for _, out := range []driver.Output{next, back, space} {
					if len(out.Ops) != 0 || out.State.Focus != "" || c.focused(out.State.Model) != field {
						t.Errorf("a choice key sent %v, focus %q, field %s; want nothing sent and the focus kept", out.Ops, out.State.Focus, c.focused(out.State.Model))
					}
				}
				for _, k := range []string{"backspace", "ctrl+u"} {
					out := step(t, s, key(k))
					if len(out.Ops) != 0 || out.State.Focus != "" || c.focused(out.State.Model) != field {
						t.Errorf("%s sent %v, focus %q, field %s; want nothing sent and the focus kept", k, out.Ops, out.State.Focus, c.focused(out.State.Model))
					}
				}
			})
		}
	}
}
