package model

import (
	"slices"
	"testing"
)

// A key bound on the main surface does nothing without a handler: every binding outside the Popup cards' modes
// must come from an entry that has one.
func TestEveryMainSurfaceKeyHasAHandlerInItsMode(t *testing.T) {
	var cardModes []Mode
	for _, card := range cards {
		cardModes = append(cardModes, card.modes...)
	}
	for _, binding := range Keymap {
		if !slices.Contains(cardModes, binding.Mode) && (binding.entry == nil || binding.entry.handle == nil) {
			t.Errorf("%q in mode %q (%s) has no handler", binding.Chord, binding.Mode, binding.Action)
		}
	}
}
