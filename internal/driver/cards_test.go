package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// popup is one popup card for the precedence test: how to open it, how to put its state on another
// model, the mode it owns, the id of its layer, and what typing into it changed.
type popup struct {
	name  string
	open  func(*testing.T, driver.State) model.Model
	graft func(dst *model.Model, src model.Model)
	mode  model.Mode
	// head is the title the card draws on its layer.
	head  string
	typed func(model.Model) string
}

func keyOpen(chords ...string) func(*testing.T, driver.State) model.Model {
	return func(t *testing.T, s driver.State) model.Model {
		t.Helper()
		in := make([]any, len(chords))
		for i, c := range chords {
			in[i] = key(c)
		}
		return feed(t, s, in...).State.Model
	}
}

// popups lists the cards from the one on top down: the single precedence order the spec states.
func popups() []popup {
	return []popup{
		{"Create", keyOpen("n"), func(dst *model.Model, src model.Model) { dst.Create = src.Create },
			model.ModeCreate, "Create Issue", func(m model.Model) string { return m.Create.Title }},
		{"Draft editor", keyOpen("e"), func(dst *model.Model, src model.Model) { dst.Drafts, dst.EditorUID = src.Drafts, src.EditorUID },
			model.ModeEditor, "Edit Issue body", func(m model.Model) string { d, _ := m.ActiveDraft(); return d.Editor.Text }},
		{"single-field form", keyOpen("+"), func(dst *model.Model, src model.Model) { dst.FieldForm = src.FieldForm },
			model.ModeFieldForm, "Add label", func(m model.Model) string { return m.FieldForm.Value }},
		{"Close", keyOpen("x"), func(dst *model.Model, src model.Model) { dst.CloseForm = src.CloseForm },
			model.ModeCloseForm, "Close Issue", func(m model.Model) string {
				values := m.CloseForm.Values
				return values.Reason + values.Message + values.Commit + values.PR + values.Test + values.Reviewed + values.External + values.Reference + values.Rationale
			}},
		{"Dates", keyOpen("d"), func(dst *model.Model, src model.Model) { dst.DatesForm = src.DatesForm },
			model.ModeDatesForm, "Dates", func(m model.Model) string { return m.DatesForm.Text.Scheduled }},
		{"Move", keyOpen("shift+m"), func(dst *model.Model, src model.Model) { dst.MoveProject = src.MoveProject },
			model.ModeMoveProject, "Move #aaaa to another Project", func(m model.Model) string { return m.MoveProject.Filter }},
		{"Federation adoption", func(t *testing.T, _ driver.State) model.Model { return confirming(t).Model },
			func(dst *model.Model, src model.Model) { dst.Federation = src.Federation },
			model.ModeFederationAdopt, "Confirm Adoption", func(m model.Model) string { return m.Federation.AdoptInput }},
		{"Recurrence", func(t *testing.T, s driver.State) model.Model {
			m := keyOpen("shift+o")(t, s)
			m.RecurrenceForm.Field = model.RecurrenceZone // a text field
			return m
		}, func(dst *model.Model, src model.Model) { dst.RecurrenceForm = src.RecurrenceForm },
			model.ModeRecurrenceForm, "Make recurring", func(m model.Model) string { return m.RecurrenceForm.Zone }},
		{"Job", keyOpen("shift+j"), func(dst *model.Model, src model.Model) { dst.JobForm = src.JobForm },
			model.ModeJobForm, "New job", func(m model.Model) string { return m.JobForm.Name }},
		{"New Project", func(t *testing.T, s driver.State) model.Model {
			return step(t, s, model.CollectionPointerSelected{Key: model.NewProjectKey}).State.Model
		}, func(dst *model.Model, src model.Model) { dst.ProjectForm = src.ProjectForm },
			model.ModeProjectForm, "New Project", func(m model.Model) string { return m.ProjectForm.Name }},
		{"Lanes", func(t *testing.T, s driver.State) model.Model {
			return feed(t, s, key("b"), model.LanesLoaded{ProjectUID: "p"}, key("shift+b")).State.Model
		}, func(dst *model.Model, src model.Model) { dst.LanesForm = src.LanesForm },
			model.ModeLanesForm, "Lanes", func(m model.Model) string { return m.LanesForm.Text }},
		{"Filter form", func(t *testing.T, s driver.State) model.Model {
			m := keyOpen("f")(t, s)
			m.FilterForm.Field = model.FilterOwner // a text field
			return m
		}, func(dst *model.Model, src model.Model) { dst.FilterForm = src.FilterForm },
			model.ModeFilterForm, "Filter work", func(m model.Model) string { return m.FilterForm.Values.Owner }},
		{"Menu", func(t *testing.T, s driver.State) model.Model { return step(t, s, openMenu).State.Model },
			func(dst *model.Model, src model.Model) { dst.Picker = src.Picker },
			model.ModePicker, "Menu", func(m model.Model) string { return m.Picker.Filter }},
	}
}

