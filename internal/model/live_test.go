package model_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestLiveMergePreservesIssueSelection(t *testing.T) {
	m, cmds := loadFirst(model.Model{Workspace: "/repo", SelectedUID: "b"}, model.ProjectLoaded{Project: model.Project{UID: "project"}}, model.SnapshotLoaded{Snapshot: model.Snapshot{Cursor: 10, Issues: []model.Issue{{UID: "a"}, {UID: "b", Title: "Before"}}}})
	if !reflect.DeepEqual(cmds, []model.Cmd{model.Subscribe{Workspace: "/repo", AfterID: 10, Generation: 1}, model.LoadDetail{Workspace: "/repo", UID: "b"}}) {
		t.Fatalf("initial subscription: %#v", cmds)
	}
	m, cmds = model.Update(m, model.StreamEvent{Generation: 1, ID: 11, Type: "issue.updated"})
	if len(cmds) != 1 {
		t.Fatalf("live refresh: %#v", cmds)
	}
	refresh := cmds[0].(model.ReadSnapshot)
	m, _ = model.Update(m, model.SnapshotLoaded{
		Request: refresh.Request, Query: refresh.Query,
		Snapshot: model.Snapshot{Cursor: 11, Issues: []model.Issue{{UID: "new"}, {UID: "b", Title: "After"}, {UID: "a"}}},
	})
	selected := slices.IndexFunc(m.Issues, func(issue model.Issue) bool { return issue.UID == "b" })
	if m.SelectedUID != "b" || selected < 0 || m.Issues[selected].Title != "After" || m.Cursor != 11 {
		t.Fatalf("live Issue merge: %+v", m)
	}
}

func TestLiveFailureAndRecovery(t *testing.T) {
	failure := errors.New("kata server not responding: http://127.0.0.1:1 (KATA_SERVER)")
	for _, tt := range []struct {
		name     string
		messages []model.Msg
		check    func(*testing.T, model.Model, []model.Cmd)
	}{
		{
			name:     "unreachable preserves kata error and exposes retry mode",
			messages: []model.Msg{model.Failed{Err: failure}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				binding, ok := model.LookupBindingForMode("ctrl+r", m.CurrentMode())
				if m.Err != failure || m.Loading || m.CurrentMode() != model.ModeError || !ok || binding.Action != model.RetryAction || !binding.ShowInFooter || len(cmds) != 0 {
					t.Fatalf("daemon error: %+v, %#v", m, cmds)
				}
			},
		},
		{
			name:     "keyboard retry reloads unreachable daemon",
			messages: []model.Msg{model.Failed{Err: failure}, model.KeyMsg{Chord: "ctrl+r"}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				if m.Err != nil || !m.Loading || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadProject{Workspace: "/repo"}}) {
					t.Fatalf("keyboard retry: %+v, %#v", m, cmds)
				}
			},
		},
		{
			name:     "unreachable and retry",
			messages: []model.Msg{model.Failed{Err: failure}, model.Retry{}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				if m.Err != nil || !m.Loading || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadProject{Workspace: "/repo"}}) {
					t.Fatalf("retry: %+v, %#v", m, cmds)
				}
			},
		},
		{
			name:     "stream drop keeps Issues read-only and resumes cursor",
			messages: []model.Msg{model.StreamDropped{Generation: 1, Err: failure}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				want := model.Subscribe{Workspace: "/repo", AfterID: 10, Generation: 2, Delay: time.Second}
				if !m.Stale || m.Err != nil || m.StreamErr != failure || m.SelectedUID != "b" || len(m.Issues) != 2 || !reflect.DeepEqual(cmds, []model.Cmd{want}) {
					t.Fatalf("stale mode: %+v, %#v", m, cmds)
				}
			},
		},
		{
			name:     "reset requests unconditional full refetch",
			messages: []model.Msg{model.StreamEvent{Generation: 1, ID: 25, Type: "sync.reset_required"}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				refresh := cmds[0].(model.ReadSnapshot)
				if !m.Resetting || !m.Stale || refresh.ETag != "" || m.Cursor != 10 {
					t.Fatalf("history reset: %+v, %#v", m, cmds)
				}
				m, cmds = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query,
					Snapshot: model.Snapshot{Cursor: 30, Issues: []model.Issue{{UID: "b", Title: "Survivor"}}},
				})
				if m.Resetting || len(m.Issues) != 1 || m.SelectedUID != "b" || !reflect.DeepEqual(cmds, []model.Cmd{model.Subscribe{Workspace: "/repo", AfterID: 30, Generation: 2}, model.LoadDetail{Workspace: "/repo", UID: "b"}}) {
					t.Fatalf("reset replaced purged Issues and resumed: %+v, %#v", m, cmds)
				}
			},
		},
		{
			name:     "reconnect refreshes before allowing writes",
			messages: []model.Msg{model.StreamDropped{Generation: 1, Err: failure}, model.StreamOpened{Generation: 2}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				if !m.Stale || !m.StreamConnected {
					t.Fatalf("reconnect before refresh: %+v", m)
				}
				refresh := cmds[0].(model.ReadSnapshot)
				m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, NotModified: true})
				if m.Stale || m.StreamErr != nil || m.ReconnectAttempt != 0 || m.SelectedUID != "b" {
					t.Fatalf("reconnected: %+v", m)
				}
			},
		},
		{
			name:     "superseded stream cannot stale current data",
			messages: []model.Msg{model.StreamDropped{Generation: 0, Err: failure}},
			check: func(t *testing.T, m model.Model, cmds []model.Cmd) {
				if m.Stale || len(cmds) != 0 {
					t.Fatalf("late stream response: %+v, %#v", m, cmds)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := loadFirst(model.Model{Workspace: "/repo", SelectedUID: "b"}, model.ProjectLoaded{Project: model.Project{UID: "project"}}, model.SnapshotLoaded{ETag: "\"old\"", Snapshot: model.Snapshot{Cursor: 10, Issues: []model.Issue{{UID: "a"}, {UID: "b"}}}})
			var cmds []model.Cmd
			for _, msg := range tt.messages {
				m, cmds = model.Update(m, msg)
			}
			tt.check(t, m, cmds)
		})
	}
}

