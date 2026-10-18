package model

import (
	"slices"
	"testing"
)

// A key bound on the main surface does nothing unless its entry has a handler in that mode: every
// binding outside the Popup cards' modes must come from an entry that is bound there and can run.
func TestEveryMainSurfaceKeyHasAHandlerInItsMode(t *testing.T) {
	var cardModes []Mode
	for _, card := range cards {
		cardModes = append(cardModes, card.modes...)
	}
	for _, binding := range Keymap {
		if slices.Contains(cardModes, binding.Mode) {
			continue
		}
		switch {
		case binding.entry == nil || binding.entry.handle == nil:
			t.Errorf("%q in mode %q (%s) has no handler", binding.Chord, binding.Mode, binding.Action)
		case len(binding.entry.modes) > 0 && !slices.Contains(binding.entry.modes, binding.Mode):
			t.Errorf("%q in mode %q (%s) belongs to an entry that is not bound there", binding.Chord, binding.Mode, binding.Action)
		}
	}
}
