package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestShowPageSwitchesThePaneToTheNamedPage(t *testing.T) {
	s := projectPane(t, 160, boardCard("p"))
	if s.Model.Page != model.PageIssues {
		t.Fatalf("the Pane starts on Page %v, want the Issues Page", s.Model.Page)
	}
	out := step(t, s, view.DecodeAction("", string(model.ShowPageAction), "attention"))
	if out.State.Model.Page != model.PageAttention {
		t.Fatalf("Page = %v after show-page attention, want the Attention Page", out.State.Model.Page)
	}
}

func TestShowPageIgnoresAnIdNoPageHas(t *testing.T) {
	s := projectPane(t, 160, boardCard("p"))
	for _, id := range []string{"nowhere", ""} {
		out := step(t, s, view.DecodeAction("", string(model.ShowPageAction), id))
		if out.State.Model.Page != model.PageIssues || len(out.Cmds) != 0 {
			t.Errorf("show-page %q: Page %v, cmds %v; want nothing changed", id, out.State.Model.Page, out.Cmds)
		}
	}
}
