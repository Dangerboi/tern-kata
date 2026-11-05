package model_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func attachPane(focus model.Column, metadata map[string]any) model.Model {
	return model.Model{
		Actor: "reviewer", Workspace: "/ws", Focus: focus, SelectedUID: "issue", Bound: true,
		Project: model.Project{ID: 2, UID: "p", Name: "work"},
		Issues:  []model.Issue{{UID: "issue", ShortID: "abcd", ProjectID: 7, ProjectName: "other", Metadata: metadata}},
	}
}

// refID is the id refs gives the Attachment called name.
func refID(name string) string { return "ID-" + strings.ReplaceAll(name, ".", "_") }

// refs is Issue metadata holding one native Attachment reference per name, with ids from refID.
func refs(names ...string) map[string]any {
	meta := map[string]any{"unrelated": "x"}
	for _, n := range names {
		meta[model.AttachmentKey(refID(n))] = map[string]any{"name": n, "media_type": "text/plain", "size": float64(3), "docbank_version": "6f1c2e8a-2222-4222-8222-000000000001"}
	}
	return meta
}

func TestAtAttachesTheTypedPathToTheIssueInItsOwnProject(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnDetail} {
		m, _ := model.Update(attachPane(focus, nil), model.KeyMsg{Chord: "@"})
		if !m.FieldForm.Open || m.FieldForm.Action != model.AttachFileAction || m.FieldForm.Err != nil {
			t.Fatalf("focus %v: @ opened %+v", focus, m.FieldForm)
		}
		m, _ = model.Update(m, model.FieldValueMsg{Value: ` "~/My shot.png" `})
		m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
		want := model.AttachFile{Workspace: "/ws", UID: "issue", ProjectID: 7, Actor: "reviewer", Path: `"~/My shot.png"`}
		if len(cmds) != 1 || cmds[0] != model.Cmd(want) {
			t.Fatalf("focus %v: cmds %#v, want %#v", focus, cmds, want)
		}
		m, _ = model.Update(m, model.FileAttached{UID: "issue", Err: errors.New("/x does not exist.")})
		if !m.FieldForm.Open || m.FieldForm.Err == nil || m.FieldForm.Err.Error() != "/x does not exist." || m.Notice != "" {
			t.Fatalf("failure not shown in the form: %+v, notice %q", m.FieldForm, m.Notice)
		}
		m, _ = model.Update(m, model.FileAttached{UID: "issue", ID: refID("shot.png"), Name: "shot.png"})
		if m.FieldForm.Open || m.Notice != "Attached shot.png to abcd" {
			t.Fatalf("form %+v, notice %q", m.FieldForm, m.Notice)
		}
	}
}

func TestAtOnAnEmptyPathAsksForOne(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, nil), model.KeyMsg{Chord: "@"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 0 || m.FieldForm.Err == nil {
		t.Fatalf("empty path sent %#v, err %v", cmds, m.FieldForm.Err)
	}
}

func TestAtSaysPlainlyWhenTheDaemonHasNoDocbank(t *testing.T) {
	m := withDetail(attachPane(model.ColumnDetail, nil), &model.IssueDetailData{Issue: &model.Issue{UID: "issue"}, AttachmentsOff: true})
	m, _ = model.Update(m, model.KeyMsg{Chord: "@"})
	if !m.FieldForm.Open || m.FieldForm.Err == nil || m.FieldForm.Err.Error() != model.DocbankOffMessage {
		t.Fatalf("@ opened %+v", m.FieldForm)
	}
}

func TestShiftUListsAttachmentNamesAndUnlinksTheChosenOne(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnDetail, refs("b.log", "a.png")), model.KeyMsg{Chord: "shift+u"})
	if !m.FieldForm.Open || m.FieldForm.Action != model.DetachFileAction {
		t.Fatalf("U opened %+v", m.FieldForm)
	}
	if got := strings.Join(m.FieldSuggestions(), ","); got != "a.png,b.log" {
		t.Fatalf("names = %q", got)
	}
	// Choosing the second row sends it without a confirmation step.
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	want := model.UnlinkAttachment{Workspace: "/ws", UID: "issue", ProjectID: 7, Actor: "reviewer", ID: refID("b.log"), Name: "b.log"}
	if len(cmds) != 1 || cmds[0] != model.Cmd(want) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
	}
}

