package model_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// clickBase is an unbound workspace in the Issue list. Rows sort by title:
// a parent (collapsed, with one child), two leaves, and a detail whose Links
// name "other".
func clickBase() model.Model {
	m, _ := model.Update(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Snapshot: g.UISnapshotResponseBody{
		Collection: []model.Issue{
			{UID: "parent", ShortID: "par1", Title: "A parent", Status: "open"},
			{UID: "child", ShortID: "chi1", Title: "B child", Status: "open"},
			{UID: "leaf", ShortID: "lea1", Title: "C leaf", Status: "open"},
			{UID: "other", ShortID: "oth1", Title: "D other", Status: "open"},
		},
		CollectionLinks: []g.UILink{{Type: "parent", FromIssueUID: "child", ToIssueUID: "parent"}},
	}})
	m, _ = model.Update(m, model.DetailLoaded{UID: "parent", Selected: &g.UISelectedAuthority{
		Issue: &model.Issue{UID: "parent"},
		Links: []g.UILink{{ID: 1, Type: "related", FromIssueUID: "parent", ToIssueUID: "other", ToQualifiedID: "work#oth1"}},
	}})
	return m
}

// clickDiff names the Model fields two states disagree on.
func clickDiff(a, b model.Model) string {
	var out string
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := range av.NumField() {
		if !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			out += fmt.Sprintf("\n  %s: %+v != %+v", av.Type().Field(i).Name, av.Field(i).Interface(), bv.Field(i).Interface())
		}
	}
	return out
}

func clickPress(m model.Model, chords ...string) (model.Model, []model.Cmd) {
	var all []model.Cmd
	for _, chord := range chords {
		var cmds []model.Cmd
		m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
		all = append(all, cmds...)
	}
	return m, all
}

// clickAt applies listKeys in the Issue list, then moves the keyboard to focus.
func clickAt(focus model.Column, listKeys ...string) model.Model {
	m, _ := clickPress(clickBase(), listKeys...)
	switch focus {
	case model.ColumnCollections:
		m, _ = clickPress(m, "escape")
	case model.ColumnDetail:
		m, _ = clickPress(m, "enter")
	}
	return m
}

// clickTabTo cycles detail sections with tab, so tests follow the on-screen order.
func clickTabTo(m model.Model, section model.DetailSection) model.Model {
	for range 6 {
		if m.DetailSection == section {
			break
		}
		m, _ = clickPress(m, "tab")
	}
	return m
}

// clickDetailPath is the keyboard route to a detail section: enter the detail when
// the keys are elsewhere, tab to the section, then press after.
func clickDetailPath(section model.DetailSection, after ...string) func(model.Model) (model.Model, []model.Cmd) {
	return func(m model.Model) (model.Model, []model.Cmd) {
		var cmds []model.Cmd
		if m.Focus != model.ColumnDetail {
			m, cmds = clickPress(m, "enter")
		}
		m = clickTabTo(m, section)
		m, more := clickPress(m, after...)
		return m, append(cmds, more...)
	}
}

func clickKeys(chords ...string) func(model.Model) (model.Model, []model.Cmd) {
	return func(m model.Model) (model.Model, []model.Cmd) { return clickPress(m, chords...) }
}