// Every pair of cards that can be open at once agrees on which one is on top, seen only through Step: the mode, the card
// that takes the keys, the card whose layer is drawn (no other card's layer is drawn above it or at all) and the card holding
// the text caret.
func TestPopupCardPrecedenceIsOneOrder(t *testing.T) {
	cards := popups()
	base := browsing(t, 160)
	opened := make([]model.Model, len(cards))
	for i, c := range cards {
		opened[i] = c.open(t, base)
		if opened[i].CurrentMode() != c.mode {
			t.Fatalf("%s opened in mode %s, want %s", c.name, opened[i].CurrentMode(), c.mode)
		}
	}
	for i := range cards {
		for j := i + 1; j < len(cards); j++ {
			top, under := cards[i], cards[j] // earlier in the list is on top
			t.Run(top.name+" over "+under.name, func(t *testing.T) {
				m := base.Model
				under.graft(&m, opened[j])
				top.graft(&m, opened[i])
				out := step(t, driver.State{Model: m}, model.ResizeMsg{Cols: 160})
				if got := out.State.Model.CurrentMode(); got != top.mode {
					t.Errorf("mode %s, want %s", got, top.mode)
				}
				layer := layerTitled(t, out.State.Model, top.head)
				if !strings.HasPrefix(out.State.Focus, layer) {
					t.Errorf("text focus %q is not on %s (%s)", out.State.Focus, top.name, layer)
				}
				drawn := drawnIDs(t, *out.View)
				var layers []string
				for id := range drawn {
					if strings.HasPrefix(id, "layer.") {
						layers = append(layers, id)
						if id != layer && !strings.HasPrefix(id, layer+".") {
							t.Errorf("drew %q, which is not part of %s", id, top.name)
						}
					}
				}
				if len(layers) == 0 {
					t.Errorf("%s was not drawn", top.name)
				}
				got := step(t, out.State, tern.Key{Name: "z", Text: "z"}).State.Model
				if !strings.Contains(top.typed(got), "z") || strings.Contains(under.typed(got), "z") {
					t.Errorf("a key reached %s=%q, %s=%q; want only the top card", top.name, top.typed(got), under.name, under.typed(got))
				}
			})
		}
	}
}

// Tern names a body editor by node id; the model hears which card's body it is.
func TestBodyEditsAndUndoReachTheirCard(t *testing.T) {
	create := feed(t, browsing(t, 160), key("n"))
	createBody := fieldLabeled(t, create.State.Model, "Body (optional)")
	create = step(t, create.State, &tern.Focus{ID: createBody})
	draft := feed(t, browsing(t, 160), key("e"))
	for _, tt := range []struct {
		name string
		s    driver.State
		id   string
		text func(model.Model) string
	}{
		{"Create", create.State, createBody, func(m model.Model) string { return m.Create.Editor.Text }},
		{"Draft editor", draft.State, cardInput(t, draft.State.Model), func(m model.Model) string { d, _ := m.ActiveDraft(); return d.Editor.Text }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			edited := step(t, tt.s, &tern.Edit{ID: tt.id, Text: "hi", Cursor: 2})
			if got := tt.text(edited.State.Model); !strings.HasSuffix(got, "hi") {
				t.Fatalf("after an edit: %q", got)
			}
			undone := step(t, edited.State, &tern.Undo{ID: tt.id})
			if got := tt.text(undone.State.Model); strings.Contains(got, "hi") {
				t.Fatalf("after undo: %q", got)
			}
			ignored := step(t, tt.s, &tern.Edit{ID: "layer.nowhere.body", Text: "hi", Cursor: 2})
			if got := tt.text(ignored.State.Model); got != tt.text(tt.s.Model) {
				t.Fatalf("an edit for another node changed the body: %q", got)
			}
		})
	}
}
