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

// Rank orders the help sheet's lines of a group: two lines with different text on one rank would order by table position.
func TestHelpSheetLinesOfAGroupHaveDistinctRanks(t *testing.T) {
	text := map[[2]int]string{}
	for _, entry := range mainActions {
		for _, line := range entry.sheet {
			at := [2]int{int(line.group), line.rank}
			if was, ok := text[at]; ok && was != line.text {
				t.Errorf("group %d rank %d is both %q and %q", line.group, line.rank, was, line.text)
			}
			text[at] = line.text
		}
	}
}
