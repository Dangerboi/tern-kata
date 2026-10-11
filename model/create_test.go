package model_test

import (
	"errors"
	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
	"testing"
	"time"
)

func TestCreateProjectRule(t *testing.T) {
	for _, tc := range []struct {
		name, key, view string
		selected        bool
		want            int64
		parent          string
	}{
		{"Project", "n", "project", true, 7, ""},
		{"Collection selected", "n", "all-open", true, 9, ""},
		{"Collection bound", "n", "all-open", false, 7, ""},
		{"child", "shift+n", "project", true, 9, "existing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := model.Model{Project: model.Project{ID: 7, UID: "bound"}, Collection: model.Collection{View: tc.view, ProjectUID: "bound"}, Issues: []model.Issue{{UID: "existing", ProjectID: 9}}}
			if tc.selected {
				m.SelectedUID = "existing"
			}
			m, _ = model.Update(m, model.KeyMsg{Chord: tc.key})
			if !m.Create.Open || m.Create.ProjectID != tc.want || m.Create.ParentUID != tc.parent {
				t.Fatalf("create: %#v", m.Create)
			}
		})
	}
}

func TestCreateDraft(t *testing.T) {
	m := model.Model{Workspace: "workspace", Actor: "actor", Project: model.Project{ID: 7}, SelectedUID: "parent", Issues: []model.Issue{{UID: "parent", ProjectID: 7}}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+n"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if m.Create.Err == nil {
		t.Fatal("empty title accepted")
	}
	for _, cmd := range cmds {
		if _, ok := cmd.(model.CreateIssueCmd); ok {
			t.Fatal("invalid create requested")
		}
	}
	for _, msg := range []model.Msg{
		model.KeyMsg{Text: " New Issue "}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Text: "Draft body"},
		model.KeyMsg{Chord: "tab"}, model.FieldLabelsLoaded{UID: model.CreateDraftUID, Labels: []string{"bug", "other"}},
		model.KeyMsg{Text: "bu"}, model.KeyMsg{Chord: "ctrl+space"}, model.KeyMsg{Text: ", custom"},
		model.KeyMsg{Chord: "tab"}, model.KeyMsg{Text: "owner"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Text: "2"},
	} {
		m, _ = model.Update(m, msg)
	}
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	found := false
	for _, cmd := range cmds {
		if c, ok := cmd.(model.CreateIssueCmd); ok {
			found = true
			if c.ProjectID != 7 || c.ParentUID != "parent" || c.Title != "New Issue" || c.Body != "Draft body" || c.Owner != "owner" || c.Actor != "actor" || c.Workspace != "workspace" || c.Priority == nil || *c.Priority != 2 || len(c.Labels) != 2 || c.Labels[0] != "bug" || c.Labels[1] != "custom" {
				t.Fatalf("command %#v", c)
			}
		}
	}
	if !found {
		t.Fatal("create missing")
	}
	rejected := errors.New("permission denied\nask owner")
	m, _ = model.Update(m, model.IssueCreated{Err: rejected})
	if !m.Create.Open || m.Create.Saving || m.Create.Err != rejected || m.Create.Editor.Text != "Draft body" {
		t.Fatal("rejected Draft lost")
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	m, _ = model.Update(m, model.IssueCreated{UID: "created"})
	if m.Create.Open || m.SelectedUID != "created" || !m.ExpandedUIDs["parent"] {
		t.Fatal("new Issue not selected")
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "parent", ProjectID: 7}}}, ETag: "before"})
	if m.SelectedUID != "created" {
		t.Fatal("early refresh lost created selection")
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "parent", ProjectID: 7}, {UID: "created", ProjectID: 7}}}, ETag: "after"})
	if m.SelectedUID != "created" {
		t.Fatal("arrival lost created selection")
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "parent", ProjectID: 7}, {UID: "created", ProjectID: 7}}}, ETag: "later"})
	if m.SelectedUID != "created" {
		t.Fatal("later refresh lost created selection")
	}
}

func TestCreateDiscardAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		messages      []model.Msg
		open, confirm bool
	}{
		{"empty", []model.Msg{model.KeyMsg{Chord: "escape"}}, false, false},
		{"nonempty", []model.Msg{model.KeyMsg{Text: "title"}, model.KeyMsg{Chord: "escape"}}, true, true},
		{"keep", []model.Msg{model.KeyMsg{Text: "title"}, model.KeyMsg{Chord: "escape"}, model.KeyMsg{Chord: "n"}}, true, false},
		{"discard", []model.Msg{model.KeyMsg{Text: "title"}, model.KeyMsg{Chord: "escape"}, model.KeyMsg{Chord: "y"}}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := model.Model{Project: model.Project{ID: 7}}
			m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
			for _, msg := range tc.messages {
				m, _ = model.Update(m, msg)
			}
			if m.Create.Open != tc.open || m.Create.DiscardConfirm != tc.confirm {
				t.Fatalf("Draft %#v", m.Create)
			}
		})
	}
	for _, priority := range []string{"-1", "5", "bad"} {
		m := model.Model{Project: model.Project{ID: 7}}
		m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
		m, _ = model.Update(m, model.KeyMsg{Text: "title"})
		m, _ = model.Update(m, model.FocusFieldMsg{Field: 4})
		m, _ = model.Update(m, model.KeyMsg{Text: priority})
		m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
		if m.Create.Err == nil {
			t.Fatal("priority accepted")
		}
		for _, cmd := range cmds {
			if _, ok := cmd.(model.CreateIssueCmd); ok {
				t.Fatal("invalid priority wrote")
			}
		}
	}
}

func TestNavigationReleasesExcludedCreatedIssue(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  model.Msg
	}{
		{"keyboard Issue", model.KeyMsg{Chord: "j"}},
		{"pointer Issue", model.SelectMsg{UID: "second"}},
		{"pointer Collection", model.CollectionSelected{Collection: model.Collection{View: "all-open", Title: "All Open"}}},
		{"reference jump", model.JumpIssueMsg{UID: "second"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			closed := new(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
			m := model.Model{Project: model.Project{ID: 7}, Collection: model.Collection{View: "logbook", Title: "Logbook"}, SelectedUID: "first"}
			m = withDetail(m, nil)
			m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "first", Status: "closed", ClosedAt: closed}, {UID: "second", Status: "closed", ClosedAt: closed}}}})
			m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
			m, _ = model.Update(m, model.KeyMsg{Text: "Created outside Logbook"})
			m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
			m, _ = model.Update(m, model.IssueCreated{UID: "created"})
			m, _ = model.Update(m, tt.msg)
			selected := m.SelectedUID
			m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "first", Status: "closed", ClosedAt: closed}, {UID: "second", Status: "closed", ClosedAt: closed}}}})
			if m.SelectedUID == "created" || (selected == "second" && m.SelectedUID != "second") {
				t.Fatalf("live refresh overrode explicit selection: %q -> %q", selected, m.SelectedUID)
			}
		})
	}
}
