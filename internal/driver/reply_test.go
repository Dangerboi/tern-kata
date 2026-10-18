package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func ptr[T any](v T) *T { return &v }

// onReplies is the Pane with alpha's Comments tab focused: a comment with two replies, a confirmation of it, and a refutation
// of a comment in beta.
func onReplies(t *testing.T) driver.State {
	t.Helper()
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	selected := &model.IssueDetailData{Issue: &alpha, Comments: []model.Comment{
		{UID: "c1", Author: "dana", CreatedAt: at, Body: "claim", Backlinks: []model.CommentLink{{Kind: "confirm"}, {Kind: "refute"}}},
		{UID: "c2", Author: "eli", CreatedAt: at.Add(time.Minute), Body: "agreed", ReplyKind: ptr("confirm"),
			Reply: &model.CommentLink{UID: ptr("c1"), Kind: "confirm", Author: ptr("dana"), IssueUID: ptr("alpha"), IssueShortID: ptr("aaaa")}},
		{UID: "c3", Author: "eli", CreatedAt: at.Add(2 * time.Minute), Body: "no", ReplyKind: ptr("refute"),
			Reply: &model.CommentLink{UID: ptr("b1"), Kind: "refute", Author: ptr("fay"), IssueUID: ptr("beta"), IssueShortID: ptr("bbbb")}},
	}}
	return feed(t, browsing(t, 160), model.DetailLoaded{UID: "alpha", Selected: selected}, key("enter")).State
}

func replyNotes(m model.Model) []string {
	var notes []string
	for _, item := range m.DetailRows() {
		notes = append(notes, strings.TrimSpace(item.Activity.Reply+" "+item.Activity.Replies))
	}
	return notes
}

func TestReplyRowsShowKindTargetAndReplyCount(t *testing.T) {
	got := strings.Join(replyNotes(onReplies(t).Model), "|")
	if want := "2 replies|confirms dana's comment|refutes fay's comment in #bbbb"; got != want {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

func TestEnterOnAReplyMovesToTheAnsweredCommentInThisIssue(t *testing.T) {
	s := feed(t, onReplies(t), key("j"), key("enter")).State
	if s.Model.SelectedUID != "alpha" || s.Model.DetailIndex() != 0 {
		t.Fatalf("selected %q row %d, want alpha row 0", s.Model.SelectedUID, s.Model.DetailIndex())
	}
}

func TestEnterOnAReplyToAnotherIssueOpensItAndBackReturns(t *testing.T) {
	s := feed(t, onReplies(t), key("j"), key("j"), key("enter")).State
	if s.Model.SelectedUID != "beta" {
		t.Fatalf("selected %q, want beta", s.Model.SelectedUID)
	}
	s = feed(t, s, key("escape")).State
	if s.Model.SelectedUID != "alpha" {
		t.Fatalf("after back selected %q, want alpha", s.Model.SelectedUID)
	}
}

func TestCommentsWithoutReplyInformationShowAsBefore(t *testing.T) {
	selected := &model.IssueDetailData{Issue: &alpha, Comments: []model.Comment{{UID: "c1", Author: "dana", Body: "plain"}}}
	s := feed(t, browsing(t, 160), model.DetailLoaded{UID: "alpha", Selected: selected}, key("enter")).State
	if got := replyNotes(s.Model); len(got) != 1 || got[0] != "" {
		t.Fatalf("notes = %q", got)
	}
}
