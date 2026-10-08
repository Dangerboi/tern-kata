package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func TestOpeningBoundProject(t *testing.T) {
	m, cmds := model.Update(model.Model{}, model.Start{Workspace: "/repo"})
	if !m.Loading || m.Workspace != "/repo" {
		t.Fatalf("opening Project: %+v", m)
	}
	if !reflect.DeepEqual(cmds, []model.Cmd{model.LoadProject{Workspace: "/repo"}}) {
		t.Fatalf("requested commands: %#v", cmds)
	}
}

func TestIssueNavigation(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		want string
		quit bool
	}{
		{"initial selection", nil, "uid-a", false},
		{"next", []string{"j"}, "uid-b", false},
		{"previous", []string{"j", "k"}, "uid-a", false},
		{"last", []string{"shift+g"}, "uid-c", false},
		{"first", []string{"shift+g", "g"}, "uid-a", false},
		{"upper boundary", []string{"k", "k"}, "uid-a", false},
		{"lower boundary", []string{"shift+g", "j"}, "uid-c", false},
		{"unknown key", []string{"z"}, "uid-a", false},
		{"quit", []string{"q"}, "uid-a", true},
		{"interrupt", []string{"ctrl+c"}, "uid-a", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := model.Update(model.Model{Loading: true}, model.ProjectLoaded{
				Project: model.Project{UID: "project", Name: "test"},
				Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{
					{UID: "uid-a", ShortID: "aaaa", Title: "First"},
					{UID: "uid-b", ShortID: "bbbb", Title: "Second"},
					{UID: "uid-c", ShortID: "cccc", Title: "Third"},
				}},
			})
			var cmds []model.Cmd
			for _, key := range tt.keys {
				m, cmds = model.Update(m, model.KeyMsg{Chord: key})
			}
			if m.Loading || m.Project.Name != "test" || len(m.Issues) != 3 || m.SelectedUID != tt.want || m.Quitting != tt.quit {
				t.Fatalf("visible Issue state: %+v", m)
			}
			if tt.quit && !reflect.DeepEqual(cmds, []model.Cmd{model.Quit{}}) {
				t.Fatalf("quit commands: %#v", cmds)
			}
		})
	}
}

func TestEmptyProjectNavigation(t *testing.T) {
	m, _ := model.Update(model.Model{}, model.ProjectLoaded{Project: model.Project{Name: "empty"}})
	for _, chord := range []string{"j", "k", "g", "shift+g"} {
		var cmds []model.Cmd
		m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
		if m.SelectedUID != "" || len(cmds) != 0 {
			t.Fatalf("%s changed empty Project: %+v, %#v", chord, m, cmds)
		}
	}
}

func TestPointerSelection(t *testing.T) {
	m := model.Model{Issues: []model.Issue{{UID: "a"}, {UID: "b"}}, SelectedUID: "a"}
	m, _ = model.Update(m, model.SelectMsg{UID: "b"})
	if m.SelectedUID != "b" {
		t.Fatalf("clicked Issue selected: %q", m.SelectedUID)
	}
	m, _ = model.Update(m, model.SelectMsg{UID: "missing"})
	if m.SelectedUID != "b" {
		t.Fatalf("unknown Issue changed selection: %q", m.SelectedUID)
	}
}

func TestDaemonFailureEndsLoading(t *testing.T) {
	want := errors.New("kata daemon v0.17.0 is too old; requires v0.18.0")
	m, _ := model.Update(model.Model{Loading: true}, model.Failed{Err: want})
	if m.Loading || m.Err != want {
		t.Fatalf("daemon error state: %+v", m)
	}
}
