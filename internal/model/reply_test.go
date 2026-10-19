package model_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// onComments is the Issue detail's Comments tab with the cursor on the first row: dana's comment, eli's reply to it, and
// a child-less Issue, so the rows are exactly the comments.
func onComments() model.Model {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 7}
	comments := []model.Comment{
		{UID: "c1", Author: "dana", Body: "claim"},
		{UID: "c2", Author: "eli", Body: "agreed", ReplyKind: new("confirm"), Reply: &model.CommentLink{UID: new("c1"), Kind: "confirm", Author: new("dana"), IssueUID: new("a")}},
	}
	return withDetail(model.Model{Workspace: "/repo", SelectedUID: "a", Actor: "me", Focus: model.ColumnDetail}, &model.IssueDetailData{Issue: &issue, Comments: comments})
}

func TestRepliesOpenTheCommentDraftOnACommentOrReplyRow(t *testing.T) {
	for _, tc := range []struct {
		name         string
		moves        []string
		uid, comment string
	}{
		{"comment", nil, "c1", "dana"},
		{"reply", []string{"j"}, "c2", "eli"},
	} {
		m, _ := clickPress(onComments(), tc.moves...)
		m, _ = clickPress(m, "shift+r")
		d, ok := m.ActiveDraft()
		if !ok || !d.Comment || d.ReplyTo != tc.uid || d.ReplyAuthor != tc.comment || d.ReplyKind != model.ReplyPlain || m.EditorMode() != model.ModeEditor {
			t.Errorf("%s: Draft %+v ok=%v", tc.name, d, ok)
		}
	}
}

func TestRepliesKeepTextAlreadyInTheDraft(t *testing.T) {
	m, _ := clickPress(onComments(), "c")
	m = typed(m, "half a thought")
	// The Draft outlives its card (a Draft is kept until saved or discarded), so R meets it with text in it.
	m.EditorUID = ""
	m, _ = clickPress(m, "shift+r")
	if d, _ := m.ActiveDraft(); d.Editor.Text != "half a thought" || d.ReplyTo != "c1" {
		t.Fatalf("Draft %+v", d)
	}
}

func TestRepliesDoNothingOnARowThatIsNotAComment(t *testing.T) {
	m := onComments()
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabEvents})
	before := m
	m, _ = clickPress(m, "shift+r")
	if !reflect.DeepEqual(m.Drafts, before.Drafts) || m.EditorUID != "" {
		t.Fatalf("an Events row opened a Draft: %+v", m.Drafts)
	}
	empty := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a", Focus: model.ColumnDetail}, &model.IssueDetailData{Issue: &model.Issue{UID: "a"}})
	if empty, _ = clickPress(empty, "shift+r"); empty.EditorUID != "" {
		t.Fatal("R with no rows opened a Draft")
	}
}

func TestOneKeyCyclesTheReplyKind(t *testing.T) {
	m, _ := clickPress(onComments(), "shift+r")
	var kinds []model.ReplyKind
	for range 5 {
		m, _ = clickPress(m, "ctrl+r")
		d, _ := m.ActiveDraft()
		kinds = append(kinds, d.ReplyKind)
	}
	if want := []model.ReplyKind{model.ReplyConfirm, model.ReplyRefute, model.ReplySupersede, model.ReplyPlain, model.ReplyConfirm}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	// The headless Tern delivers Ctrl+r as meta+r.
	m, _ = clickPress(m, "meta+r")
	if d, _ := m.ActiveDraft(); d.ReplyKind != model.ReplyRefute {
		t.Fatalf("meta+r kind %q", d.ReplyKind)
	}
}

func TestConfirmAndRefuteUnder40CharactersAreRefusedOnSave(t *testing.T) {
	m, _ := clickPress(onComments(), "shift+r", "ctrl+r")
	m = typed(m, "  too   short\n\nto  count  ")
	m, cmds := clickPress(m, "ctrl+o")
	d, _ := m.ActiveDraft()
	if len(cmds) != 0 || d.Saving || d.Err == nil || !strings.Contains(d.Err.Error(), "40") {
		t.Fatalf("short confirm: cmds %v Draft %+v", cmds, d)
	}
	m, _ = clickPress(m, "ctrl+r")
	if m, cmds = clickPress(m, "ctrl+o"); len(cmds) != 0 {
		t.Fatalf("short refute sent %v", cmds)
	}
	m, _ = clickPress(m, "ctrl+r")
	if _, cmds = clickPress(m, "ctrl+o"); len(cmds) != 1 {
		t.Fatalf("a supersede of any length is sent, got %v", cmds)
	}
}

func TestAReplySavesWithItsTargetAndKind(t *testing.T) {
	m, _ := clickPress(onComments(), "shift+r", "ctrl+r")
	text := strings.Repeat("it reproduces. ", 3)
	m = typed(m, text)
	_, cmds := clickPress(m, "ctrl+o")
	want := []model.Cmd{model.SaveDraft{Workspace: "/repo", UID: "a", ProjectID: 2, BaseText: "body", Text: text, Comment: true, Actor: "me", ReplyTo: "c1", ReplyKind: model.ReplyConfirm, Key: "-1"}}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
	}
}

func TestADuplicateReplyOffersSendAnywayWhichResendsWithForce(t *testing.T) {
	m, _ := clickPress(onComments(), "shift+r")
	m = typed(m, "again")
	m, _ = clickPress(m, "ctrl+o")
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: model.DuplicateReply{Message: "a reply of this kind already exists"}})
	d, _ := m.ActiveDraft()
	if d.Saving || !d.Duplicate() || d.Err == nil || d.Editor.Text != "again" {
		t.Fatalf("Draft after duplicate: %+v", d)
	}
	_, cmds := clickPress(m, "ctrl+f")
	want := []model.Cmd{model.SaveDraft{Workspace: "/repo", UID: "a", ProjectID: 2, BaseText: "body", Text: "again", Comment: true, Actor: "me", ReplyTo: "c1", ReplyKind: model.ReplyPlain, Force: true, Key: "-2"}}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("send anyway %#v, want %#v", cmds, want)
	}
	// Anything else that changes the Draft takes the offer back.
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: model.DuplicateReply{Message: "dup"}})
	m, _ = clickPress(m, "ctrl+r")
	if _, cmds = clickPress(m, "ctrl+f"); len(cmds) != 0 {
		t.Fatalf("send anyway offered after the kind changed: %v", cmds)
	}
}
