package model_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ctrl+n/ctrl+p and ctrl+v/alt+v do what the arrows and the page keys do on every main surface.
func TestEmacsKeysMatchArrowAndPageKeys(t *testing.T) {
	modes := arrowModes(t)
	modes["search"] = pagedList(25)
	modes["search"], _ = model.Update(modes["search"], model.KeyMsg{Chord: "/"})
	modes["list"] = pagedList(25)
	for name, start := range modes {
		for _, pair := range []struct{ emacs, twin string }{
			{"ctrl+n", "down"}, {"ctrl+p", "up"}, {"ctrl+v", "page_down"}, {"alt+v", "page_up"},
		} {
			t.Run(name+"/"+pair.emacs, func(t *testing.T) {
				for _, setup := range [][]string{nil, {"page_down"}, {"page_down", "page_down"}} {
					m := start
					for _, chord := range setup {
						m, _ = model.Update(m, model.KeyMsg{Chord: chord})
					}
					want, wantCmds := model.Update(m, model.KeyMsg{Chord: pair.twin})
					got, gotCmds := model.Update(m, model.KeyMsg{Chord: pair.emacs})
					if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) {
						t.Fatalf("%s after %v differs from %s: %+v vs %+v", pair.emacs, setup, pair.twin, got, want)
					}
				}
			})
		}
	}
}

func TestInboxKeyOpensTheInboxFromListDetailAndSidebar(t *testing.T) {
	modes := arrowModes(t)
	for name, start := range modes {
		start.Collections = modes["list"].Collections
		m, cmds := model.Update(start, model.KeyMsg{Chord: "shift+i", Text: "I"})
		if m.Collection.View != "inbox" || !m.Loading || len(cmds) == 0 {
			t.Errorf("%s: I left the Collection at %+v (loading %v, commands %#v)", name, m.Collection, m.Loading, cmds)
		}
	}
}

func TestInboxKeyIsTextInSearch(t *testing.T) {
	m := pagedList(3)
	m, _ = model.Update(m, model.KeyMsg{Chord: "/"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+i", Text: "I"})
	if m.Filters.Search != "I" || m.Collection.View == "inbox" {
		t.Fatalf("search %q, Collection %+v", m.Filters.Search, m.Collection)
	}
}

func TestEmacsKeysAreHelpedAndNotInTheFooter(t *testing.T) {
	for _, mode := range []model.Mode{model.ModeList, model.ModeCollections, model.ModeDetail} {
		var footer []string
		for binding := range model.BindingsForMode(mode, true) {
			footer = append(footer, binding.Key)
			if binding.Action == model.OpenInbox {
				t.Errorf("%s footer lists the Inbox key", mode)
			}
		}
		if joined := strings.Join(footer, " "); strings.Contains(joined, "C-") || strings.Contains(joined, "M-") {
			t.Errorf("%s footer lists Emacs keys: %s", mode, joined)
		}
	}
}
