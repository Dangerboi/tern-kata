package view_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// borders is every bordered container in a rendering by id: "accent" when
// Tern draws its ring in the accent tone (Selected), "muted" otherwise.
func borders(nodes shown) map[string]string {
	out := map[string]string{}
	for id, n := range nodes {
		if n.K == "card" {
			out[id] = "muted"
			if selected, _ := n.P["selected"].(bool); selected {
				out[id] = "accent"
			}
		}
	}
	return out
}

func TestSideBySidePanelsAreBordered(t *testing.T) {
	m := browsing()
	if m.Focus != model.ColumnList {
		t.Fatalf("fixture: focus %v, want the list", m.Focus)
	}
	got := borders(render(t, m))
	if want := map[string]string{"main.columns.list": "accent", "main.columns.detail": "muted"}; !maps.Equal(got, want) {
		t.Errorf("list focused: borders %v, want %v", got, want)
	}

	m = apply(m, model.KeyMsg{Chord: "enter"})
	if m.Focus != model.ColumnDetail {
		t.Fatalf("fixture: focus %v after enter, want the detail", m.Focus)
	}
	got = borders(render(t, m))
	if want := map[string]string{"main.columns.list": "muted", "main.columns.detail": "accent"}; !maps.Equal(got, want) {
		t.Errorf("detail focused: borders %v, want %v", got, want)
	}
}

func TestTopBarSitsOutsideBothBorders(t *testing.T) {
	nodes := render(t, browsing())
	for _, id := range []string{"main.topbar", "main.topbar.heading"} {
		if _, ok := nodes[id]; !ok {
			t.Errorf("%s is missing", id)
		}
	}
	for _, panel := range []string{"main.columns.list", "main.columns.detail"} {
		if slices.ContainsFunc(nodes.under(panel), func(id string) bool { return id == "main.topbar" }) {
			t.Errorf("top bar is inside %s", panel)
		}
	}
}

func TestSingleColumnHasNoBorders(t *testing.T) {
	for _, cols := range []int{model.SideBySideCols - 1, model.MinCols} {
		for _, focus := range []string{"", "enter"} {
			m := apply(browsing(), model.ResizeMsg{Cols: cols})
			if focus != "" {
				m = apply(m, model.KeyMsg{Chord: focus})
			}
			if got := borders(render(t, m)); len(got) != 0 {
				t.Errorf("%d columns, focus %q: borders %v, want none", cols, focus, got)
			}
		}
	}
}
