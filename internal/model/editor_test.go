package model_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func editorModel() model.Model {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "a😀b", Revision: 7}
	return withDetail(model.Model{Workspace: "/repo", SelectedUID: "a", Focus: model.ColumnDetail}, &model.IssueDetailData{Issue: &issue})
}

func TestDraftNativeEditAndSave(t *testing.T) {
	m := editorModel()
	for _, msg := range []model.Msg{model.KeyMsg{Chord: "e"}, model.EditMsg{Target: model.EditDraftBody, From: 1, To: 3, Text: "new", Cursor: 4, Len: 4}} {
		m, _ = model.Update(m, msg)
	}
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	want := []model.Cmd{model.SaveDraft{Workspace: "/repo", UID: "a", ProjectID: 2, BaseRevision: 7, BaseText: "a😀b", Text: "anewb"}}
	if m.Drafts["a"].Editor.Text != "anewb" || !m.Drafts["a"].Saving || !reflect.DeepEqual(cmds, want) {
		t.Fatalf("Draft/save: %+v %#v", m.Drafts, cmds)
	}
}

func TestDraftSequences(t *testing.T) {
	latest := model.Issue{UID: "a", ProjectID: 2, Body: "Remote body", Revision: 8}
	key := func(chord string) model.Msg { return model.KeyMsg{Chord: chord} }
	typed := model.KeyMsg{Chord: "paste", Text: "local"}
	conflict := model.DraftConflict{UID: "a", Issue: latest}
	for _, tt := range []struct {
		name     string
		messages []model.Msg
		text     string
		mode     model.Mode
		present  bool
		command  model.Cmd
	}{
		{"typed undo", []model.Msg{key("e"), typed, key("ctrl+z")}, "a😀b", model.ModeEditor, true, nil},
		{"global printable keys become text", []model.Msg{key("e"), model.KeyMsg{Chord: "q", Text: "q"}, model.KeyMsg{Chord: "shift+?", Text: "?"}}, "a😀bq?", model.ModeEditor, true, nil},
		{"native undo", []model.Msg{key("e"), model.EditMsg{Target: model.EditDraftBody, From: 1, To: 3, Text: "x", Cursor: 2, Len: 4}, model.UndoMsg{Target: model.EditDraftBody}}, "a😀b", model.ModeEditor, true, nil},
		{"stale edit", []model.Msg{key("e"), model.EditMsg{Target: model.EditDraftBody, From: 0, To: 1, Text: "x", Cursor: 1, Len: 99}}, "a😀b", model.ModeEditor, true, nil},
		{"surrogate split", []model.Msg{key("e"), model.EditMsg{Target: model.EditDraftBody, From: 2, To: 3, Text: "x", Cursor: 3, Len: 4}}, "a😀b", model.ModeEditor, true, nil},
		{"ordinary backspace", []model.Msg{key("e"), key("left"), key("backspace")}, "ab", model.ModeEditor, true, nil},
		{"selection replacement", []model.Msg{key("e"), key("shift+left"), typed}, "a😀local", model.ModeEditor, true, nil},
		{"discard asks", []model.Msg{key("e"), key("escape")}, "a😀b", model.ModeDiscard, true, nil},
		{"discard cancel", []model.Msg{key("e"), key("escape"), key("n")}, "a😀b", model.ModeEditor, true, nil},
		{"discard confirmed", []model.Msg{key("e"), key("escape"), key("y")}, "", model.ModeDetail, false, nil},
		{"empty comment escape", []model.Msg{key("c"), key("escape")}, "", model.ModeDetail, false, nil},
		{"conflict keeps both", []model.Msg{key("e"), typed, key("ctrl+o"), conflict}, "a😀blocal", model.ModeConflict, true, nil},
		{"conflict overwrite", []model.Msg{key("e"), typed, conflict, key("o")}, "a😀blocal", model.ModeEditor, true, model.SaveDraft{Workspace: "/repo", UID: "a", ProjectID: 2, BaseRevision: 8, BaseText: "Remote body", Text: "a😀blocal"}},
		{"conflict discard", []model.Msg{key("e"), typed, conflict, key("d")}, "", model.ModeDetail, false, nil},
		{"live detail preserves Draft", []model.Msg{key("e"), typed, model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &latest}}}, "a😀blocal", model.ModeEditor, true, nil},
		{"comment save", []model.Msg{key("c"), typed, key("meta+o")}, "local", model.ModeEditor, true, model.SaveDraft{Workspace: "/repo", UID: "a", ProjectID: 2, BaseRevision: 7, BaseText: "a😀b", Text: "local", Comment: true}},
		{"external editor", []model.Msg{key("e"), key("ctrl+e")}, "a😀b", model.ModeEditor, true, model.OpenDraftEditor{UID: "a", Text: "a😀b"}},
		{"CRLF paste", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\r\ny\rz"}}, "a😀bx\ny\nz", model.ModeEditor, true, nil},
		{"plain paste unchanged", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\ny"}}, "a😀bx\ny", model.ModeEditor, true, nil},
		{"CRLF paste then Home", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\r\ny"}, key("home"), model.KeyMsg{Text: "Z"}}, "a😀bx\nZy", model.ModeEditor, true, nil},
		{"CRLF paste then Up", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\r\ny"}, key("up"), model.KeyMsg{Text: "Z"}}, "aZ😀bx\ny", model.ModeEditor, true, nil},
		{"CRLF paste then Up End", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\r\ny"}, key("up"), key("end"), model.KeyMsg{Text: "Z"}}, "a😀bxZ\ny", model.ModeEditor, true, nil},
		{"CRLF paste then Up Down", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\r\ny"}, key("up"), key("down"), model.KeyMsg{Text: "Z"}}, "a😀bx\nyZ", model.ModeEditor, true, nil},
		{"CRLF paste undo is one step", []model.Msg{key("e"), model.KeyMsg{Chord: "paste", Text: "x\r\ny\rz"}, key("ctrl+z")}, "a😀b", model.ModeEditor, true, nil},
		{"external CRLF", []model.Msg{key("e"), model.DraftEditorClosed{UID: "a", Text: "one\r\ntwo\rthree"}}, "one\ntwo\nthree", model.ModeEditor, true, nil},
		{"external edit undo", []model.Msg{key("e"), model.DraftEditorClosed{UID: "a", Text: "external"}, key("ctrl+z")}, "a😀b", model.ModeEditor, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := editorModel()
			var cmds []model.Cmd
			for _, msg := range tt.messages {
				m, cmds = model.Update(m, msg)
			}
			draft, present := m.Drafts["a"]
			if present != tt.present || draft.Editor.Text != tt.text || m.CurrentMode() != tt.mode {
				t.Fatalf("Draft=%+v mode=%s", draft, m.CurrentMode())
			}
			var want []model.Cmd
			if tt.command != nil {
				want = []model.Cmd{tt.command}
			}
			if !reflect.DeepEqual(cmds, want) {
				t.Fatalf("Cmds=%#v want %#v", cmds, want)
			}
			if draft.Conflict != nil && draft.Conflict.Body != "Remote body" {
				t.Fatal("remote version lost")
			}
			if present && draft.BaseRevision != 7 && tt.name != "conflict overwrite" {
				t.Fatal("live update rebased Draft")
			}
			if tt.name == "conflict discard" && m.DetailAuthority().Issue.Body != "Remote body" {
				t.Fatal("discard did not retain remote version")
			}
		})
	}
}

func TestDraftSurvivesLiveSnapshotAndSaveFailure(t *testing.T) {
	m := editorModel()
	m.Collection = model.Collection{View: "project"}
	m.Actor = "writer"
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	previous := m
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "local"})
	if previous.Drafts["a"].Editor.Text != "a😀b" {
		t.Fatal("Update mutated the previous Draft")
	}
	m, cmds := model.Update(m, model.StreamEvent{ID: 9})
	read, ok := cmds[0].(model.ReadSnapshot)
	if !ok {
		t.Fatalf("a daemon event asked for %#v, want a snapshot read", cmds)
	}
	latest := model.Issue{UID: "a", ProjectID: 2, Body: "Remote body", Revision: 8}
	snapshot := g.UISnapshotResponseBody{Cursor: 9, Collection: []g.UIIssue{latest}}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: snapshot, Selected: &model.IssueDetailData{Issue: &latest}})
	if m.Drafts["a"].Editor.Text != "a😀blocal" || m.Drafts["a"].BaseRevision != 7 {
		t.Fatal("snapshot destroyed/rebased Draft")
	}
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if len(cmds) != 1 || cmds[0].(model.SaveDraft).BaseRevision != 7 || cmds[0].(model.SaveDraft).Actor != "writer" {
		t.Fatalf("save: %#v", cmds)
	}
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: fmt.Errorf("write failed")})
	if m.Drafts["a"].Editor.Text != "a😀blocal" || m.Drafts["a"].Saving || m.Drafts["a"].Err == nil {
		t.Fatal("save failure lost Draft")
	}
	m.Stale = true
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if len(cmds) != 0 || m.Drafts["a"].Err == nil {
		t.Fatal("disconnected Draft wrote")
	}
}

func TestDraftOpensTheFullStoredBodyIncludingParentSection(t *testing.T) {
	body := "## Parent\n\nup1\n\n## What\n\nbuild"
	issue := model.Issue{UID: "a", ProjectID: 2, Body: body, Revision: 7}
	m := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a", Focus: model.ColumnDetail}, &model.IssueDetailData{Issue: &issue})
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	if got := m.Drafts["a"].Editor.Text; got != body {
		t.Errorf("draft text %q, want %q", got, body)
	}
}
