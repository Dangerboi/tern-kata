package driver_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// closedCards is n cards closed on October days 1..n at noon: card n closed last.
func closedCards(n int) []model.Issue {
	var issues []model.Issue
	for i := 1; i <= n; i++ {
		issues = append(issues, closedAt(fmt.Sprintf("c%02d", i), time.October, i))
	}
	return issues
}

func doneHead(t *testing.T, s driver.State) string {
	t.Helper()
	heads, _ := lanes(t, s)
	return heads[len(heads)-1]
}

func TestDoneShowsTheTwentyNewestClosedAndSaysHowManyQualify(t *testing.T) {
	var issues []model.Issue
	for i := 0; i < 25; i++ {
		at := time.Date(2026, 10, 8, 0, i, 0, 0, time.UTC)
		issue := boardCard(fmt.Sprintf("c%02d", i))
		issue.Status, issue.ClosedAt = "closed", &at
		issues = append(issues, issue)
	}
	s := boardPane(t, issues...)
	_, cards := lanes(t, s)
	done := cards[len(cards)-1]
	if len(done) != 20 || done[0] != "c24" || done[19] != "c05" {
		t.Errorf("Done holds %v, want the 20 newest closed, newest first", done)
	}
	if head := doneHead(t, s); !strings.Contains(head, "20 of 25") {
		t.Errorf("head = %s, want 20 of 25", head)
	}
	if head := doneHead(t, boardPane(t, closedCards(3)...)); strings.Contains(head, " of ") {
		t.Errorf("head = %s, want no total when everything shows", head)
	}
}

func TestSavedDoneLimitsChangeDone(t *testing.T) {
	s := boardPane(t, closedCards(6)...)
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", DoneCards: 2}).State
	wantLane(t, s, 5, "Done", "c06", "c05")
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", DoneDays: 7}).State
	wantLane(t, s, 5, "Done", "c06", "c05", "c04", "c03", "c02", "c01")
	s = step(t, s, model.LanesSaved{ProjectUID: "p", DoneDays: 2, DoneCards: 1}).State
	wantLane(t, s, 5, "Done", "c06")
	days, cards := s.Model.DoneLimits()
	if days != 2 || cards != 1 {
		t.Errorf("limits %d, %d", days, cards)
	}
}

func TestLanesCardTabsToDoneFieldsAndSavesThem(t *testing.T) {
	s := step(t, boardPane(t, closedCards(2)...), model.LanesLoaded{ProjectUID: "p", DoneDays: 30}).State
	s = step(t, s, key("shift+b")).State
	if got := s.Model.LanesForm.Days; got != "30" || s.Model.LanesForm.Cards != "" {
		t.Fatalf("prefill days %q cards %q", got, s.Model.LanesForm.Cards)
	}
	s = step(t, s, key("tab")).State
	if s.Model.LanesForm.Field != model.LanesDays || !strings.HasPrefix(s.Focus, view.LanesFieldID(model.LanesDays)) {
		t.Fatalf("field %q focus %q", s.Model.LanesForm.Field, s.Focus)
	}
	s = typed(t, step(t, s, key("ctrl+u")).State, "7")
	s = typed(t, step(t, s, key("tab")).State, "5")
	if s.Model.LanesForm.Days != "7" || s.Model.LanesForm.Cards != "5" {
		t.Fatalf("days %q cards %q", s.Model.LanesForm.Days, s.Model.LanesForm.Cards)
	}
	if got := step(t, s, key("shift+tab")).State.Model.LanesForm.Field; got != model.LanesDays {
		t.Errorf("shift+tab went to %q", got)
	}
	out := step(t, s, key("enter"))
	want := []model.Cmd{model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: model.DefaultLanes(), DoneDays: 7, DoneCards: 5}}
	if !reflect.DeepEqual(out.Cmds, want) {
		t.Fatalf("enter sent %#v, want %#v", out.Cmds, want)
	}
}

func TestLanesCardBlankDoneFieldsRestoreTheDefaults(t *testing.T) {
	s := step(t, boardPane(t), model.LanesLoaded{ProjectUID: "p", DoneDays: 30, DoneCards: 5}).State
	s = step(t, s, key("shift+b")).State
	s = step(t, step(t, s, key("tab")).State, key("ctrl+u")).State
	s = step(t, step(t, s, key("tab")).State, key("ctrl+u")).State
	out := step(t, s, key("enter"))
	if len(out.Cmds) != 1 {
		t.Fatalf("cmds %#v", out.Cmds)
	}
	if save, _ := out.Cmds[0].(model.SaveLanes); save.DoneDays != 0 || save.DoneCards != 0 {
		t.Errorf("saved %#v, want default Done", save)
	}
}

func TestLanesCardRefusesBadDoneLimitsNamingTheField(t *testing.T) {
	for _, tt := range []struct {
		tabs       int
		text, want string
	}{
		{1, "0", "Done days"}, {1, "-3", "Done days"}, {1, "x", "Done days"}, {1, "1.5", "Done days"},
		{2, "0", "Done cards"}, {2, "ten", "Done cards"},
	} {
		t.Run(fmt.Sprintf("%d %s", tt.tabs, tt.text), func(t *testing.T) {
			s := lanesCard(t)
			for i := 0; i < tt.tabs; i++ {
				s = step(t, s, key("tab")).State
			}
			out := step(t, typed(t, s, tt.text), key("enter"))
			msg, ok := nodeText(t, out.State, view.LanesID+".error")
			if len(out.Cmds) != 0 || out.State.Model.TopCard() != model.CardLanes || !ok || !strings.Contains(msg, tt.want) {
				t.Errorf("cmds %v card %v error %q (drawn %v), want a refusal naming %q", out.Cmds, out.State.Model.TopCard(), msg, ok, tt.want)
			}
		})
	}
}
