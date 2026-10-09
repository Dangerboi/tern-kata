package model_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func pagedList(n int) model.Model {
	issues := make([]model.Issue, n)
	for i := range issues {
		issues[i] = model.Issue{UID: fmt.Sprintf("i%02d", i), Status: "open"}
	}
	m, _ := model.Update(model.Model{Cols: 160}, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, Snapshot: g.UISnapshotResponseBody{Collection: issues}})
	return m
}

func TestPageKeysMoveTheListSelectionByAPageAndClamp(t *testing.T) {
	m := pagedList(25)
	for _, step := range []struct{ chord, want string }{
		{"page_down", "i10"}, {"page_down", "i20"}, {"page_down", "i24"}, {"page_down", "i24"},
		{"page_up", "i14"}, {"page_up", "i04"}, {"page_up", "i00"}, {"page_up", "i00"},
	} {
		var cmds []model.Cmd
		m, cmds = model.Update(m, model.KeyMsg{Chord: step.chord})
		if m.SelectedUID != step.want || len(cmds) > 1 {
			t.Fatalf("%s: selected %q, want %q (commands %#v)", step.chord, m.SelectedUID, step.want, cmds)
		}
		for _, cmd := range cmds {
			if _, ok := cmd.(model.Scroll); ok {
				t.Fatalf("%s asked Tern to scroll: %#v", step.chord, cmds)
			}
		}
	}
}

func TestPageKeysStillScrollTheCollectionsSidebar(t *testing.T) {
	m := model.Model{Cols: 160, Focus: model.ColumnCollections}
	_, cmds := model.Update(m, model.KeyMsg{Chord: "page_down"})
	if want := []model.Cmd{model.Scroll{Column: model.ColumnCollections, Down: true}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands %#v", cmds)
	}
}

func TestPageDownInSearchEntersResultsThenMovesTheSelection(t *testing.T) {
	m := pagedList(25)
	m.Search = model.SearchState{Open: true}
	m, _ = model.Update(m, model.KeyMsg{Chord: "page_down"})
	if !m.Search.Results || m.SelectedUID != "i10" {
		t.Fatalf("search page navigation: %+v selected %q", m.Search, m.SelectedUID)
	}
}
