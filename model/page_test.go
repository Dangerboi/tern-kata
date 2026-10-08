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

func TestPageKeysMoveTheDetailCursorByAPage(t *testing.T) {
	links := make([]g.UILink, 25)
	for i := range links {
		links[i] = g.UILink{ID: int64(i + 1), Type: "child", FromIssueUID: "a", ToIssueUID: fmt.Sprintf("c%d", i)}
	}
	m := pagedList(1)
	m, _ = model.Update(m, model.DetailLoaded{UID: m.SelectedUID, Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: m.SelectedUID}, Links: links}})
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	for m.DetailSection != model.SectionLinks {
		m, _ = model.Update(m, model.KeyMsg{Chord: "tab"})
	}
	count := m.DetailRowCount(model.SectionLinks)
	if count < 20 {
		t.Fatalf("fixture: %d Link rows", count)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "page_down"})
	if m.DetailSection != model.SectionLinks || m.DetailIndex != 10 {
		t.Fatalf("page down: section %v index %d, want Links 10", m.DetailSection, m.DetailIndex)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "page_up"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "page_up"})
	if m.DetailSection != model.SectionLinks || m.DetailIndex != 0 {
		t.Fatalf("page up clamps at the section's first row: section %v index %d", m.DetailSection, m.DetailIndex)
	}
	for range 5 {
		m, _ = model.Update(m, model.KeyMsg{Chord: "page_down"})
	}
	if m.DetailSection != model.SectionLinks && m.DetailIndex < 0 {
		t.Fatalf("section %v index %d", m.DetailSection, m.DetailIndex)
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
