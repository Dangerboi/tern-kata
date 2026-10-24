package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// withLinks is alpha's Comments tab with the cursor on c1, which eli's c2 confirms here and fay's b1 refutes in beta; c2 answers
// c1 and c3 has no links.
func withLinks(t *testing.T) driver.State {
	t.Helper()
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	selected := &model.IssueDetailData{Issue: &alpha, Comments: []model.Comment{
		{UID: "c1", Author: "dana", CreatedAt: at, Body: "claim", Backlinks: []model.CommentLink{
			{UID: ptr("c2"), Kind: "confirm", Author: ptr("eli"), IssueUID: ptr("alpha"), IssueShortID: ptr("aaaa")},
			{UID: ptr("b1"), Kind: "refute", Author: ptr("fay"), IssueUID: ptr("beta"), IssueShortID: ptr("bbbb")}}},
		{UID: "c2", Author: "eli", CreatedAt: at.Add(time.Minute), Body: "agreed", ReplyKind: ptr("confirm"),
			Reply: &model.CommentLink{UID: ptr("c1"), Kind: "confirm", Author: ptr("dana"), IssueUID: ptr("alpha"), IssueShortID: ptr("aaaa")}},
		{UID: "c3", Author: "gus", CreatedAt: at.Add(2 * time.Minute), Body: "unrelated"},
	}}
	return feed(t, browsing(t, 160), model.DetailLoaded{UID: "alpha", Selected: selected}, key("enter")).State
}

func chosenLink(t *testing.T, s driver.State) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if text := wireText(n); strings.Contains(text, `"link `) && strings.Contains(text, "/") {
			return text
		}
	}
	return ""
}

func TestBracketsStepThroughACommentsLinksAndWrap(t *testing.T) {
	s := withLinks(t)
	at := func(want model.CommentLinkChoice) {
		t.Helper()
		got, ok := s.Model.DetailCommentLink()
		if !ok || got != want {
			t.Fatalf("link = %+v (%v), want %+v", got, ok, want)
		}
	}
	at(model.CommentLinkChoice{Index: 1, Count: 2, Label: "eli confirms"})
	s = feed(t, s, key("]")).State
	at(model.CommentLinkChoice{Index: 2, Count: 2, Label: "fay refutes in #bbbb"})
	s = feed(t, s, key("]")).State
	at(model.CommentLinkChoice{Index: 1, Count: 2, Label: "eli confirms"})
	s = feed(t, s, key("[")).State
	at(model.CommentLinkChoice{Index: 2, Count: 2, Label: "fay refutes in #bbbb"})
	s = feed(t, s, key("[")).State
	at(model.CommentLinkChoice{Index: 1, Count: 2, Label: "eli confirms"})
}

func TestTheCommentRowMarksTheChosenLink(t *testing.T) {
	s := withLinks(t)
	if got := chosenLink(t, s); !strings.Contains(got, "link 1/2: eli confirms") {
		t.Fatalf("marked link %q, want link 1/2: eli confirms", got)
	}
	s = feed(t, s, key("]")).State
	if got := chosenLink(t, s); !strings.Contains(got, "link 2/2: fay refutes in #bbbb") {
		t.Fatalf("marked link %q, want link 2/2: fay refutes in #bbbb", got)
	}
	if s = feed(t, s, key("j"), key("j")).State; chosenLink(t, s) != "" {
		t.Fatalf("a comment without links marks %q", chosenLink(t, s))
	}
}

func TestEachCommentStartsOnItsFirstLink(t *testing.T) {
	s := feed(t, withLinks(t), key("]"), key("j"), key("k")).State
	if got, _ := s.Model.DetailCommentLink(); got.Index != 1 {
		t.Fatalf("back on c1 the link is %d, want 1", got.Index)
	}
}

