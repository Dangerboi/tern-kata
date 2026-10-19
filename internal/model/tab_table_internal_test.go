package model

import "testing"

func TestEveryActivityTabHasATableEntry(t *testing.T) {
	seen := map[RowKind]ActivityTab{}
	for tab := range TabCount {
		e := tabTable[tab]
		if e.label == "" || e.empty == "" || e.kind == RowChild {
			t.Errorf("tab %d has no complete entry in the tab table: %+v", tab, e)
		}
		if other, dup := seen[e.kind]; dup {
			t.Errorf("tabs %d and %d list the same row kind", other, tab)
		}
		seen[e.kind] = tab
		if e.kind.Tab() != tab {
			t.Errorf("row kind of tab %d belongs to tab %d", tab, e.kind.Tab())
		}
	}
}
