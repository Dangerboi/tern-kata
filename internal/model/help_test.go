package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestHelpKeys(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		open bool
	}{
		{"open", []string{"?"}, true},
		{"toggle closed", []string{"?", "?"}, false},
		{"escape closes", []string{"?", "escape"}, false},
		{"navigation blocked", []string{"?", "j"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := withDetail(model.Model{Issues: []model.Issue{{UID: "a"}, {UID: "b"}}, SelectedUID: "a"}, nil)
			for _, key := range tt.keys {
				var cmds []model.Cmd
				m, cmds = model.Update(m, model.KeyMsg{Chord: key})
				if len(cmds) != 0 {
					t.Fatalf("help requested commands: %#v", cmds)
				}
			}
			if m.HelpOpen != tt.open || m.SelectedUID != "a" {
				t.Fatalf("help state: %+v", m)
			}
		})
	}
}

func TestHintsFollowCurrentMode(t *testing.T) {
	footer := func(m model.Model) string {
		got := ""
		for binding := range model.BindingsForMode(m.CurrentMode(), true) {
			got += binding.Key + " " + binding.Help + ";"
		}
		return got
	}
	m := model.Model{}
	listHints := footer(m)
	if !strings.Contains(listHints, "j/↓ next Issue;") || !strings.Contains(listHints, "? help;") || strings.Contains(listHints, "close help") {
		t.Fatalf("list hints = %q", listHints)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "?"})
	if got := footer(m); !strings.Contains(got, "esc close help;") || strings.Contains(got, "next Issue") {
		t.Fatalf("help hints = %q", got)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	if got := footer(m); got != listHints {
		t.Fatalf("hints after closing help = %q, want %q", got, listHints)
	}
}

func TestFormsOnlyAdvertiseHandledBindings(t *testing.T) {
	for _, key := range []string{"+", "x"} {
		t.Run(key, func(t *testing.T) {
			m := withDetail(model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", Status: "open"}}}, nil)
			m, _ = model.Update(m, model.KeyMsg{Chord: key})
			for _, chord := range []string{"q", "?"} {
				m, _ = model.Update(m, model.KeyMsg{Chord: chord, Text: chord})
			}
			if m.HelpOpen || m.Quitting {
				t.Fatal("form input dispatched a global action")
			}
			for binding := range model.BindingsForMode(m.CurrentMode(), true) {
				if binding.Mode == "" {
					t.Fatalf("form advertises inactive global binding: %+v", binding)
				}
			}
		})
	}
}