func TestEnterOnAChosenLinkOnThisIssueMovesToThatComment(t *testing.T) {
	s := feed(t, withLinks(t), key("enter")).State
	if m := s.Model; m.SelectedUID != "alpha" || m.DetailIndex() != 1 {
		t.Fatalf("selected %q row %d, want alpha row 1", m.SelectedUID, m.DetailIndex())
	}
	s = feed(t, s, key("enter")).State
	if m := s.Model; m.SelectedUID != "alpha" || m.DetailIndex() != 0 {
		t.Fatalf("answering back: selected %q row %d, want alpha row 0", m.SelectedUID, m.DetailIndex())
	}
}

func TestEnterOnAChosenLinkOnAnotherIssueLandsOnThatComment(t *testing.T) {
	s := feed(t, withLinks(t), key("]"), key("enter")).State
	if s.Model.SelectedUID != "beta" {
		t.Fatalf("selected %q, want beta", s.Model.SelectedUID)
	}
	at := time.Date(2026, 10, 8, 19, 0, 0, 0, time.UTC)
	betaDetail := &model.IssueDetailData{Issue: &beta, Comments: []model.Comment{
		{UID: "b0", Author: "hal", CreatedAt: at, Body: "first"},
		{UID: "b1", Author: "fay", CreatedAt: at.Add(time.Minute), Body: "no"},
	}}
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: betaDetail}).State
	if m := s.Model; m.ActiveTab() != model.TabComments || m.DetailIndex() != 1 {
		t.Fatalf("tab %v row %d, want the Comments tab on b1", m.ActiveTab(), m.DetailIndex())
	}
	if s = feed(t, s, key("escape")).State; s.Model.SelectedUID != "alpha" {
		t.Fatalf("after back selected %q, want alpha", s.Model.SelectedUID)
	}
}

func TestEnterOnAChosenLinkInAnotherProjectOpensItThere(t *testing.T) {
	task := boardCard("task")
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	detail := &model.IssueDetailData{Issue: &task, Comments: []model.Comment{
		{UID: "t1", Author: "dana", CreatedAt: at, Body: "claim", Backlinks: []model.CommentLink{
			{UID: ptr("f1"), Kind: "confirm", Author: ptr("fay"), IssueUID: ptr("far"), IssueShortID: ptr("far"), ProjectUID: ptr("q")}}},
	}}
	s := loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"},
		model.Snapshot{Catalog: twoProjects(), Issues: []model.Issue{task}},
		model.SelectMsg{UID: "task"}, model.DetailLoaded{UID: "task", Selected: detail}, key("enter"))
	if _, ok := s.Model.DetailCommentLink(); !ok {
		t.Fatal("the comment has no link")
	}
	out := step(t, s, key("enter"))
	if !readsProject(out.Cmds, "q") {
		t.Fatalf("enter reads no snapshot of infra: %#v", out.Cmds)
	}
	s = inInfra(t, out.State, out.Cmds)
	far := infraCard("far")
	farDetail := &model.IssueDetailData{Issue: &far, Comments: []model.Comment{
		{UID: "f0", Author: "hal", CreatedAt: at, Body: "first"},
		{UID: "f1", Author: "fay", CreatedAt: at.Add(time.Minute), Body: "agreed"},
	}}
	s = step(t, s, model.DetailLoaded{UID: "far", Selected: farDetail}).State
	if m := s.Model; m.Collection != model.ProjectCollection("q", "infra") || m.SelectedUID != "far" || m.ActiveTab() != model.TabComments || m.DetailIndex() != 1 {
		t.Fatalf("collection %v selected %q tab %v row %d, want infra's far on its Comments tab at f1", m.Collection, m.SelectedUID, m.ActiveTab(), m.DetailIndex())
	}
}

func TestEnterOnACommentWithoutLinksDoesNothing(t *testing.T) {
	s := feed(t, withLinks(t), key("j"), key("j"), key("enter")).State
	if m := s.Model; m.SelectedUID != "alpha" || m.DetailIndex() != 2 {
		t.Fatalf("selected %q row %d, want alpha row 2", m.SelectedUID, m.DetailIndex())
	}
}
