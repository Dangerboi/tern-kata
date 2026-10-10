package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func TestVisibleColumnsByWidth(t *testing.T) {
	list, detail, sidebar := model.ColumnList, model.ColumnDetail, model.ColumnCollections
	for _, tt := range []struct {
		name  string
		cols  int
		bound bool
		focus model.Column
		want  []model.Column
	}{
		{"bound 79 shows nothing", 79, true, list, nil},
		{"unbound 79 shows nothing", 79, false, list, nil},
		{"bound 80 shows the list", 80, true, list, []model.Column{list}},
		{"unbound 80 shows the list", 80, false, list, []model.Column{list}},
		{"bound 139 shows the list", 139, true, list, []model.Column{list}},
		{"unbound 139 shows the list", 139, false, list, []model.Column{list}},
		{"bound 80 detail focused shows detail", 80, true, detail, []model.Column{detail}},
		{"bound 139 detail focused shows detail", 139, true, detail, []model.Column{detail}},
		{"unbound 139 sidebar focused shows the sidebar", 139, false, sidebar, []model.Column{sidebar}},
		{"bound 140 shows list and detail", 140, true, list, []model.Column{list, detail}},
		{"unbound 140 shows sidebar, list and detail", 140, false, list, []model.Column{sidebar, list, detail}},
		{"bound 140 detail focused shows both", 140, true, detail, []model.Column{list, detail}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := pickerStart(tt.cols, tt.bound)
			m.Focus = tt.focus
			if got := m.VisibleColumns(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("visible columns = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResizeBelowMinimumKeepsEverythingElse(t *testing.T) {
	for _, tt := range []struct {
		name  string
		bound bool
		keys  []string
	}{
		{"list selection", true, []string{"j"}},
		{"open detail", true, []string{"j", "enter"}},
		{"help layer", true, []string{"j", "?"}},
		{"Menu", true, []string{"p"}},
		{"unbound sidebar focus", false, []string{"escape"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wide, _ := pickerKeys(pickerStart(160, tt.bound), tt.keys...)
			narrow, cmds := model.Update(wide, model.ResizeMsg{Cols: 40})
			if len(cmds) != 0 || len(narrow.VisibleColumns()) != 0 {
				t.Fatalf("narrow: columns=%v commands=%#v", narrow.VisibleColumns(), cmds)
			}
			back, _ := model.Update(narrow, model.ResizeMsg{Cols: 160})
			if !reflect.DeepEqual(back, wide) {
				t.Fatalf("resizing down and back changed the state:\nbefore %+v\nafter  %+v", wide, back)
			}
		})
	}
}

func TestQuitStillWorksBelowMinimumWidth(t *testing.T) {
	for _, chord := range []string{"q", "ctrl+c"} {
		t.Run(chord, func(t *testing.T) {
			m, _ := model.Update(pickerStart(60, true), model.KeyMsg{Chord: chord, Text: chord})
			if !m.Quitting {
				t.Fatalf("%s did not quit below 80 columns", chord)
			}
		})
	}
}

// The app sends every Scroll to Tern, which ends the Pane's program when the
// column is not drawn (a Pane under 80 columns, or one column of two, the first
// load, a daemon failure). The model asks only for columns on screen.
func TestScrollIsOnlyRequestedOfColumnsOnScreen(t *testing.T) {
	list, detail := model.ColumnList, model.ColumnDetail
	for _, tt := range []struct {
		name  string
		cols  int
		focus model.Column
		setup []model.Msg
		msg   model.Msg
		want  []model.Cmd
	}{
		{"page below 80 columns from the list", 79, list, nil, model.KeyMsg{Chord: "page_down"}, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}},
		{"page below 80 columns from the detail", 79, detail, nil, model.KeyMsg{Chord: "page_up"}, nil},
		{"page in the one column shown scrolls it", 100, detail, nil, model.KeyMsg{Chord: "page_down"}, []model.Cmd{model.Scroll{Column: detail, By: model.ScrollPageDown}}},
		{"another detail arrives behind the single list", 100, list, []model.Msg{model.DetailLoaded{UID: "a", Selected: authority("a")}, model.SelectMsg{UID: "b"}}, model.DetailLoaded{UID: "b", Selected: authority("b")}, nil},
		{"another detail arrives beside the list", 160, list, []model.Msg{model.DetailLoaded{UID: "a", Selected: authority("a")}, model.SelectMsg{UID: "b"}}, model.DetailLoaded{UID: "b", Selected: authority("b")}, []model.Cmd{model.Scroll{Column: detail, By: model.ScrollToStart}}},
		{"page during the daemon failure", 160, list, []model.Msg{model.Failed{Err: errors.New("kata server not responding")}}, model.KeyMsg{Chord: "page_down"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := projectOf("a", "b")
			m.Cols, m.Focus = tt.cols, tt.focus
			for _, msg := range tt.setup {
				m, _ = model.Update(m, msg)
			}
			if _, cmds := model.Update(m, tt.msg); len(cmds) != len(tt.want) || (len(tt.want) > 0 && !reflect.DeepEqual(cmds, tt.want)) {
				t.Fatalf("commands = %#v, want %#v", cmds, tt.want)
			}
		})
	}
	t.Run("page during the first load", func(t *testing.T) {
		m, _ := model.Update(model.Model{Cols: 160}, model.Start{Workspace: "/repo"})
		if _, cmds := model.Update(m, model.KeyMsg{Chord: "page_down"}); len(cmds) != 0 {
			t.Fatalf("commands = %#v", cmds)
		}
	})
}

// Shows is what keeps Reveal and Scroll away from a node the view does not draw.
func TestShowsOnlyTheDrawnColumns(t *testing.T) {
	list, detail := model.ColumnList, model.ColumnDetail
	for _, tt := range []struct {
		name   string
		cols   int
		focus  model.Column
		err    error
		column model.Column
		want   bool
	}{
		{"below 80 draws no list", 79, list, nil, list, false},
		{"below 80 draws no detail", 79, detail, nil, detail, false},
		{"one column draws the focused list", 100, list, nil, list, true},
		{"one column draws no detail behind the list", 100, list, nil, detail, false},
		{"one column draws the focused detail", 100, detail, nil, detail, true},
		{"one column draws no list behind the detail", 100, detail, nil, list, false},
		{"side by side draws both", 160, list, nil, detail, true},
		{"a daemon failure fills the Pane", 160, list, errors.New("down"), list, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := pickerStart(tt.cols, true)
			m.Focus, m.Err = tt.focus, tt.err
			if got := m.Shows(tt.column); got != tt.want {
				t.Fatalf("Shows(%v) = %v, want %v", tt.column, got, tt.want)
			}
		})
	}
}
