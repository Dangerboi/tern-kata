package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// cardOpeners opens each popup card by its own key from a state where it can open. A card of the model's card table with no
// opener here fails the test that walks the table.
func cardOpeners() map[model.Card]func(*testing.T) driver.State {
	return map[model.Card]func(*testing.T) driver.State{
		model.CardCreate:     popupOpener("Create"),
		model.CardDraft:      popupOpener("Draft editor"),
		model.CardField:      popupOpener("single-field form"),
		model.CardClose:      popupOpener("Close"),
		model.CardDates:      popupOpener("Dates"),
		model.CardRecurrence: popupOpener("Recurrence"),
		model.CardJob:        popupOpener("Job"),
		model.CardProject:    popupOpener("New Project"),
		model.CardLanes:      popupOpener("Lanes"),
		model.CardFilter:     popupOpener("Filter form"),
		model.CardPicker:     popupOpener("Menu"),
		model.CardWorkflow: func(t *testing.T) driver.State {
			return step(t, workflowsPane(t, 160, reviewFlow()), key("e")).State
		},
		model.CardBackfill: func(t *testing.T) driver.State {
			return step(t, jobsPane(t, 160, disabled(owing("J1", "open", "all"))), key("p")).State
		},
		model.CardWorker: func(t *testing.T) driver.State { return workerCardOpen(t, nil, noWorkerFile) },
	}
}

// Every card of the model's card table, opened by its own key, is drawn as a layer named from the table, and Tern's focus
// input on each field node it draws moves the card's own focus to that field.
func TestEveryCardIsDrawnAndItsFieldNodesMoveItsFocus(t *testing.T) {
	openers := cardOpeners()
	focused := map[model.Card]func(model.Model) model.Field{}
	for _, c := range editingCards() {
		focused[c.card] = c.focused
	}
	for _, card := range model.Cards() {
		open, ok := openers[card]
		if !ok {
			t.Fatalf("card %q has no opener in this test", card.Name())
		}
		t.Run(card.Name(), func(t *testing.T) {
			s := open(t)
			if got := s.Model.TopCard(); got != card {
				t.Fatalf("opened card %q, want %q", got.Name(), card.Name())
			}
			drawn := drawnIDs(t, view.Render(s.Model, now))
			if id := view.LayerID(card); !drawn[id] {
				t.Fatalf("no layer %q drawn", id)
			}
			fields, _ := model.CardFields(card)
			if model.CardInput(card) {
				id := view.FieldID(card, "")
				if !drawn[id] {
					t.Errorf("the card's one input %q is not drawn", id)
				}
				if got, field, ok := view.FieldFromID(id); !ok || got != card || field != "" {
					t.Errorf("node %q maps back to %q field %q (%v)", id, got.Name(), field, ok)
				}
			}
			if len(fields) > 0 && focused[card] == nil {
				t.Fatalf("card %q has fields and no way to read its focused field here", card.Name())
			}
			seen := 0
			for _, f := range fields {
				id := view.FieldID(card, f.Name)
				if !drawn[id] {
					continue // a field the card does not show as it stands has no node to focus
				}
				seen++
				if got, field, ok := view.FieldFromID(id); !ok || got != card || field != f.Name {
					t.Errorf("node %q maps back to %q field %q (%v)", id, got.Name(), field, ok)
				}
				out := step(t, s, &tern.Focus{ID: id})
				if got := focused[card](out.State.Model); got != f.Name {
					t.Errorf("focus input on %q left field %q, want %q", id, got, f.Name)
				}
				if out.State.Model.TopCard() != card {
					t.Errorf("focus input on %q changed the card on top to %q", id, out.State.Model.TopCard().Name())
				}
			}
			if len(fields) > 0 && seen == 0 {
				t.Errorf("none of the card's fields %v is drawn", fields)
			}
		})
	}
}
