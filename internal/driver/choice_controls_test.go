package driver_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// Each choice field of a popup card is drawn as the control the decided mapping names, inside a FormField that is the field's
// node; a field that takes several values at once stays a row of words.
func TestChoiceFieldsDrawTheirControl(t *testing.T) {
	want := map[string]map[model.Field]string{
		"Recurrence": {model.RecurrenceFrequency: "select", model.RecurrenceEnds: "select"},
		"Job":        {model.JobFieldKind: "select", model.JobFieldTrigger: "select", model.JobFieldCatchup: "select", model.JobFieldEnabled: "switch"},
		"Lanes":      {model.LanesShare: "tabs"},
		"Worker":     {model.WorkerFieldTrust: "switch"},
		"Filter":     {model.FilterProject: "select", model.FilterStatus: "tabs", model.FilterRelationship: "select"},
		"View":       {model.ViewBase: "select", model.FilterProject: "select", model.FilterStatus: "tabs", model.FilterRelationship: "select"},
	}
	for _, c := range editingCards() {
		fields := want[c.name]
		if fields == nil {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			s := c.open(t)
			kinds := map[string]string{}
			for _, n := range wireNodes(t, view.Render(s.Model, now)) {
				kinds[n.ID] = n.K
			}
			for field, kind := range fields {
				id := view.FieldID(c.card, field)
				if kinds[id] != "field" {
					t.Errorf("%s is drawn as %q, want a FormField", field, kinds[id])
				}
				if got := kinds[id+".control"]; got != kind {
					t.Errorf("%s control is drawn as %q, want %q", field, got, kind)
				}
			}
		})
	}
	weekdays := editingCards()[5].open(t)
	for _, n := range wireNodes(t, view.Render(weekdays.Model, now)) {
		if n.ID == view.FieldID(model.CardRecurrence, model.RecurrenceWeekdays)+".control" {
			t.Errorf("Weekdays is drawn as a %s control", n.K)
		}
	}
}

// A pick in a select or a tab strip and a flip of a switch are the Change, Select and Toggle events Tern sends; they set the
// field as the keys do.
func TestControlEventsSetTheChoice(t *testing.T) {
	cards := map[string]editingCard{}
	for _, c := range editingCards() {
		cards[c.name] = c
	}
	node := func(c editingCard, field model.Field) string { return view.FieldID(c.card, field) + ".control" }
	t.Run("select pick", func(t *testing.T) {
		c := cards["Filter"]
		s := c.open(t)
		out := step(t, s, &tern.Change{ID: node(c, model.FilterRelationship), Value: "2"})
		if got := out.State.Model.FilterForm.Values.Relationship; got != "child" {
			t.Fatalf("relationship = %q, want child", got)
		}
		if out.State.Model.FilterForm.Field != model.FilterRelationship {
			t.Fatalf("focus %q, want the picked field", out.State.Model.FilterForm.Field)
		}
	})
	t.Run("tab pick", func(t *testing.T) {
		c := cards["Lanes"]
		s := c.open(t)
		out := step(t, s, &tern.Select{ID: node(c, model.LanesShare), Item: "1"})
		if !out.State.Model.LanesForm.Share {
			t.Fatal("the Project tab did not set Save to")
		}
		out = step(t, out.State, &tern.Select{ID: node(c, model.LanesShare), Item: node(c, model.LanesShare) + ".0"})
		if out.State.Model.LanesForm.Share {
			t.Fatal("a tab named by its whole node id did not set Save to")
		}
	})
	t.Run("switch flip", func(t *testing.T) {
		c := cards["Worker"]
		s := c.open(t)
		on := step(t, s, &tern.Change{ID: node(c, model.WorkerFieldTrust), Value: true}).State
		if !on.Model.WorkerCard.Trusted {
			t.Fatal("flipping the switch on did not trust")
		}
		off := step(t, on, &tern.Change{ID: node(c, model.WorkerFieldTrust), Value: false}).State
		if off.Model.WorkerCard.Trusted {
			t.Fatal("flipping the switch off kept the trust")
		}
		jc := cards["Job"]
		js := jc.open(t)
		flipped := step(t, js, &tern.Change{ID: node(jc, model.JobFieldEnabled), Value: false}).State
		if flipped.Model.JobForm.Enabled {
			t.Fatal("flipping Enabled off left the job enabled")
		}
	})
	t.Run("pointer opens and closes the menu", func(t *testing.T) {
		c := cards["Recurrence"]
		s := c.open(t)
		opened := step(t, s, &tern.Toggle{ID: node(c, model.RecurrenceEnds), Open: true}).State
		if !opened.Model.OpenMenu(model.CardRecurrence, model.RecurrenceEnds).Open || opened.Model.RecurrenceForm.Field != model.RecurrenceEnds {
			t.Fatal("the pointer's open did not open the menu on its field")
		}
		closed := step(t, opened, &tern.Toggle{ID: node(c, model.RecurrenceEnds), Open: false}).State
		if closed.Model.OpenMenu(model.CardRecurrence, model.RecurrenceEnds).Open {
			t.Fatal("the pointer's close left the menu open")
		}
		if !reflect.DeepEqual(step(t, s, &tern.Toggle{ID: "main.columns", Open: true}).State.Model, s.Model) {
			t.Fatal("a toggle on another node changed the model")
		}
	})
}
