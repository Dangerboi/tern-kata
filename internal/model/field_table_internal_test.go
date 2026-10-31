package model

import "testing"

// A card with fields declares them in a field table, and names each field once.
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
		seen := map[Field]bool{}
		for _, f := range e.fields {
			if f.Name == "" || seen[f.Name] {
				t.Errorf("%s declares the field %q twice or without a name", e.card.Name(), f.Name)
			}
			seen[f.Name] = true
		}
	}
}
