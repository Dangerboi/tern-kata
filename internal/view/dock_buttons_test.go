package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// The dock does not repeat a card's buttons: a card binding is a footer hint only when no button of the card presses its action,
// so no hint's chord is the chord of a button drawn in the same card.
func TestDockHintsDoNotRepeatTheCardsButtons(t *testing.T) {
	for name, m := range map[string]model.Model{
		"Create":          press(selectedFirst(), "n"),
		"Create child":    press(selectedFirst(), "shift+n"),
		"Discard Draft":   press(selectedFirst(), "n", "t", "escape"),
		"Close":           press(selectedFirst(), "x"),
		"Filter":          press(selectedFirst(), "f"),
		"Field form":      press(selectedFirst(), "+"),
		"Dates":           press(selectedFirst(), "d"),
		"Move to Project": press(selectedFirst(), "shift+m"),
		"Agents":          press(selectedFirst(), "w"),
		"New Project":     newProjectModel(false, "escape", "escape", "end", "enter"),
	} {
		t.Run(name, func(t *testing.T) {
			mode := m.CurrentMode()
			if mode == model.ModeList || mode == model.ModeBoard {
				t.Fatalf("the card did not open: mode %s", mode)
			}
			pressed := map[string]string{}
			for id, node := range pickerNodes(t, m) {
				if node.K != "button" || !strings.HasPrefix(id, "layer.") {
					continue
				}
				actions, _ := node.P["actions"].(map[string]any)
				if click, _ := actions["click"].(string); strings.HasPrefix(click, view.ButtonClick("")) {
					pressed[strings.TrimPrefix(click, view.ButtonClick(""))] = id
				}
			}
			if len(pressed) == 0 {
				t.Fatalf("mode %s draws no buttons", mode)
			}
			for binding := range model.BindingsForMode(mode, true) {
				if id, repeated := pressed[binding.Chord]; repeated {
					t.Errorf("the footer hint %q (%s) repeats the button %s", binding.Chord, binding.Help, id)
				}
			}
		})
	}
}
