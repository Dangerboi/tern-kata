package model_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// dblclickRows is every kind of Issue row a double click can land on in clickBase: a leaf,
// a parent (collapsed or expanded by space) and the child nested under it.
var dblclickRows = []struct {
	name     string
	uid      string
	listKeys []string
}{
	{"leaf", "leaf", nil},
	{"collapsed parent", "parent", nil},
	{"expanded parent", "parent", []string{"space"}},
	{"nested child", "child", []string{"space"}},
}

// dblclickStarts is where the keys can be when the pointer lands on a row, side by side.
var dblclickStarts = []struct {
	name string
	from func(listKeys ...string) model.Model
}{
	{"list", func(k ...string) model.Model { return clickAt(model.ColumnList, k...) }},
	{"Collections sidebar", func(k ...string) model.Model { return clickAt(model.ColumnCollections, k...) }},
	{"detail", func(k ...string) model.Model { return clickAt(model.ColumnDetail, k...) }},
	{"detail after a jump", func(k ...string) model.Model {
		m, _ := model.Update(clickAt(model.ColumnDetail, k...), model.JumpIssueMsg{UID: "other"})
		return m
	}},
}

func dblclick(uid string) model.Msg { return model.ActionMsg{Action: model.OpenRow, Value: uid} }

func TestDoubleClickEndsWhereSelectingTheRowThenEnterEnds(t *testing.T) {
	for _, row := range dblclickRows {
		for _, start := range dblclickStarts {
			t.Run(row.name+" from "+start.name, func(t *testing.T) {
				from := start.from(row.listKeys...)
				pointer, pointerCmds := model.Update(from, dblclick(row.uid))
				keyboard, keyCmds := model.Update(from, model.SelectMsg{UID: row.uid})
				keyboard, more := clickPress(keyboard, "enter")
				keyCmds = append(keyCmds, more...)
				if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(pointerCmds, keyCmds) {
					t.Fatalf("double click and select, enter diverge (click != keys):%s\ncommands: %#v != %#v", clickDiff(pointer, keyboard), pointerCmds, keyCmds)
				}
				if pointer.Focus != model.ColumnDetail || pointer.SelectedUID != row.uid {
					t.Fatalf("focus=%v selected=%q, want the detail of %q", pointer.Focus, pointer.SelectedUID, row.uid)
				}
				afterClick, clickCmds := clickPress(pointer, "escape")
				afterKeys, escCmds := clickPress(keyboard, "escape")
				if !reflect.DeepEqual(afterClick, afterKeys) || !reflect.DeepEqual(clickCmds, escCmds) {
					t.Fatalf("esc differs after double click (click != keys):%s\ncommands: %#v != %#v", clickDiff(afterClick, afterKeys), clickCmds, escCmds)
				}
				if afterClick.Focus != model.ColumnList || afterClick.SelectedUID != row.uid {
					t.Fatalf("after esc focus=%v selected=%q, want the list with %q selected", afterClick.Focus, afterClick.SelectedUID, row.uid)
				}
			})
		}
	}
}

func TestDoubleClickInTheSingleColumnTierShowsTheDetailInPlaceOfTheList(t *testing.T) {
	for _, row := range dblclickRows {
		t.Run(row.name, func(t *testing.T) {
			from := clickAt(model.ColumnList, row.listKeys...)
			from.Cols = 100
			if !slices.Equal(from.VisibleColumns(), []model.Column{model.ColumnList}) {
				t.Fatalf("fixture: columns %v", from.VisibleColumns())
			}
			m, _ := model.Update(from, dblclick(row.uid))
			if !slices.Equal(m.VisibleColumns(), []model.Column{model.ColumnDetail}) || m.SelectedUID != row.uid {
				t.Fatalf("columns %v, selected %q, want the detail of %q alone", m.VisibleColumns(), m.SelectedUID, row.uid)
			}
			m, _ = clickPress(m, "escape")
			if !slices.Equal(m.VisibleColumns(), []model.Column{model.ColumnList}) || m.SelectedUID != row.uid {
				t.Fatalf("esc: columns %v, selected %q, want the list with %q selected", m.VisibleColumns(), m.SelectedUID, row.uid)
			}
		})
	}
}

// Tern sends a double click as two single clicks, then the double click's own action. A parent's
// single click folds it, so the two folds cancel and the double click leaves the fold as it was.
func TestDoubleClickOnAParentLeavesItsFoldAsItWas(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		name := map[bool]string{false: "collapsed", true: "expanded"}[expanded]
		t.Run(name, func(t *testing.T) {
			var listKeys []string
			if expanded {
				listKeys = []string{"space"}
			}
			for _, from := range []model.Column{model.ColumnList, model.ColumnCollections, model.ColumnDetail} {
				m := clickAt(from, listKeys...)
				rows := m.VisibleRows()
				fold := model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}
				for _, msg := range []model.Msg{fold, fold, dblclick("parent")} {
					m, _ = model.Update(m, msg)
				}
				if !reflect.DeepEqual(m.VisibleRows(), rows) {
					t.Errorf("from %v: rows %v, were %v", from, m.VisibleRows(), rows)
				}
				if m.Focus != model.ColumnDetail || m.SelectedUID != "parent" {
					t.Errorf("from %v: focus=%v selected=%q, want the detail of the parent", from, m.Focus, m.SelectedUID)
				}
			}
		})
	}
}

func TestDoubleClickOnARowFoldedAwayChangesNothing(t *testing.T) {
	m := clickAt(model.ColumnList)
	pointer, pointerCmds := model.Update(m, dblclick("child"))
	keyboard, keyCmds := model.Update(m, model.SelectMsg{UID: "child"})
	if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(pointerCmds, keyCmds) || pointer.Focus != model.ColumnList {
		t.Fatalf("double click on a hidden row (click != select):%s", clickDiff(pointer, keyboard))
	}
}