func TestLiveRefreshPreservesDetailPosition(t *testing.T) {
	for _, authority := range []bool{true, false} {
		t.Run(map[bool]string{true: "snapshot authority", false: "detail refetch"}[authority], func(t *testing.T) {
			m := withDetail(model.Model{LiveState: model.LiveState{StreamGeneration: 1}, SelectedUID: "issue", Focus: model.ColumnDetail},
				&model.IssueDetailData{Issue: &model.Issue{UID: "issue"}, Comments: []model.Comment{{UID: "one"}, {UID: "two"}}})
			m = cursorAt(m, 1)
			m, cmds := model.Update(m, model.StreamEvent{Generation: 1, ID: 1, Type: "issue.updated"})
			refresh := cmds[0].(model.ReadSnapshot)
			if refresh.Query.SelectedIssueUID == nil || *refresh.Query.SelectedIssueUID != "issue" || refresh.Query.IncludeHistory == nil || !*refresh.Query.IncludeHistory {
				t.Fatalf("refresh omitted selected authority/history: %+v", refresh.Query)
			}
			snapshot := model.Snapshot{Cursor: 1, Issues: []model.Issue{{UID: "issue"}}}
			loaded := model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot}
			if authority {
				loaded.Selected = m.DetailAuthority()
			}
			m, cmds = model.Update(m, loaded)
			if !authority {
				if len(cmds) != 1 {
					t.Fatalf("missing detail refetch: %#v", cmds)
				}
				m, _ = model.Update(m, model.DetailLoaded{UID: "issue", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "issue"}, Comments: []model.Comment{{UID: "one"}, {UID: "two"}}}})
			}
			if m.ActiveTab() != model.TabComments || cursorKey(m) != "two" || m.SelectedUID != "issue" {
				t.Fatalf("refresh reset reading position: tab=%v cursor on %q selected=%q", m.ActiveTab(), cursorKey(m), m.SelectedUID)
			}
		})
	}
}
