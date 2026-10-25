package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func typed(t *testing.T, s driver.State, text string) driver.State {
	t.Helper()
	for _, r := range text {
		chord := string(r)
		if r == ' ' {
			chord = "space"
		}
		s = step(t, s, key(chord)).State
	}
	return s
}

// lanesCard is the Board with the Lanes card open and its field emptied.
func lanesCard(t *testing.T, issues ...model.Issue) driver.State {
	t.Helper()
	return step(t, boardPane(t, issues...), key("shift+b")).State
}

func nodeText(t *testing.T, s driver.State, id string) (string, bool) {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id {
			text, _ := n.P["text"].(string)
			return text, true
		}
	}
	return "", false
}

func lanesField(t *testing.T, s driver.State) string {
	t.Helper()
	text, _ := nodeText(t, s, fieldLabeled(t, s.Model, "Lane list"))
	return text
}

func TestBOpensTheLanesCardPrefilledAndEscClosesItUnchanged(t *testing.T) {
	board := boardPane(t, boardCard("a", "needs-triage"))
	board = step(t, board, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"in-review", "shipping"}}).State
	open := step(t, board, key("shift+b"))
	if open.State.Model.CurrentMode() != model.ModeLanesForm || open.State.Model.TopCard() != model.CardLanes || len(open.Cmds) != 0 {
		t.Fatalf("mode %s card %v cmds %v", open.State.Model.CurrentMode(), open.State.Model.TopCard(), open.Cmds)
	}
	if got := lanesField(t, open.State); got != "in-review, shipping" {
		t.Errorf("field = %q, want the current Lane list", got)
	}
	if !strings.HasPrefix(open.State.Focus, layerTitled(t, open.State.Model, "Lanes")) {
		t.Errorf("text focus %q is not on the Lanes card", open.State.Focus)
	}
	edited := typed(t, open.State, "x")
	closed := step(t, edited, key("escape"))
	if closed.State.Model.TopCard() != model.NoCard || len(closed.Cmds) != 0 || !reflect.DeepEqual(closed.State.Model.LaneList(), board.Model.LaneList()) {
		t.Fatalf("esc: card %v cmds %v lanes %v", closed.State.Model.TopCard(), closed.Cmds, closed.State.Model.LaneList())
	}
}

func TestBDoesNothingOffTheBoard(t *testing.T) {
	out := step(t, projectPane(t, 160, boardCard("a")), key("shift+b"))
	if out.State.Model.TopCard() != model.NoCard {
		t.Fatalf("B opened %v off the Board", out.State.Model.TopCard())
	}
}

func TestEnterSavesTheNormalisedListWithOneSaveAndTheBoardChanges(t *testing.T) {
	s := lanesCard(t, boardCard("a", "in-review"))
	s = typed(t, step(t, s, key("ctrl+u")).State, "In review,  Ready  to ship ,wip")
	out := step(t, s, key("enter"))
	want := []model.Cmd{model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"in-review", "ready-to-ship", "wip"}}}
	if !reflect.DeepEqual(out.Cmds, want) {
		t.Fatalf("enter sent %#v, want exactly %#v", out.Cmds, want)
	}
	if out.State.Model.TopCard() != model.CardLanes {
		t.Errorf("the card closed before the save answered: %v", out.State.Model.TopCard())
	}
	saved := step(t, out.State, model.LanesSaved{ProjectUID: "p", Lanes: []string{"in-review", "ready-to-ship", "wip"}}).State
	wantLane(t, saved, 1, "In review", "a")
	wantLane(t, saved, 2, "Ready to ship")
	wantLane(t, saved, 3, "Wip")
}