func TestEveryClickTargetEndsWhereItsKeysEnd(t *testing.T) {
	today := "main.columns.collections.today"
	for _, tt := range []struct {
		name     string
		from     model.Column
		listKeys []string
		click    model.Msg
		// The keyboard user starts here, and then presses keyboard.
		keyFrom  model.Column
		keyboard func(model.Model) (model.Model, []model.Cmd)
	}{
		{"leaf row from list", model.ColumnList, nil, model.SelectMsg{UID: "leaf"}, model.ColumnList, clickKeys("j")},
		{"leaf row from detail", model.ColumnDetail, nil, model.SelectMsg{UID: "leaf"}, model.ColumnDetail, clickKeys("escape", "j")},
		{"leaf row from Collections", model.ColumnCollections, nil, model.SelectMsg{UID: "leaf"}, model.ColumnList, clickKeys("j")},
		{"nested child row", model.ColumnList, []string{"space"}, model.SelectMsg{UID: "child"}, model.ColumnList, clickKeys("j")},
		{"parent row from list", model.ColumnList, []string{"shift+g"}, model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}, model.ColumnList, clickKeys("g", "space")},
		{"parent row from detail", model.ColumnDetail, nil, model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}, model.ColumnDetail, clickKeys("escape", "space")},
		{"parent row from Collections", model.ColumnCollections, nil, model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}, model.ColumnList, clickKeys("space")},
		{"parent row collapses an expanded parent", model.ColumnCollections, []string{"space"}, model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}, model.ColumnList, clickKeys("space")},
		{"list heading from detail", model.ColumnDetail, nil, model.FocusColumnMsg{Column: model.ColumnList}, model.ColumnDetail, clickKeys("escape")},
		{"list heading from Collections", model.ColumnCollections, nil, model.FocusColumnMsg{Column: model.ColumnList}, model.ColumnList, clickKeys()},
		{"detail title from list", model.ColumnList, nil, model.FocusColumnMsg{Column: model.ColumnDetail}, model.ColumnList, clickKeys("enter")},
		{"section heading", model.ColumnList, nil, model.DetailSectionMsg{Section: model.SectionLinks}, model.ColumnList, clickDetailPath(model.SectionLinks)},
		{"detail row", model.ColumnDetail, nil, model.DetailItemMsg{Section: model.SectionLinks, Key: "1"}, model.ColumnDetail, clickDetailPath(model.SectionLinks)},
		{"referenced Issue from detail", model.ColumnDetail, nil, model.JumpIssueMsg{UID: "other"}, model.ColumnDetail, clickDetailPath(model.SectionLinks, "enter")},
		{"referenced Issue from list", model.ColumnList, nil, model.JumpIssueMsg{UID: "other"}, model.ColumnList, clickDetailPath(model.SectionLinks, "enter")},
		{"Collection from Collections", model.ColumnCollections, nil, model.CollectionPointerSelected{ID: today}, model.ColumnCollections, clickKeys("j", "enter")},
		{"Collection from list", model.ColumnList, nil, model.CollectionPointerSelected{ID: today}, model.ColumnList, clickKeys("escape", "j", "enter")},
		{"Collection from detail", model.ColumnDetail, nil, model.CollectionPointerSelected{ID: today}, model.ColumnDetail, clickKeys("escape", "escape", "j", "enter")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pointer, pointerCmds := model.Update(clickAt(tt.from, tt.listKeys...), tt.click)
			keyboard, keyboardCmds := tt.keyboard(clickAt(tt.keyFrom, tt.listKeys...))
			if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(pointerCmds, keyboardCmds) {
				t.Fatalf("click and keys diverge (click != keys):%s\ncommands: %#v != %#v", clickDiff(pointer, keyboard), pointerCmds, keyboardCmds)
			}
			afterClick, clickCmds := clickPress(pointer, "escape")
			afterKeys, keyCmds := clickPress(keyboard, "escape")
			if !reflect.DeepEqual(afterClick, afterKeys) || !reflect.DeepEqual(clickCmds, keyCmds) {
				t.Fatalf("esc differs after click (click != keys):%s\ncommands: %#v != %#v", clickDiff(afterClick, afterKeys), clickCmds, keyCmds)
			}
		})
	}
}

func TestEveryFooterHintClicksLikeItsKey(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnCollections, model.ColumnDetail} {
		m := clickAt(focus)
		for binding := range model.BindingsForMode(m.CurrentMode(), true) {
			t.Run(string(m.CurrentMode())+"/"+binding.Chord, func(t *testing.T) {
				keyboard, keyCmds := model.Update(m, model.KeyMsg{Chord: binding.Chord})
				pointer, clickCmds := model.Update(m, model.ActionMsg{Action: "key", Value: binding.Chord})
				if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(clickCmds, keyCmds) {
					t.Fatalf("hint differs from key (click != key):%s\ncommands: %#v != %#v", clickDiff(pointer, keyboard), clickCmds, keyCmds)
				}
			})
		}
	}
}

func TestLayerClicksEndWhereTheirKeysEnd(t *testing.T) {
	for _, tt := range []struct {
		name  string
		open  string
		click model.Msg
		keys  []string
	}{
		{"filters Apply button", "f", model.FilterFormActionMsg{Action: model.ApplyFilters}, []string{"ctrl+o"}},
		{"filters Cancel button", "f", model.FilterFormActionMsg{Action: model.CancelFilters}, []string{"escape"}},
		{"filters field", "f", model.FilterFieldMsg{Field: 2}, []string{"tab", "tab"}},
		{"create field", "n", model.CreateFieldMsg{Field: 3}, []string{"tab", "tab", "tab"}},
		{"close Close button", "x", model.CloseSubmitMsg{}, []string{"ctrl+o"}},
		{"close Quick done button", "x", model.CloseSubmitMsg{Quick: true}, []string{"ctrl+d"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			open, _ := clickPress(clickAt(model.ColumnList), tt.open)
			pointer, clickCmds := model.Update(open, tt.click)
			keyboard, keyCmds := clickPress(open, tt.keys...)
			if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(clickCmds, keyCmds) {
				t.Fatalf("click and keys diverge (click != keys):%s\ncommands: %#v != %#v", clickDiff(pointer, keyboard), clickCmds, keyCmds)
			}
		})
	}
}

