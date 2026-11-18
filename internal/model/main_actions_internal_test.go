package model

import (
	"errors"
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

// Every footer ends the same way: the Page's own keys, then esc (Back or Menu), then ? and q where the mode has them.
func TestFooterEndsWithEscHelpAndQuitInEveryMode(t *testing.T) {
	modes := map[Mode]bool{}
	for _, binding := range Keymap {
		modes[binding.Mode] = true
	}
	for mode := range modes {
		last := 0
		for binding := range BindingsForMode(mode, true) {
			rank := footerRank(binding)
			if rank < last {
				t.Errorf("mode %q: %q (%s) follows a key that belongs after it", mode, binding.Chord, binding.Help)
			}
			last = rank
		}
	}
}

// The daemon error screen can open the Daemons Page: D works while a daemon error is set and the Page replaces the error screen.
func TestDaemonsKeyOpensTheDaemonsPageFromTheErrorScreen(t *testing.T) {
	m, _ := Update(Model{}, Failed{Err: errors.New("daemon unreachable")})
	if m.CurrentMode() != ModeError {
		t.Fatalf("mode %s, want the error screen", m.CurrentMode())
	}
	for binding := range BindingsForMode(ModeError, true) {
		if binding.Action == ShowDaemonsPage {
			t.Errorf("the footer lists D, which the error screen's button presses: %+v", binding)
		}
	}
	m, cmds := Update(m, KeyMsg{Chord: "shift+d", Text: "D"})
	if m.Page != PageDaemons || m.CurrentMode() == ModeError || len(cmds) == 0 {
		t.Fatalf("D left page %v mode %s with %d commands, want the Daemons Page", m.Page, m.CurrentMode(), len(cmds))
	}
}
