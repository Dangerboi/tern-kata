package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func TestQAsksBeforeQuittingAndYQuits(t *testing.T) {
	asked := step(t, browsing(t, 160), key("q"))
	m := asked.State.Model
	if asked.Quit || !m.AskQuit || m.CurrentMode() != model.ModeQuit {
		t.Fatalf("q: quit=%v asking=%v mode=%q, want the question", asked.Quit, m.AskQuit, m.CurrentMode())
	}
	if !strings.Contains(wireTextOf(t, asked.State), model.QuitQuestion) {
		t.Errorf("the Pane does not draw %q", model.QuitQuestion)
	}
	if !driver.Step(asked.State, key("y"), now).Quit {
		t.Error("y did not quit")
	}
}

func TestNAndEscStayAfterTheQuitQuestion(t *testing.T) {
	for _, chord := range []string{"n", "escape"} {
		s := feed(t, browsing(t, 160), key("q"), key(chord))
		if s.Quit || s.State.Model.AskQuit || s.State.Model.CurrentMode() == model.ModeQuit {
			t.Errorf("%s: quit=%v asking=%v, want the Pane back", chord, s.Quit, s.State.Model.AskQuit)
		}
		if driver.Step(s.State, key("n"), now).Quit {
			t.Errorf("%s: later keys quit", chord)
		}
	}
}

func TestOtherKeysAreSwallowedWhileTheQuitQuestionIsAsked(t *testing.T) {
	asked := step(t, browsing(t, 160), key("q")).State
	for _, chord := range []string{"j", "?", "q", "6", "shift+c", "/"} {
		out := step(t, asked, key(chord))
		if out.Quit || !out.State.Model.AskQuit || out.State.Model.SelectedUID != asked.Model.SelectedUID || out.State.Model.Page != model.PageIssues || out.State.Model.HelpOpen || out.State.Model.Search.Open {
			t.Errorf("%s: quit=%v asking=%v, want the question left as it was", chord, out.Quit, out.State.Model.AskQuit)
		}
	}
}

func TestCtrlCQuitsAtOnceEvenWhileAsked(t *testing.T) {
	s := browsing(t, 160)
	if !driver.Step(s, key("ctrl+c"), now).Quit {
		t.Error("ctrl+c did not quit at once")
	}
	if !driver.Step(step(t, s, key("q")).State, key("ctrl+c"), now).Quit {
		t.Error("ctrl+c did not quit while the question was asked")
	}
}

func TestQQuitsAtOnceWhenKataDoesNotConfirm(t *testing.T) {
	s := browsing(t, 160)
	s.Model.QuitAtOnce = true
	if out := driver.Step(s, key("q"), now); !out.Quit {
		t.Errorf("q did not quit at once: %+v", out)
	}
}

func TestQIsTextInSearchAndCards(t *testing.T) {
	searching := feed(t, browsing(t, 160), key("/"), key("q"))
	if m := searching.State.Model; searching.Quit || m.AskQuit || m.Filters.Search != "q" {
		t.Errorf("search: quit=%v asking=%v text=%q, want q typed", searching.Quit, m.AskQuit, m.Filters.Search)
	}
	for chord, card := range map[string]model.Card{"n": model.CardCreate, "+": model.CardField, "a": model.CardField, "x": model.CardClose} {
		open := feed(t, browsing(t, 160), key(chord), key("q"))
		if m := open.State.Model; open.Quit || m.AskQuit || m.TopCard() != card {
			t.Errorf("%s then q: quit=%v asking=%v card=%v, want the %v card still open", chord, open.Quit, m.AskQuit, m.TopCard(), card)
		}
	}
	field := feed(t, browsing(t, 160), key("+"), key("q"), key("q"))
	if got := field.State.Model.FieldForm.Value; got != "qq" {
		t.Errorf("label field = %q, want qq typed", got)
	}
}

func TestQInTheHelpSheetClosesItAndAsks(t *testing.T) {
	s := feed(t, browsing(t, 160), key("?"), key("q")).State
	if m := s.Model; !m.AskQuit || m.HelpOpen {
		t.Errorf("q in help: asking=%v help=%v, want the question with the sheet closed", m.AskQuit, m.HelpOpen)
	}
}

func TestCtrlJAndCtrlKChangeTheDetailTabs(t *testing.T) {
	detail := feed(t, browsing(t, 160), key("tab")).State
	if detail.Model.Focus != model.ColumnDetail {
		t.Fatalf("focus %v, want the detail", detail.Model.Focus)
	}
	next := step(t, detail, key("tab")).State.Model.ActiveTab()
	if next == detail.Model.ActiveTab() {
		t.Fatal("fixture: tab did not change the tab")
	}
	if got := step(t, detail, key("ctrl+j")).State.Model.ActiveTab(); got != next {
		t.Errorf("ctrl+j: tab %v, want %v", got, next)
	}
	if got := step(t, step(t, detail, key("ctrl+j")).State, key("ctrl+k")).State.Model.ActiveTab(); got != detail.Model.ActiveTab() {
		t.Errorf("ctrl+k after ctrl+j: tab %v, want %v", got, detail.Model.ActiveTab())
	}
	previous := step(t, detail, key("shift+tab")).State.Model.ActiveTab()
	if got := step(t, detail, key("ctrl+k")).State.Model.ActiveTab(); got != previous {
		t.Errorf("ctrl+k: tab %v, want %v", got, previous)
	}
}