func TestLanesCardRefusesBadNamesNamingTheEntry(t *testing.T) {
	long := strings.Repeat("x", 65)
	for _, tt := range []struct{ name, text, want string }{
		{"empty entry", "a,, b", "Entry 2"},
		{"trailing comma", "a, b,", "Entry 3"},
		{"nothing left after tidying", "a, !!!", `"!!!"`},
		{"too long", "a, " + long, `"` + long[:20]},
		{"duplicate", "In review, ready, in-review", `"in-review"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := typed(t, step(t, lanesCard(t), key("ctrl+u")).State, tt.text)
			out := step(t, s, key("enter"))
			if len(out.Cmds) != 0 || out.State.Model.TopCard() != model.CardLanes {
				t.Fatalf("cmds %v card %v, want a refusal with the card open", out.Cmds, out.State.Model.TopCard())
			}
			msg, ok := errorOnCard(t, out.State.Model)
			if !ok || !strings.Contains(msg, tt.want) {
				t.Errorf("error = %q (drawn %v), want it to name %q", msg, ok, tt.want)
			}
			if got := lanesField(t, out.State); got != tt.text {
				t.Errorf("a refusal changed the field to %q", got)
			}
		})
	}
}

func TestEmptyLanesFieldSavesNoLanesWhichRestoresTheDefault(t *testing.T) {
	s := step(t, lanesCard(t), key("ctrl+u")).State
	out := step(t, s, key("enter"))
	want := []model.Cmd{model.SaveLanes{ProjectUID: "p", ProjectName: "work"}}
	if !reflect.DeepEqual(out.Cmds, want) {
		t.Fatalf("enter sent %#v, want %#v", out.Cmds, want)
	}
}

func TestLanesCardHintListsLabelsThatAreNotLanesYet(t *testing.T) {
	s := lanesCard(t, boardCard("a", "needs-triage", "bug"), boardCard("b", "docs", "bug"))
	hint, ok := textOnCard(t, s.Model, "Not Lanes yet")
	if !ok || !strings.Contains(hint, "bug, docs") || strings.Contains(hint, "needs-triage") {
		t.Fatalf("hint = %q (drawn %v), want bug and docs only", hint, ok)
	}
	s = step(t, lanesCard(t, boardCard("a", "needs-triage")), key("a")).State
	if _, ok := textOnCard(t, s.Model, "Not Lanes yet"); ok {
		t.Error("the hint is drawn although every label is a Lane")
	}
}

func TestLanesCardLetsNoGlobalChordThrough(t *testing.T) {
	s := lanesCard(t)
	for _, chord := range []string{"?", "q"} {
		out := step(t, s, key(chord))
		if out.State.Model.Quitting || out.State.Model.HelpOpen || out.State.Model.TopCard() != model.CardLanes {
			t.Errorf("%s: quitting=%v help=%v card=%v", chord, out.State.Model.Quitting, out.State.Model.HelpOpen, out.State.Model.TopCard())
		}
	}
	if got := lanesField(t, step(t, s, tern.Key{Name: "z", Text: "z"}).State); !strings.HasSuffix(got, "z") {
		t.Errorf("a typed key did not reach the field: %q", got)
	}
}

func TestBWaitsForTheSavedLaneListToLoad(t *testing.T) {
	s := feed(t, projectPane(t, 160, boardCard("a")), key("b")).State
	out := step(t, s, key("shift+b"))
	if out.State.Model.TopCard() != model.NoCard || out.State.Model.Notice == "" {
		t.Fatalf("card %v notice %q, want no card and a Notice", out.State.Model.TopCard(), out.State.Model.Notice)
	}
	out = step(t, step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"in-review"}}).State, key("shift+b"))
	if out.State.Model.TopCard() != model.CardLanes {
		t.Fatalf("card %v after the load answered", out.State.Model.TopCard())
	}
}

func TestBOpensTheLanesCardInTheLaneViewOfANarrowPane(t *testing.T) {
	s := feed(t, projectPane(t, 120, boardCard("a", "needs-info")), key("b"), model.LanesLoaded{ProjectUID: "p"}).State
	if got := step(t, s, key("shift+b")).State.Model.TopCard(); got != model.CardLanes {
		t.Fatalf("card %v", got)
	}
}

func TestALoadThatDropsTheNarrowLaneViewsLaneRepicksIt(t *testing.T) {
	s := feed(t, projectPane(t, 120, boardCard("a", "needs-info")), key("b")).State
	if !strings.Contains(laneHeader(t, s), "Needs info") {
		t.Fatalf("fixture: header %s", laneHeader(t, s))
	}
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"in-review"}}).State
	if got := laneHeader(t, s); !strings.Contains(got, "No lane") || !reflect.DeepEqual(laneRows(t, s), []string{"a"}) {
		t.Errorf("header %s rows %v, want a under No lane", got, laneRows(t, s))
	}
}

func TestLaneViewTogglesTheDetailLikeTheList(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("enter")).State
	if got := step(t, s, key("shift+l")).State; drawsDetail(t, got) {
		t.Error("L left the detail drawn")
	}
}

func savingLanesCard(t *testing.T) driver.State {
	t.Helper()
	s := typed(t, step(t, lanesCard(t), key("ctrl+u")).State, "wip, shipping")
	return step(t, s, key("enter")).State
}

func TestBWithAnUnreadableFileLeavesANoticeAndNoCard(t *testing.T) {
	s := step(t, boardPane(t, boardCard("a")), model.LanesLoaded{ProjectUID: "p", Err: errors.New("lanes.toml: bad syntax")}).State
	s.Model.Notice = ""
	out := step(t, s, key("shift+b")).State
	if out.Model.TopCard() != model.NoCard || !strings.Contains(out.Model.Notice, "bad syntax") {
		t.Fatalf("card %v Notice %q, want no card and the file's error", out.Model.TopCard(), out.Model.Notice)
	}
}

func TestLanesCardSavingKeepsTheCardLockedWithoutACaret(t *testing.T) {
	s := savingLanesCard(t)
	if s.Model.TopCard() != model.CardLanes {
		t.Fatalf("card %v", s.Model.TopCard())
	}
	if _, _, ok := s.Model.TextCaret(); ok {
		t.Error("the caret shows while saving")
	}
	out := step(t, typed(t, s, "x"), key("enter"))
	if got := lanesField(t, out.State); got != "wip, shipping" || len(out.Cmds) != 0 {
		t.Errorf("field %q cmds %v, want the text kept and no second save", got, out.Cmds)
	}
}

func TestAFailedLanesSaveKeepsTheCardWithTextAndError(t *testing.T) {
	s := step(t, savingLanesCard(t), model.LanesSaved{ProjectUID: "p", Lanes: []string{"wip", "shipping"}, Err: errors.New("disk full")}).State
	msg, ok := errorOnCard(t, s.Model)
	if s.Model.TopCard() != model.CardLanes || !ok || !strings.Contains(msg, "disk full") || lanesField(t, s) != "wip, shipping" {
		t.Fatalf("card %v error %q (drawn %v) field %q", s.Model.TopCard(), msg, ok, lanesField(t, s))
	}
	if _, _, ok := s.Model.TextCaret(); !ok {
		t.Error("the card is not editable again")
	}
	if out := step(t, s, key("enter")); len(out.Cmds) != 1 {
		t.Errorf("retry sent %v", out.Cmds)
	}
}

func TestASuccessfulLanesSaveClosesTheCardAndChangesTheBoard(t *testing.T) {
	s := step(t, savingLanesCard(t), model.LanesSaved{ProjectUID: "p", Lanes: []string{"wip", "shipping"}}).State
	if s.Model.TopCard() != model.NoCard {
		t.Fatalf("card %v", s.Model.TopCard())
	}
	wantLane(t, s, 1, "Wip")
}

func TestALanesSaveResultAfterCancelOrForAnotherProjectChangesNoCard(t *testing.T) {
	cancelled := step(t, savingLanesCard(t), key("escape")).State
	for _, msg := range []model.LanesSaved{{ProjectUID: "p", Err: errors.New("x")}, {ProjectUID: "p", Lanes: []string{"wip"}}} {
		if got := step(t, cancelled, msg).State.Model.TopCard(); got != model.NoCard {
			t.Errorf("%+v reopened %v", msg, got)
		}
	}
	other := step(t, savingLanesCard(t), model.LanesSaved{ProjectUID: "other", Lanes: []string{"wip"}}).State
	if other.Model.TopCard() != model.CardLanes {
		t.Errorf("another Project's result closed the card: %v", other.Model.TopCard())
	}
}

func TestLanesCardRefusesDoneLimitsAboveTheMaximumNamingTheField(t *testing.T) {
	for _, tt := range []struct {
		tabs       int
		text, want string
	}{{1, "3651", "Done days"}, {2, "501", "Done cards"}} {
		s := lanesCard(t)
		for range tt.tabs {
			s = step(t, s, key("tab")).State
		}
		out := step(t, typed(t, s, tt.text), key("enter"))
		msg, ok := errorOnCard(t, out.State.Model)
		if len(out.Cmds) != 0 || !ok || !strings.Contains(msg, tt.want) {
			t.Errorf("%s: cmds %v error %q, want a refusal naming %q", tt.text, out.Cmds, msg, tt.want)
		}
	}
	s := typed(t, step(t, lanesCard(t), key("tab")).State, "3650")
	if out := step(t, s, key("enter")); len(out.Cmds) != 1 {
		t.Errorf("3650 refused: %v", out.Cmds)
	}
}

func TestALoadedDoneLimitAboveTheMaximumCountsAsUnset(t *testing.T) {
	s := step(t, boardPane(t, boardCard("a")), model.LanesLoaded{ProjectUID: "p", DoneDays: 99999, DoneCards: 501}).State
	s = step(t, s, key("shift+b")).State
	for _, label := range []string{"Days back", "Most cards"} {
		if text, _ := nodeText(t, s, fieldLabeled(t, s.Model, label)); text != "" {
			t.Errorf("%s = %q, want blank (the default)", label, text)
		}
	}
}

func TestAnEarlierCancelledSavesResultLeavesTheNewerCardAlone(t *testing.T) {
	first := savingLanesCard(t)
	s := step(t, first, key("escape")).State
	s = typed(t, step(t, s, key("shift+b")).State, "x")
	s = step(t, s, key("enter")).State
	if !s.Model.LanesForm.Saving {
		t.Fatalf("fixture: second card not saving: %+v", s.Model.LanesForm)
	}
	stale := model.LanesSaved{ProjectUID: "p", Lanes: []string{"wip", "shipping"}}
	for _, msg := range []model.LanesSaved{stale, {ProjectUID: "p", Lanes: []string{"wip", "shipping"}, Err: errors.New("late")}} {
		got := step(t, s, msg).State.Model
		if got.TopCard() != model.CardLanes || !got.LanesForm.Saving || got.LanesForm.Err != nil {
			t.Fatalf("%+v touched the newer card: %v %+v", msg, got.TopCard(), got.LanesForm)
		}
	}
	own := model.LanesSaved{ProjectUID: "p", Lanes: s.Model.LanesForm.Sent.Lanes}
	if got := step(t, s, own).State.Model.TopCard(); got != model.NoCard {
		t.Errorf("its own result left card %v open", got)
	}
}

func TestALoadedDoneLimitAtTheMaximumStaysSet(t *testing.T) {
	s := step(t, boardPane(t, boardCard("a")), model.LanesLoaded{ProjectUID: "p", DoneDays: 3650, DoneCards: 500}).State
	if s.Model.Board.DoneDays != 3650 || s.Model.Board.DoneCards != 500 {
		t.Errorf("limits %d days %d cards, want 3650 and 500", s.Model.Board.DoneDays, s.Model.Board.DoneCards)
	}
	s = step(t, s, key("shift+b")).State
	for label, want := range map[string]string{"Days back": "3650", "Most cards": "500"} {
		if text, _ := nodeText(t, s, fieldLabeled(t, s.Model, label)); text != want {
			t.Errorf("%s = %q, want %q", label, text, want)
		}
	}
}
