package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Tern keeps the node of its last reveal in view and ignores the wheel past it,
// until that node is removed. A reveal is therefore sent only for a move of the
// detail's cursor the keys made, never for what a click already put under the
// pointer (ef39).
func TestDetailIsRevealedOnlyForKeyMovesOfItsCursor(t *testing.T) {
	links := []g.UILink{
		{ID: 1, Type: "child", FromIssueUID: "i00", ToIssueUID: "c0"},
		{ID: 2, Type: "child", FromIssueUID: "i00", ToIssueUID: "c1"},
	}
	loaded := pagedList(1)
	loaded, _ = model.Update(loaded, model.DetailLoaded{UID: loaded.SelectedUID, Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: loaded.SelectedUID}, Links: links}})
	focused, _ := model.Update(loaded, model.KeyMsg{Chord: "enter"})
	inLinks := focused
	for inLinks.DetailSection != model.SectionLinks {
		inLinks, _ = model.Update(inLinks, model.KeyMsg{Chord: "tab"})
	}
	for _, tt := range []struct {
		name  string
		start model.Model
		msg   model.Msg
		want  bool
	}{
		{"enter into the detail", loaded, model.KeyMsg{Chord: "enter"}, false},
		{"a click on the detail title", loaded, model.FocusColumnMsg{Column: model.ColumnDetail}, false},
		{"a click on a section heading", loaded, model.DetailSectionMsg{Section: model.SectionBody}, false},
		{"a click on a row", inLinks, model.DetailItemMsg{Section: model.SectionLinks, Key: "2"}, false},
		{"tab to the next section", focused, model.KeyMsg{Chord: "tab"}, true},
		{"the same key from a footer hint", focused, model.ActionMsg{Action: "key", Value: "tab"}, true},
		{"j to the next row", inLinks, model.KeyMsg{Chord: "j"}, true},
		{"a key that moves nothing", inLinks, model.KeyMsg{Chord: "x"}, false},
	} {
		next, _ := model.Update(tt.start, tt.msg)
		if got := model.RevealsDetail(tt.start, next, tt.msg); got != tt.want {
			t.Errorf("%s: reveal = %v, want %v", tt.name, got, tt.want)
		}
	}
}
