package driver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// hintIDs are the ids of the card on top's hint nodes, sorted.
func hintIDs(t *testing.T, m model.Model) []string {
	t.Helper()
	layer := cardLayerID(t, m)
	var ids []string
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if strings.HasPrefix(n.ID, layer+".") && strings.Contains(n.ID, "hint") {
			ids = append(ids, n.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

// A field's description is drawn whether or not the field has focus, so moving focus never adds or removes a line and the
// card keeps its height.
func TestCardHintsStayWhileFocusMoves(t *testing.T) {
	cards := map[string]func(t *testing.T) driver.State{
		"Create": func(t *testing.T) driver.State {
			return step(t, projectPane(t, 160, withDates("a", "", "")), key("n")).State
		},
		"Job": newJobCard,
	}
	for name, open := range cards {
		t.Run(name, func(t *testing.T) {
			s := open(t)
			want := hintIDs(t, s.Model)
			if len(want) == 0 {
				t.Fatal("the card draws no hint")
			}
			for range 12 {
				s = step(t, s, key("tab")).State
				if got := hintIDs(t, s.Model); !slices.Equal(got, want) {
					t.Fatalf("hints after Tab\n%v\nwant\n%v", got, want)
				}
			}
		})
	}
}
