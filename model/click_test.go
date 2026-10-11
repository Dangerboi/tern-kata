package model_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// clickBase is an unbound workspace in the Issue list. Rows sort by update time, newest first:
// a parent (collapsed, with one child), two leaves, and a detail whose Links
// name "other".
func clickBase() model.Model {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{
		Collection: []model.Issue{
			{UID: "parent", ShortID: "par1", Title: "A parent", Status: "open", UpdatedAt: ago(1)},
			{UID: "child", ShortID: "chi1", Title: "B child", Status: "open", UpdatedAt: ago(2)},
			{UID: "leaf", ShortID: "lea1", Title: "C leaf", Status: "open", UpdatedAt: ago(3)},
			{UID: "other", ShortID: "oth1", Title: "D other", Status: "open", UpdatedAt: ago(4)},
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

// clickTabTo cycles Activity tabs with tab, so tests follow the on-screen order.
func clickTabTo(m model.Model, tab model.ActivityTab) model.Model {
	for range model.TabCount {
		if m.ActiveTab() == tab {
			break
		}
		m, _ = clickPress(m, "tab")
	}
	return m
}

// clickDetailPath is the keyboard route to an Activity tab: enter the detail when
// the keys are elsewhere, tab to the tab, then press after.
func clickDetailPath(tab model.ActivityTab, after ...string) func(model.Model) model.Model {
	return func(m model.Model) model.Model {
		if m.Focus != model.ColumnDetail {
			m, _ = clickPress(m, "enter")
		}
		m = clickTabTo(m, tab)
		m, _ = clickPress(m, after...)
		return m
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
		{"clicked Activity tab", model.ColumnList, []model.Msg{model.ActivityTabMsg{Tab: model.TabLinks}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "parent", nil},
		{"clicked detail row", model.ColumnList, []model.Msg{model.DetailItemMsg{Tab: model.TabLinks, Key: "1"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "parent", nil},
		{"clicked Collection", model.ColumnList, []model.Msg{model.CollectionPointerSelected{Key: "today"}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, "parent", nil},
		{"Collection after a jump ends the jump", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.CollectionPointerSelected{Key: "today"}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "other", nil},
		{"row after a jump ends the jump", model.ColumnDetail, []model.Msg{model.JumpIssueMsg{UID: "other"}, model.SelectMsg{UID: "leaf"}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, "leaf", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := clickAt(tt.from)
			for _, msg := range tt.msgs {
				m, _ = model.Update(m, msg)
			}
			if m.Focus != tt.focus || m.SelectedUID != tt.uid || !slices.Equal(m.JumpHistory(), tt.back) {
				t.Fatalf("focus=%v selected=%q back=%v, want focus=%v selected=%q back=%v", m.Focus, m.SelectedUID, m.JumpHistory(), tt.focus, tt.uid, tt.back)
			}
		})
	}
}

// Jump history belongs to the detail column: hiding detail with the keys ends
// it as clicking away does, so a later esc never replays an earlier jump.
func TestJumpHistoryEndsWhenKeysLeaveDetail(t *testing.T) {
	m := clickDetailPath(model.TabLinks, "enter")(clickAt(model.ColumnDetail))
	if m.SelectedUID != "other" || len(m.JumpHistory()) != 1 {
		t.Fatalf("jump did not record history: selected=%q back=%v", m.SelectedUID, m.JumpHistory())
	}
	m, _ = clickPress(m, "shift+l", "shift+l", "enter", "escape")
	if m.Focus != model.ColumnList || m.SelectedUID != "other" || len(m.JumpHistory()) != 0 {
		t.Fatalf("esc replayed a jump: focus=%v selected=%q back=%v", m.Focus, m.SelectedUID, m.JumpHistory())
	}
}

func TestEscClosesLayerAfterClickInsideIt(t *testing.T) {
	for _, tt := range []struct {
		name  string
		open  string
		click model.Msg
	}{
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
