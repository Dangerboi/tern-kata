package model_test

import (
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

func TestHintsFollowCurrentModeAndKeymap(t *testing.T) {
	original := model.Keymap
	t.Cleanup(func() { model.Keymap = original })
	model.Keymap = []model.Binding{
		{Chord: "j", Key: "j", Action: model.MoveDown, Help: "next Issue", Mode: model.ModeList, ShowInFooter: true},
		{Chord: "g", Key: "g", Action: model.MoveFirst, Help: "first Issue", Mode: model.ModeList},
		{Chord: "?", Key: "?", Action: model.ToggleHelp, Help: "help", ShowInFooter: true},
		{Chord: "escape", Key: "esc", Action: model.CloseHelp, Help: "close help", Mode: model.ModeHelp, ShowInFooter: true},
	}
	for _, tt := range []struct {
		keys []string
		want string
	}{
		{nil, "j next Issue;? help;"},
		{[]string{"?"}, "? help;esc close help;"},
		{[]string{"?", "escape"}, "j next Issue;? help;"},
	} {
		m := model.Model{}
		for _, key := range tt.keys {
			var cmds []model.Cmd
			m, cmds = model.Update(m, model.KeyMsg{Chord: key})
			if len(cmds) != 0 {
				t.Fatalf("hint mode requested commands: %#v", cmds)
			}
		}
		got := ""
		for binding := range model.BindingsForMode(m.CurrentMode(), true) {
			got += binding.Key + " " + binding.Help + ";"
		}
		if got != tt.want {
			t.Fatalf("visible hints = %q, want %q", got, tt.want)
		}
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
