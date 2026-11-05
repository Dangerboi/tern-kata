package model_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// treeOf is the Comments tab's rows of the comments, in a detail Issue "a".
func treeOf(comments ...model.Comment) []model.DetailItem {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 7}
	m := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a", Actor: "me", Focus: model.ColumnDetail}, &model.IssueDetailData{Issue: &issue, Comments: comments})
	return m.DetailRows()
}

func answering(uid, kind string) *model.CommentLink {
	return &model.CommentLink{UID: &uid, Kind: kind, IssueUID: new("a")}
}

func depths(rows []model.DetailItem) string {
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s:%d", r.Key, r.Depth))
	}
	return strings.Join(out, " ")
}

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestCommentsNestRepliesUnderTheirTargets(t *testing.T) {
	rows := treeOf(
		model.Comment{UID: "c1", Author: "dana", CreatedAt: t0},
		model.Comment{UID: "c2", Author: "eli", CreatedAt: t0.Add(1), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
		model.Comment{UID: "c3", Author: "fay", CreatedAt: t0.Add(2)},
		model.Comment{UID: "c4", Author: "gus", CreatedAt: t0.Add(3), Reply: answering("c2", "reply"), ReplyKind: new("reply")},
		model.Comment{UID: "c5", Author: "hal", CreatedAt: t0.Add(4), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
	)
	if got, want := depths(rows), "c1:0 c2:1 c4:2 c5:1 c3:0"; got != want {
		t.Fatalf("tree %q, want %q", got, want)
	}
}

func TestCommentsOrderSiblingsByTimeThenUID(t *testing.T) {
	rows := treeOf(
		model.Comment{UID: "c1", CreatedAt: t0},
		model.Comment{UID: "cz", CreatedAt: t0.Add(5), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
		model.Comment{UID: "cb", CreatedAt: t0.Add(5), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
		model.Comment{UID: "ca", CreatedAt: t0.Add(9), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
	)
	if got, want := depths(rows), "c1:0 cb:1 cz:1 ca:1"; got != want {
		t.Fatalf("tree %q, want %q", got, want)
	}
}

func TestCommentsStopNestingAtTheDepthCap(t *testing.T) {
	var comments []model.Comment
	for i := range 7 {
		c := model.Comment{UID: fmt.Sprintf("c%d", i), CreatedAt: t0.Add(time.Duration(i))}
		if i > 0 {
			c.Reply, c.ReplyKind = answering(fmt.Sprintf("c%d", i-1), "reply"), new("reply")
		}
		comments = append(comments, c)
	}
	if got, want := depths(treeOf(comments...)), "c0:0 c1:1 c2:2 c3:3 c4:4 c5:4 c6:4"; got != want {
		t.Fatalf("tree %q, want %q", got, want)
	}
}

func TestCommentsShowACycleOnce(t *testing.T) {
	rows := treeOf(
		model.Comment{UID: "c1", CreatedAt: t0, Reply: answering("c2", "reply"), ReplyKind: new("reply")},
		model.Comment{UID: "c2", CreatedAt: t0.Add(1), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
	)
	if len(rows) != 2 || !slices.ContainsFunc(rows, func(r model.DetailItem) bool { return r.Key == "c1" }) || !slices.ContainsFunc(rows, func(r model.DetailItem) bool { return r.Key == "c2" }) {
		t.Fatalf("cycle rows %s", depths(rows))
	}
}

func TestCommentsShowAReplyFromAnotherIssueAsALeaf(t *testing.T) {
	foreign := model.CommentLink{UID: new("x9"), Kind: "confirm", Author: new("ivy"), IssueUID: new("b"), IssueShortID: new("bbbb"), Handle: new("c:x9"), CreatedAt: t0.Add(3), TargetEdited: true}
	rows := treeOf(
		model.Comment{UID: "c1", CreatedAt: t0, Backlinks: []model.CommentLink{foreign}},
		model.Comment{UID: "c2", CreatedAt: t0.Add(1), Reply: answering("c1", "reply"), ReplyKind: new("reply")},
	)
	if len(rows) != 2 {
		t.Fatalf("a foreign reply got its own row: %s", depths(rows))
	}
	leaves := rows[0].Leaves
	if len(leaves) != 1 || leaves[0].Depth != 1 || !leaves[0].Edited || !strings.HasPrefix(leaves[0].Text, "↳ confirm c:x9 in #bbbb by ivy") {
		t.Fatalf("leaves %+v", leaves)
	}
}

func TestCommentsTagConfirmAndRefuteWithBeforeEdit(t *testing.T) {
	tags := func(links ...model.CommentLink) string {
		var out []string
		for _, s := range treeOf(model.Comment{UID: "c1", CreatedAt: t0, Backlinks: links})[0].Activity.Tags {
			out = append(out, s.Text)
		}
		return strings.Join(out, "|")
	}
	fresh := model.CommentLink{UID: new("r1"), Kind: "confirm"}
	stale := model.CommentLink{UID: new("r2"), Kind: "confirm", TargetEdited: true}
	refuted := model.CommentLink{UID: new("r3"), Kind: "refute", TargetEdited: true}
	for _, tc := range []struct {
		links []model.CommentLink
		want  string
	}{
		{nil, ""},
		{[]model.CommentLink{fresh}, "confirm"},
		{[]model.CommentLink{stale}, "confirm (before edit)"},
		{[]model.CommentLink{stale, fresh}, "confirm"},
		{[]model.CommentLink{stale, refuted}, "confirm (before edit)|refute (before edit)"},
		{[]model.CommentLink{{UID: new("r4"), Kind: "reply"}}, ""},
	} {
		if got := tags(tc.links...); got != tc.want {
			t.Errorf("tags of %d links = %q, want %q", len(tc.links), got, tc.want)
		}
	}
}

func TestCommentsAReplyToAnEditedTargetSaysSo(t *testing.T) {
	target := answering("c1", "confirm")
	target.TargetEdited = true
	rows := treeOf(model.Comment{UID: "c1", Author: "dana"}, model.Comment{UID: "c2", Author: "eli", CreatedAt: t0, Reply: target, ReplyKind: new("confirm")})
	if got := rows[1].Activity.Reply; !strings.HasSuffix(got, "Target edited after this reply") {
		t.Fatalf("reply note %q", got)
	}
}
