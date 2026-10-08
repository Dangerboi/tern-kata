package model_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// arrowModes returns one Model per mode whose navigation visibly moves.
func arrowModes(t *testing.T) map[string]model.Model {
	t.Helper()
	peer := "peer"
	// Unbound: a bound Project has no sidebar, so escape would open the Collections picker.
	list, _ := model.Update(model.Model{Cols: 160}, model.ProjectLoaded{
		Project:  model.Project{Name: "Work"},
		Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "a"}, {UID: "b"}, {UID: "c"}}},
	})
	collections, _ := model.Update(list, model.KeyMsg{Chord: "escape"})
	detail := model.Model{Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", DetailUID: "a", Issues: []model.Issue{{UID: "a"}}, Detail: &g.UISelectedAuthority{
		Issue: &model.Issue{UID: "a"},
		Links: []g.UILink{
			{ID: 1, FromIssueUID: "a", ToIssueUID: peer, ToQualifiedID: "other#bbbb", Type: "related"},
			{ID: 2, FromIssueUID: "a", ToIssueUID: "peer2", ToQualifiedID: "other#cccc", Type: "related"},
		},
		History: []g.Event{{UID: "event-1", RelatedIssueUID: &peer}},
	}}
	detail, _ = model.Update(detail, model.KeyMsg{Chord: "tab"})
	detail, _ = model.Update(detail, model.KeyMsg{Chord: "tab"})
	detail, _ = model.Update(detail, model.KeyMsg{Chord: "tab"})
	modes := map[string]model.Model{"list": list, "collections": collections, "detail": detail}
	for name, want := range map[string]model.Mode{"list": model.ModeList, "collections": model.ModeCollections, "detail": model.ModeDetail} {
		if got := modes[name].CurrentMode(); got != want {
			t.Fatalf("%s fixture in mode %s", name, got)
		}
	}
	return modes
}

func TestArrowKeysMatchLetterKeys(t *testing.T) {
	for name, start := range arrowModes(t) {
		for _, pair := range []struct{ arrow, letter string }{
			{"down", "j"}, {"up", "k"}, {"end", "shift+g"}, {"home", "g"},
		} {
			t.Run(name+"/"+pair.arrow, func(t *testing.T) {
				// Move first so that up/home have somewhere to go.
				for _, setup := range [][]string{nil, {"shift+g"}, {"j"}} {
					m := start
					for _, chord := range setup {
						m, _ = model.Update(m, model.KeyMsg{Chord: chord})
					}
					want, wantCmds := model.Update(m, model.KeyMsg{Chord: pair.letter})
					got, gotCmds := model.Update(m, model.KeyMsg{Chord: pair.arrow})
					if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) {
						t.Fatalf("%s after %v differs from %s: %+v vs %+v", pair.arrow, setup, pair.letter, got, want)
					}
					if reflect.DeepEqual(setup, []string{"shift+g"}) && pair.arrow == "up" && reflect.DeepEqual(got, m) {
						t.Fatalf("up did not move from the last row")
					}
				}
			})
		}
	}
}

func TestLeftInDetailBacksOutLikeEscape(t *testing.T) {
	detail := arrowModes(t)["detail"]
	want, wantCmds := model.Update(detail, model.KeyMsg{Chord: "escape"})
	got, gotCmds := model.Update(detail, model.KeyMsg{Chord: "left"})
	if got.Focus != model.ColumnList || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) {
		t.Fatalf("left in detail: %+v %#v, escape: %+v %#v", got, gotCmds, want, wantCmds)
	}
}

func TestArrowKeysAreHintedAndHelped(t *testing.T) {
	for _, mode := range []model.Mode{model.ModeList, model.ModeCollections, model.ModeDetail} {
		var footer, help []string
		for binding := range model.BindingsForMode(mode, true) {
			footer = append(footer, binding.Key)
		}
		for _, binding := range model.Keymap {
			if binding.Mode == mode {
				help = append(help, binding.Chord)
			}
		}
		for _, chord := range []string{"up", "down", "home", "end"} {
			if !slices.Contains(help, chord) {
				t.Errorf("%s help lacks %s", mode, chord)
			}
		}
		if joined := strings.Join(footer, " "); !strings.Contains(joined, "↓") || !strings.Contains(joined, "↑") {
			t.Errorf("%s footer lacks arrows: %s", mode, joined)
		}
	}
	if binding, _ := model.LookupBindingForMode("left", model.ModeDetail); binding.Action != model.DrillOut {
		t.Errorf("detail left: %+v", binding)
	}
}
