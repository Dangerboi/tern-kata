package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func issueWith(uid string, comments ...model.Comment) *model.IssueDetailData {
	return &model.IssueDetailData{Issue: &model.Issue{UID: uid}, Comments: comments}
}

func arrive(m model.Model, uid string, comments ...model.Comment) model.Model {
	m, _ = model.Update(m, model.ResizeMsg{Cols: m.Cols})
	m, _ = model.Update(m, model.DetailLoaded{UID: uid, Selected: issueWith(uid, comments...)})
	return m
}

func TestGoingBackRestoresTheTabAndRowAcrossSeveralJumps(t *testing.T) {
	m := model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a",
		Issues: []model.Issue{{UID: "a"}, {UID: "b"}, {UID: "c"}}}
	m = arrive(m, "a", model.Comment{UID: "a1"}, model.Comment{UID: "a2"}, model.Comment{UID: "a3"})
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabComments})
	m, _ = clickPress(m, "j", "j") // a3
	m, _ = model.Update(m, model.JumpIssueMsg{UID: "b"})
	m = arrive(m, "b", model.Comment{UID: "b1"}, model.Comment{UID: "b2"})
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabComments})
	m, _ = clickPress(m, "j") // b2
	m, _ = model.Update(m, model.JumpIssueMsg{UID: "c"})
	m = arrive(m, "c")

	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	m = arrive(m, "b", model.Comment{UID: "b1"}, model.Comment{UID: "b2"})
	if m.SelectedUID != "b" || m.ActiveTab() != model.TabComments || cursorKey(m) != "b2" {
		t.Fatalf("back to b: %s tab=%v row=%q", m.SelectedUID, m.ActiveTab(), cursorKey(m))
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	m = arrive(m, "a", model.Comment{UID: "a1"}, model.Comment{UID: "a2"}, model.Comment{UID: "a3"})
	if m.SelectedUID != "a" || m.ActiveTab() != model.TabComments || cursorKey(m) != "a3" {
		t.Fatalf("back to a: %s tab=%v row=%q", m.SelectedUID, m.ActiveTab(), cursorKey(m))
	}
}

func TestGoingBackToARowThatDisappearedLandsOnTheTabsFirstRow(t *testing.T) {
	m := model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}, {UID: "b"}}}
	m = arrive(m, "a", model.Comment{UID: "a1"}, model.Comment{UID: "a2"})
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabComments})
	m, _ = clickPress(m, "j")
	m, _ = model.Update(m, model.JumpIssueMsg{UID: "b"})
	m = arrive(m, "b")
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	m = arrive(m, "a", model.Comment{UID: "a1"})
	if cursorKey(m) != "a1" || m.ActiveTab() != model.TabComments {
		t.Fatalf("row=%q tab=%v", cursorKey(m), m.ActiveTab())
	}
}

func replyTo(issue, comment string) model.Comment {
	return model.Comment{UID: "r", ReplyKind: new("confirm"), Reply: &model.CommentLink{UID: &comment, Kind: "confirm", IssueUID: &issue}}
}

func TestEnterOnAReplyIntoAnotherIssueLandsOnTheAnsweredComment(t *testing.T) {
	m := model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}, {UID: "b"}}}
	m = arrive(m, "a", replyTo("b", "b2"))
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabComments})
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	if m.SelectedUID != "b" {
		t.Fatalf("selected %s", m.SelectedUID)
	}
	m = arrive(m, "b", model.Comment{UID: "b1"}, model.Comment{UID: "b2"}, model.Comment{UID: "b3"})
	if m.ActiveTab() != model.TabComments || cursorKey(m) != "b2" {
		t.Fatalf("tab=%v row=%q", m.ActiveTab(), cursorKey(m))
	}
}

func TestEnterOnAReplyToAMissingCommentLandsOnTheFirstCommentsRow(t *testing.T) {
	m := model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}, {UID: "b"}}}
	m = arrive(m, "a", replyTo("b", "gone"))
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabComments})
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	m = arrive(m, "b", model.Comment{UID: "b1"}, model.Comment{UID: "b2"})
	if m.ActiveTab() != model.TabComments || cursorKey(m) != "b1" {
		t.Fatalf("tab=%v row=%q", m.ActiveTab(), cursorKey(m))
	}
}

func TestEnterOnAReplyInTheSameIssueMovesTheCursorWithoutAJump(t *testing.T) {
	m := onComments()
	m, cmds := clickPress(m, "j", "enter")
	if m.SelectedUID != "a" || cursorKey(m) != "c1" || len(cmds) != 0 {
		t.Fatalf("selected=%s row=%q cmds=%v", m.SelectedUID, cursorKey(m), cmds)
	}
}
