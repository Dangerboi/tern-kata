package model

import "testing"

// A card with fields declares them in a field table, and the table is the only source of its busy condition, focus, text,
// erase and choice behaviour: no such card sets them by hand.
func TestEveryCardWithFieldsDeclaresAFieldTable(t *testing.T) {
	for _, e := range buildCards() {
		if len(e.fields) == 0 {
			continue
		}
		if e.table == nil {
			t.Errorf("%s has fields and no field table", e.card.Name())
		}
	}
	for _, e := range buildCards() {
		if e.table == nil {
			continue
		}
		if e.busy == nil || e.focus == nil || e.text == nil || e.erased == nil {
			t.Errorf("%s has a field table that did not fill its entry", e.card.Name())
		}
	}
}
