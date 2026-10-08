package model_test

import (
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
		{"Collections picker", true, []string{"p"}},
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
