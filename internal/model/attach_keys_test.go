package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func attachPane(focus model.Column, metadata map[string]any, projectID int64) model.Model {
	return model.Model{
		Actor: "reviewer", Workspace: "/ws", Focus: focus, SelectedUID: "issue", Bound: true,
		Project: model.Project{ID: 2, UID: "p", Name: "work"},
		Issues:  []model.Issue{{UID: "issue", ShortID: "abcd", ProjectID: projectID, ProjectName: "other", Metadata: metadata}},
	}
}

func pointers(names ...string) map[string]any {
	meta := map[string]any{"unrelated": "x"}
	for _, n := range names {
		meta[model.AttachmentKeyPrefix+n] = map[string]any{"host": "box", "path": "/store/issue/" + n}
	}
	return meta
}

func TestUAttachesTheTypedPath(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnDetail} {
		m, _ := model.Update(attachPane(focus, nil, 2), model.KeyMsg{Chord: "u"})
		if !m.FieldForm.Open || m.FieldForm.Action != model.AttachFileAction || m.FieldForm.Err != nil {
			t.Fatalf("focus %v: u opened %+v", focus, m.FieldForm)
		}
		m, _ = model.Update(m, model.FieldValueMsg{Value: ` "~/My shot.png" `})
		m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
		want := model.AttachFile{Workspace: "/ws", Ref: "issue", Actor: "reviewer", Path: `"~/My shot.png"`}
		if len(cmds) != 1 || cmds[0] != model.Cmd(want) {
			t.Fatalf("focus %v: cmds %#v, want %#v", focus, cmds, want)
		}
		m, _ = model.Update(m, model.FileAttached{Ref: "issue", Err: errors.New("/x does not exist.")})
		if !m.FieldForm.Open || m.FieldForm.Err == nil || m.FieldForm.Err.Error() != "/x does not exist." {
			t.Fatalf("failure not shown in the form: %+v", m.FieldForm)
		}
		m, _ = model.Update(m, model.FileAttached{Ref: "issue", Name: "shot.png", Path: "/store/issue/shot.png"})
		if m.FieldForm.Open {
			t.Fatal("form still open after attaching")
		}
	}
}

func TestUOnAnEmptyPathAsksForOne(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, nil, 2), model.KeyMsg{Chord: "u"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 0 || m.FieldForm.Err == nil {
		t.Fatalf("empty path sent %#v, err %v", cmds, m.FieldForm.Err)
	}
}

func TestShiftUListsAttachmentNamesAndDetachesTheChosenOne(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnDetail, pointers("b.log", "a.png"), 2), model.KeyMsg{Chord: "shift+u"})
	if !m.FieldForm.Open || m.FieldForm.Action != model.DetachFileAction {
		t.Fatalf("U opened %+v", m.FieldForm)
	}
	if got := strings.Join(m.FieldSuggestions(), ","); got != "a.png,b.log" {
		t.Fatalf("names = %q", got)
	}
	// Choosing the second row sends it without a confirmation step.
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	want := model.DetachFile{Workspace: "/ws", Ref: "issue", Actor: "reviewer", Name: "b.log"}
	if len(cmds) != 1 || cmds[0] != model.Cmd(want) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
	}
}

func TestShiftUNamesAnAttachmentTypedInFull(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, pointers("a.png", "a.png.bak"), 2), model.KeyMsg{Chord: "shift+u"})
	m, _ = model.Update(m, model.FieldValueMsg{Value: "a.png"})
	_, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 1 || cmds[0].(model.DetachFile).Name != "a.png" {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestShiftUWithNoAttachmentsSaysSo(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, map[string]any{"other": 1}, 2), model.KeyMsg{Chord: "shift+u"})
	if !m.FieldForm.Open || m.FieldForm.Err == nil || !strings.Contains(m.FieldForm.Err.Error(), "no attachments") {
		t.Fatalf("U with none: %+v", m.FieldForm)
	}
}

func TestDetachReportShowsAsANoticeUntilTheNextKey(t *testing.T) {
	m, _ := model.Update(attachPane(model.ColumnList, pointers("a.png"), 2), model.KeyMsg{Chord: "shift+u"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	done := model.FileDetached{Ref: "issue", Name: "a.png", Host: "other-box", Path: "/s/a.png", Fate: model.FileLeft}
	m, _ = model.Update(m, done)
	if m.FieldForm.Open || m.Notice != done.Report() || !strings.Contains(m.Notice, "other-box") {
		t.Fatalf("form %+v notice %q", m.FieldForm, m.Notice)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"})
	if m.Notice != "" {
		t.Fatalf("notice outlived a key: %q", m.Notice)
	}
}

func TestAttachKeysExplainARefusal(t *testing.T) {
	unbound := attachPane(model.ColumnList, pointers("a.png"), 2)
	unbound.Bound, unbound.Project = false, model.Project{}
	for name, pane := range map[string]model.Model{
		"unbound":       unbound,
		"other Project": attachPane(model.ColumnList, pointers("a.png"), 3),
	} {
		for _, chord := range []string{"u", "shift+u"} {
			m, cmds := model.Update(pane, model.KeyMsg{Chord: chord})
			if !m.FieldForm.Open || m.FieldForm.Err == nil {
				t.Fatalf("%s %s: no reason shown: %+v", name, chord, m.FieldForm)
			}
			want := model.AttachRefusal(pane.Project, 3, "abcd", "other")
			if name == "unbound" {
				want = model.AttachRefusal(pane.Project, 2, "abcd", "other")
			}
			if m.FieldForm.Err.Error() != want.Error() || !strings.Contains(want.Error(), `"other"`) {
				t.Fatalf("%s %s: reason %q", name, chord, m.FieldForm.Err)
			}
			m, cmds = model.Update(m, model.FieldValueMsg{Value: "a.png"})
			if _, cmds = model.Update(m, model.KeyMsg{Chord: "enter"}); len(cmds) != 0 {
				t.Fatalf("%s %s: refused key still sent %#v", name, chord, cmds)
			}
		}
	}
}

func TestAttachKeysAreInTheFooterAndHelpSheet(t *testing.T) {
	for _, chord := range []string{"u", "shift+u"} {
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
	for _, want := range []string{"attach file|u", "remove attachment|U"} {
		if !strings.Contains(all, want) {
			t.Fatalf("help sheet lacks %q:\n%s", want, all)
		}
	}
}
