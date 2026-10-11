package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func TestListTabFocusesDetailSideBySide(t *testing.T) {
	m := clickBase()
	if m.Focus != model.ColumnList {
		t.Fatalf("start focus = %v", m.Focus)
	}
	m, cmds := clickPress(m, "tab")
	if m.Focus != model.ColumnDetail || len(cmds) != 0 {
		t.Fatalf("tab: focus=%v commands=%#v", m.Focus, cmds)
	}
}

func TestListTabDoesNothingSingleColumn(t *testing.T) {
	m, _ := model.Update(clickBase(), model.ResizeMsg{Cols: 100})
	got, _ := clickPress(m, "tab")
	if got.Focus != model.ColumnList || got.DetailHidden() {
		t.Fatalf("single column: focus=%v hidden=%v", got.Focus, got.DetailHidden())
	}
}

func TestListTabDoesNothingWithoutDetail(t *testing.T) {
	hidden, _ := clickPress(clickBase(), "shift+l")
	if got, _ := clickPress(hidden, "tab"); got.Focus != model.ColumnList {
		t.Fatalf("detail hidden: focus=%v", got.Focus)
	}
	empty := clickBase()
	empty.SelectedUID = ""
	if got, _ := clickPress(empty, "tab"); got.Focus != model.ColumnList {
		t.Fatalf("nothing selected: focus=%v", got.Focus)
	}
}

func TestDetailTabStillCyclesActivityTabs(t *testing.T) {
	m := clickAt(model.ColumnDetail)
	before := m.ActiveTab()
	m, _ = clickPress(m, "tab")
	if m.Focus != model.ColumnDetail || m.ActiveTab() == before {
		t.Fatalf("detail tab: focus=%v tab=%v (was %v)", m.Focus, m.ActiveTab(), before)
	}
	m, _ = clickPress(m, "shift+tab")
	if m.ActiveTab() != before {
		t.Fatalf("shift+tab: tab=%v, want %v", m.ActiveTab(), before)
	}
}
