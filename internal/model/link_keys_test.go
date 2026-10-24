package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestLinkKeysAreInTheDetailFooterAndHelpSheet(t *testing.T) {
	for _, chord := range []string{"b", "l"} {
		b, ok := model.LookupBindingForMode(chord, model.ModeDetail)
		if !ok || !b.ShowInFooter {
			t.Fatalf("%s not in the detail footer: %+v", chord, b)
		}
	}
	var lines []string
	for _, g := range model.HelpSheet() {
		for _, l := range g.Lines {
			lines = append(lines, l.Text+"|"+strings.Join(l.Keys, ","))
		}
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{"add a Blocking link: this Issue blocks the one you name|b", "add a related link|l"} {
		if !strings.Contains(all, want) {
			t.Fatalf("help sheet lacks %q:\n%s", want, all)
		}
	}
}

func TestBStillTogglesTheBoardInTheListAndLane(t *testing.T) {
	for _, mode := range []model.Mode{model.ModeList, model.ModeLane} {
		b, ok := model.LookupBindingForMode("b", mode)
		if !ok || b.Action != model.ToggleBoard {
			t.Fatalf("b in %s is %+v", mode, b)
		}
	}
}