func TestBackspaceGoesBackWhereEscDoes(t *testing.T) {
	detail := feed(t, browsing(t, 160), key("tab")).State
	if out := step(t, detail, key("backspace")).State.Model; out.Focus != model.ColumnList {
		t.Errorf("backspace in the detail: focus %v, want the list", out.Focus)
	}
	lane := feed(t, laneBoard(t, 160), key("enter")).State
	if !lane.Model.LaneViewShown() {
		t.Fatal("fixture: no Lane view")
	}
	if out := step(t, lane, key("backspace")).State.Model; out.LaneViewShown() || !out.BoardShown() {
		t.Errorf("backspace in the Lane view: lane=%v board=%v, want the Board", out.LaneViewShown(), out.BoardShown())
	}
	credentials := credentialsOpened(t, projectPane(t, 160, boardCard("a"))).State
	if out := step(t, credentials, key("backspace")).State.Model; out.Page != model.PageIssues {
		t.Errorf("backspace on the Credentials Page: page %v, want Issues", out.Page)
	}
}

func TestCtrlAltAnglesGoToTheFirstAndLastRow(t *testing.T) {
	first := tern.Key{Name: "<", Ctrl: true, Alt: true}
	last := tern.Key{Name: ">", Ctrl: true, Alt: true}
	s := projectPane(t, 160, boardCard("a"), boardCard("b"), boardCard("c"))
	end := step(t, s, last).State.Model.SelectedUID
	if want := step(t, s, key("shift+g")).State.Model.SelectedUID; end != want || end == s.Model.SelectedUID {
		t.Errorf("ctrl+alt+>: selected %q, want the last row %q", end, want)
	}
	if got := step(t, step(t, s, last).State, first).State.Model.SelectedUID; got != s.Model.SelectedUID {
		t.Errorf("ctrl+alt+<: selected %q, want the first row %q", got, s.Model.SelectedUID)
	}
	detail := feed(t, browsing(t, 160), key("tab")).State
	if got, want := step(t, detail, last).State.Model.DetailIndex(), step(t, detail, key("shift+g")).State.Model.DetailIndex(); got != want {
		t.Errorf("detail ctrl+alt+>: row %d, want %d", got, want)
	}
}

// wroteField is the field write among cmds.
func wroteField(cmds []model.Cmd) (model.WriteField, bool) {
	for _, cmd := range cmds {
		if write, ok := cmd.(model.WriteField); ok {
			return write, true
		}
	}
	return model.WriteField{}, false
}

func TestBangThenADigitOrDashSetsOrClearsThePriorityAtOnce(t *testing.T) {
	for _, value := range []string{"0", "2", "4", "-"} {
		out := feed(t, browsing(t, 160), key("!"), key(value))
		write, ok := wroteField(out.Cmds)
		if !ok || write.Action != model.PriorityAction || write.Value != value || write.UID != "alpha" {
			t.Errorf("! then %s: wrote %#v, want the priority at once", value, out.Cmds)
		}
	}
}

func TestBangThenAnotherKeyClosesTheFieldAndIsHandledAsUsual(t *testing.T) {
	s := browsing(t, 160)
	opened := step(t, s, key("!")).State
	if opened.Model.TopCard() != model.CardField {
		t.Fatal("fixture: ! did not open the field")
	}
	out := step(t, opened, key("j"))
	if m := out.State.Model; m.TopCard() != model.NoCard || m.SelectedUID != "beta" {
		t.Errorf("! then j: card %v selected %q, want the field closed and beta selected", m.TopCard(), m.SelectedUID)
	}
	if _, ok := wroteField(out.Cmds); ok {
		t.Error("! then j wrote")
	}
	if m := step(t, opened, key("escape")).State.Model; m.TopCard() != model.NoCard || m.Page != model.PageIssues {
		t.Errorf("! then esc: card %v, want the field closed and nothing else done", m.TopCard())
	}
	if m := step(t, opened, key("5")).State.Model; m.TopCard() != model.NoCard {
		t.Errorf("! then 5: card %v, want the field closed", m.TopCard())
	}
}

func TestTOpensTheAttentionPageFromEveryPageAndTheMenu(t *testing.T) {
	for name, s := range map[string]driver.State{
		"Issues":    projectPane(t, 160, boardCard("a")),
		"Board":     boardPane(t, boardCard("a")),
		"Jobs":      jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false))),
		"Runs":      runsPane(t, 160, finishedRun("R1")),
		"Workflows": workflowsPane(t, 160, nightlyFlow()),
		"Menu":      inSidebar(projectPane(t, 160, boardCard("a"))),
	} {
		s.Model.Actor = "dana"
		out := step(t, s, key("shift+t"))
		if _, ok := askedAttention(out.Cmds); !ok || out.State.Model.Page != model.PageAttention {
			t.Errorf("%s: T gave page %v and %#v, want the Attention Page and a read", name, out.State.Model.Page, out.Cmds)
		}
	}
}

// inSidebar is s with the keys in the Menu's sidebar, which lists the Collections.
func inSidebar(s driver.State) driver.State {
	s.Model.Focus = model.ColumnCollections
	return s
}

func TestCtrlCQuitsAtOnceFromEveryCard(t *testing.T) {
	for card, open := range cardOpeners() {
		if !driver.Step(open(t), key("ctrl+c"), now).Quit {
			t.Errorf("ctrl+c did not quit while the %q card was open", card.Name())
		}
	}
}
