package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func TestPageNavigationScrollsFocusedColumn(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnCollections, model.ColumnDetail} {
		for _, chord := range []string{"page_up", "page_down"} {
			m := model.Model{Focus: focus, SelectedUID: "issue", DetailUID: "issue"}
			next, cmds := model.Update(m, model.KeyMsg{Chord: chord})
			want := []model.Cmd{model.Scroll{Column: focus, Down: chord == "page_down"}}
			if !reflect.DeepEqual(cmds, want) || next.SelectedUID != m.SelectedUID {
				t.Fatalf("focus=%v chord=%s: commands=%#v selected=%q", focus, chord, cmds, next.SelectedUID)
			}
		}
	}
	m := model.Model{Search: model.SearchState{Open: true}, SelectedUID: "issue", DetailUID: "issue"}
	m, cmds := model.Update(m, model.KeyMsg{Chord: "page_down"})
	if !m.Search.Results || !reflect.DeepEqual(cmds, []model.Cmd{model.Scroll{Column: model.ColumnList, Down: true}}) {
		t.Fatalf("search page navigation: %+v %#v", m.Search, cmds)
	}
}
