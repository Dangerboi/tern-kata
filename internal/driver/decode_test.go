package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// An action's value reaches the model only when Tern sent it as a string: a number, a boolean, an object or nothing leaves the
// row folded, where the string value unfolds it.
func TestAnActionValueReachesTheModelOnlyWhenItIsAString(t *testing.T) {
	act, _, _ := strings.Cut(view.ToggleClick("parent"), "=")
	folded := len(clickBase().Model.VisibleRows())
	for _, tt := range []struct {
		name  string
		value any
		rows  int
	}{
		{"string", "parent", folded + 1},
		{"number", float64(3), folded},
		{"bool", true, folded},
		{"object", map[string]any{"col": "title"}, folded},
		{"nil", nil, folded},
	} {
		s := clickBase()
		out := step(t, s, &tern.Action{ID: rowOf(t, s.Model, view.IssueListID, "parent"), Act: act, Value: tt.value})
		if got := len(out.State.Model.VisibleRows()); got != tt.rows {
			t.Errorf("%s: %d rows shown, want %d", tt.name, got, tt.rows)
		}
	}
}