func TestRetryButtonRetriesLikeItsKey(t *testing.T) {
	m := model.Model{Workspace: "/repo", Err: errors.New("kata server not responding")}
	pointer, clickCmds := model.Update(m, model.Retry{})
	keyboard, keyCmds := clickPress(m, "ctrl+r")
	if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(clickCmds, keyCmds) || len(keyCmds) != 1 {
		t.Fatalf("retry click and key diverge (click != key):%s\ncommands: %#v != %#v", clickDiff(pointer, keyboard), clickCmds, keyCmds)
	}
}

func TestEscAfterClickBacksOut(t *testing.T) {
	for _, tt := range []struct {
		name  string
		from  model.Column
		msgs  []model.Msg
		focus model.Column
		uid   string
		back  []string
	}{
		{"clicked row", model.ColumnList, []model.Msg{model.SelectMsg{UID: "leaf"}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, "leaf", nil},
		{"clicked parent beside the sidebar", model.ColumnCollections, []model.Msg{model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}}, model.ColumnList, "parent", nil},
		{"clicked parent then esc", model.ColumnCollections, []model.Msg{model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, "parent", nil},
		{"clicked nested child then esc", model.ColumnDetail, []model.Msg{model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}, model.SelectMsg{UID: "child"}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, "child", nil},
		{"clicked referenced Issue", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}}, model.ColumnDetail, "other", []string{"parent"}},
		{"esc returns to the Issue it came from", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.KeyMsg{Chord: "escape"}}, model.ColumnDetail, "parent", nil},
		{"second esc leaves detail", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.KeyMsg{Chord: "escape"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "parent", nil},
		{"clicked detail heading", model.ColumnList, []model.Msg{model.DetailSectionMsg{Section: model.SectionLinks}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "parent", nil},
		{"clicked detail row", model.ColumnList, []model.Msg{model.DetailItemMsg{Section: model.SectionLinks, Key: "1"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "parent", nil},
		{"clicked Collection", model.ColumnList, []model.Msg{model.CollectionPointerSelected{ID: "main.columns.collections.today"}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, "parent", nil},
		{"Collection after a jump ends the jump", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.CollectionPointerSelected{ID: "main.columns.collections.today"}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "other", nil},
		{"list heading after a jump ends the jump", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.FocusColumnMsg{Column: model.ColumnList}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "other", nil},
		{"row after a jump ends the jump", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.SelectMsg{UID: "leaf"}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "leaf", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := clickAt(tt.from)
			for _, msg := range tt.msgs {
				m, _ = model.Update(m, msg)
			}
			if m.Focus != tt.focus || m.SelectedUID != tt.uid || !slices.Equal(m.DetailBack, tt.back) {
				t.Fatalf("focus=%v selected=%q back=%v, want focus=%v selected=%q back=%v", m.Focus, m.SelectedUID, m.DetailBack, tt.focus, tt.uid, tt.back)
			}
		})
	}
}

// Jump history belongs to the detail column: hiding detail with the keys ends
// it as clicking away does, so a later esc never replays an earlier jump.
func TestJumpHistoryEndsWhenKeysLeaveDetail(t *testing.T) {
	m, _ := clickDetailPath(model.SectionLinks, "enter")(clickAt(model.ColumnDetail))
	if m.SelectedUID != "other" || len(m.DetailBack) != 1 {
		t.Fatalf("jump did not record history: selected=%q back=%v", m.SelectedUID, m.DetailBack)
	}
	m, _ = clickPress(m, "shift+l", "shift+l", "enter", "escape")
	if m.Focus != model.ColumnList || m.SelectedUID != "other" || len(m.DetailBack) != 0 {
		t.Fatalf("esc replayed a jump: focus=%v selected=%q back=%v", m.Focus, m.SelectedUID, m.DetailBack)
	}
}

func TestEscClosesLayerAfterClickInsideIt(t *testing.T) {
	for _, tt := range []struct {
		name  string
		open  string
		click model.Msg
	}{
		{"filter form field", "f", model.FilterFieldMsg{Field: 2}},
		{"create form field", "n", model.CreateFieldMsg{Field: 3}},
		{"close form field", "x", model.CloseFieldMsg{Field: 2}},
		{"label prompt", "+", model.FieldValueMsg{Value: "x"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := clickPress(clickAt(model.ColumnList), tt.open)
			if m.CurrentMode() == model.ModeList {
				t.Fatalf("%q did not open a layer", tt.open)
			}
			m, _ = model.Update(m, tt.click)
			m, _ = clickPress(m, "escape")
			if m.CurrentMode() != model.ModeList {
				t.Fatalf("esc after click left mode %s", m.CurrentMode())
			}
		})
	}
}