func TestUnlinkSendsTheAttachmentPickedWhenTheFormOpenedEvenIfTheListChangesMeanwhile(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnDetail, refs("b.log", "c.log")), model.KeyMsg{Chord: "shift+u"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"}) // c.log
	// A reload adds an Attachment that sorts before it, which would shift an index into the new list.
	m.Issues[0].Metadata = refs("a.log", "b.log", "c.log")
	_, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 1 || cmds[0].(model.UnlinkAttachment).ID != refID("c.log") {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestShiftUTellsTwoAttachmentsOfOneNameApart(t *testing.T) {
	meta := refs("a.png")
	meta[model.AttachmentKey("OTHER-ID-123456")] = map[string]any{"name": "a.png", "media_type": "image/png", "size": float64(1), "docbank_version": "v"}
	m, _ := model.Update(attachPane(model.ColumnList, meta), model.KeyMsg{Chord: "shift+u"})
	got := m.FieldSuggestions()
	if len(got) != 2 || got[0] == got[1] || !strings.HasPrefix(got[0], "a.png [") || !strings.HasSuffix(got[1], "]") {
		t.Fatalf("labels %q", got)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"})
	_, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 1 || cmds[0].(model.UnlinkAttachment).ID != "OTHER-ID-123456" {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestShiftUNamesAnAttachmentTypedInFull(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, refs("a.png", "a.png.bak")), model.KeyMsg{Chord: "shift+u"})
	m, _ = model.Update(m, model.FieldValueMsg{Value: "a.png"})
	_, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 1 || cmds[0].(model.UnlinkAttachment).Name != "a.png" {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestShiftUWithNoAttachmentsSaysSoWithoutOpeningAForm(t *testing.T) {
	m, cmds := model.Update(attachPane(model.ColumnList, map[string]any{"other": 1, "tern-kata.attachment.old": map[string]any{"host": "h", "path": "/p"}}), model.KeyMsg{Chord: "shift+u"})
	if m.FieldForm.Open || slices.ContainsFunc(cmds, func(c model.Cmd) bool { _, ok := c.(model.UnlinkAttachment); return ok }) || m.Notice != "abcd has no attachments." {
		t.Fatalf("U with none: form %+v notice %q", m.FieldForm, m.Notice)
	}
}

func TestSubmitTargetsTheFormsIssueWhenTheCursorMoves(t *testing.T) {
	m := attachPane(model.ColumnList, refs("a.png"))
	m.Issues = append([]model.Issue{{UID: "newer", ShortID: "zzzz", ProjectID: 2}}, m.Issues...)
	m, _ = model.Update(m, model.KeyMsg{Chord: "@"})
	m, _ = model.Update(m, model.FieldValueMsg{Value: "/x"})
	// A live refresh moves the cursor to another Issue while the form is open.
	m.SelectedUID = "newer"
	_, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	want := model.AttachFile{Workspace: "/ws", UID: "issue", ProjectID: 7, Actor: "reviewer", Path: "/x"}
	if !slices.Contains(cmds, model.Cmd(want)) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
	}
}

func TestSubmitWhenTheFormsIssueIsGoneShowsAnError(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, nil), model.KeyMsg{Chord: "@"})
	m, _ = model.Update(m, model.FieldValueMsg{Value: "/x"})
	m.Issues = nil
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 0 || m.FieldForm.Err == nil {
		t.Fatalf("cmds %#v, err %v", cmds, m.FieldForm.Err)
	}
}

func TestUnlinkSaysTheBytesStayAndShowsAsANoticeUntilTheNextKey(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, refs("a.png")), model.KeyMsg{Chord: "shift+u"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	m, _ = model.Update(m, model.AttachmentUnlinked{UID: "issue", Name: "a.png"})
	if m.FieldForm.Open || m.Notice != "Unlinked a.png from abcd; its bytes stay in Docbank" {
		t.Fatalf("form %+v notice %q", m.FieldForm, m.Notice)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"})
	if m.Notice != "" {
		t.Fatalf("notice outlived a key: %q", m.Notice)
	}
}

func TestAFailedUnlinkKeepsItsErrorInTheForm(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, refs("a.png")), model.KeyMsg{Chord: "shift+u"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	m, _ = model.Update(m, model.AttachmentUnlinked{UID: "issue", Name: "a.png", Err: errors.New("daemon down")})
	if !m.FieldForm.Open || m.FieldForm.Err == nil || m.Notice != "" {
		t.Fatalf("form %+v notice %q", m.FieldForm, m.Notice)
	}
}

func TestAttachKeysAreInTheFooterAndHelpSheet(t *testing.T) {
	for _, chord := range []string{"@", "shift+u"} {
		b, ok := model.LookupBindingForMode(chord, model.ModeDetail)
		if !ok || !b.ShowInFooter {
			t.Fatalf("%s not in the detail footer: %+v", chord, b)
		}
	}
	var lines []string
	for _, g := range model.HelpSheet() {
		for _, l := range g.Lines {
			lines = append(lines, l.Text+"|"+strings.Join(l.Keys, ","))
		}
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{"attach file (upload to Docbank)|@", "unlink attachment|U"} {
		if !strings.Contains(all, want) {
			t.Fatalf("help sheet lacks %q:\n%s", want, all)
		}
	}
}
