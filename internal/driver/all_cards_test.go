package driver_test

import (
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
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
		model.CardMove:       popupOpener("Move"),
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

// Every card of the model's card table, opened by its own key, is drawn as a layer of its own, and Tern's focus input on each
// field node it draws moves the card's own focus to a field of its list, a different one for each node.
func TestEveryCardIsDrawnAndItsFieldNodesMoveItsFocus(t *testing.T) {
	openers := cardOpeners()
	focused := map[model.Card]func(model.Model) model.Field{}
	for _, c := range editingCards() {
		focused[c.card] = c.focused
	}
	layers := map[string]string{}
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
			layer := cardLayerID(t, s.Model)
			if other, dup := layers[layer]; dup {
				t.Fatalf("cards %q and %q are both drawn as %q", other, card.Name(), layer)
			}
			layers[layer] = card.Name()
			specs, _ := model.CardFields(card)
			if model.CardInput(card) {
				cardInput(t, s.Model)
			}
			nodes := cardFields(t, s.Model)
			if len(specs) > 0 && len(nodes) == 0 {
				t.Fatalf("none of the card's fields %v is drawn", specs)
			}
			if len(specs) > 0 && focused[card] == nil {
				t.Fatalf("card %q has fields and no way to read its focused field here", card.Name())
			}
			reached := map[model.Field]string{}
			for _, n := range nodes {
				if len(specs) == 0 {
					break // the one input of a card without a field list has no focus to move
				}
				out := step(t, s, &tern.Focus{ID: n.ID})
				got := focused[card](out.State.Model)
				if !slices.ContainsFunc(specs, func(f model.FieldSpec) bool { return f.Name == got }) {
					t.Errorf("focus input on %q left field %q, which the card does not list", n.ID, got)
				}
				if other, dup := reached[got]; dup {
					t.Errorf("focus input on %q and on %q both left field %q", other, n.ID, got)
				}
				reached[got] = n.ID
				if out.State.Model.TopCard() != card {
					t.Errorf("focus input on %q changed the card on top to %q", n.ID, out.State.Model.TopCard().Name())
				}
			}
		})
	}
}
