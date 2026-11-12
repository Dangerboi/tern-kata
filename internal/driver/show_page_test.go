package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
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

func TestShowPageKeepsAnOpenCardAndItsTypedText(t *testing.T) {
	s := typed(t, step(t, browsing(t, 160), key("n")).State, "phone")
	if s.Model.TopCard() != model.CardCreate || s.Model.Create.Title != "phone" {
		t.Fatalf("fixture: card %v, title %q", s.Model.TopCard(), s.Model.Create.Title)
	}
	out := step(t, s, view.DecodeAction("", string(model.ShowPageAction), "attention"))
	m := out.State.Model
	if m.Page != s.Model.Page || m.TopCard() != model.CardCreate || m.Create.Title != "phone" {
		t.Errorf("show-page under the create card: Page %v, card %v, title %q; want nothing changed", m.Page, m.TopCard(), m.Create.Title)
	}
	if m.Toast == nil || m.Toast.Text != "Close what is open to show the Attention Page." || m.Toast.Label != "" {
		t.Errorf("show-page under the create card raised toast %+v, want the plain close-it toast", m.Toast)
	}
	// A toast with no label draws no button: Tern would name one after its action, which says nothing.
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if strings.Contains(n.ID, "toast-") && n.P["action"] != nil {
			t.Errorf("the close-it toast %s draws a button: %v", n.ID, n.P)
		}
	}
}

// On the error screen nothing is open to close, and the error already says what is wrong: the click only focuses the Pane.
// A search left open when the error came is not drawn on the error screen, so it does not count as something to close.
func TestShowPageDoesNothingOnTheErrorScreen(t *testing.T) {
	failed := model.Failed{Err: errors.New("kata server not responding")}
	for name, s := range map[string]driver.State{
		"browsing":         feed(t, browsing(t, 160), failed).State,
		"search left open": feed(t, browsing(t, 160), key("/"), failed).State,
	} {
		out := step(t, s, view.DecodeAction("", string(model.ShowPageAction), "attention"))
		if out.State.Model.Page != s.Model.Page || out.State.Model.Toast != nil || len(out.Cmds) != 0 {
			t.Errorf("%s: show-page on the error screen: Page %v, toast %+v, cmds %v; want nothing", name, out.State.Model.Page, out.State.Model.Toast, out.Cmds)
		}
	}
}

// The Page already shown needs nothing closed: the click raises no toast asking to close the help sheet over it.
func TestShowPageRaisesNoToastForThePageAlreadyShown(t *testing.T) {
	on := step(t, projectPane(t, 160, boardCard("p")), view.DecodeAction("", string(model.ShowPageAction), "attention")).State
	helped := step(t, on, key("?")).State
	if helped.Model.Page != model.PageAttention || !helped.Model.HelpOpen {
		t.Fatalf("fixture: Page %v, help open %v", helped.Model.Page, helped.Model.HelpOpen)
	}
	out := step(t, helped, view.DecodeAction("", string(model.ShowPageAction), "attention"))
	if out.State.Model.Toast != nil || len(out.Cmds) != 0 || !out.State.Model.HelpOpen {
		t.Errorf("show-page for the Page shown: toast %+v, cmds %v, help open %v; want nothing", out.State.Model.Toast, out.Cmds, out.State.Model.HelpOpen)
	}
}
