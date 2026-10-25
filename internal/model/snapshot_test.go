package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

var workProject = model.Project{ID: 7, UID: "p", Name: "work"}

func workSnapshot(uids ...string) model.Snapshot {
	snapshot := model.Snapshot{Catalog: []model.CatalogProject{{Project: workProject}}}
	for i, uid := range uids {
		// the Collection lists newest first, so a later uid is lower in the list
		snapshot.Issues = append(snapshot.Issues, model.Issue{UID: uid, ShortID: uid + "00", Status: "open", ProjectID: 7, UpdatedAt: ago(i)})
	}
	return snapshot
}

// loadedWork is a Pane that finished its first load of Project work, listing first and second.
func loadedWork(t *testing.T) model.Model {
	t.Helper()
	m, _ := loadFirst(model.Model{Workspace: "/repo", Cols: 160}, model.ProjectLoaded{Project: workProject}, model.SnapshotLoaded{Snapshot: workSnapshot("first", "second")})
	return m
}

func uids(m model.Model) []string {
	var got []string
	for _, issue := range m.Issues {
		got = append(got, issue.UID)
	}
	return got
}

func readIn(t *testing.T, cmds []model.Cmd) model.ReadSnapshot {
	t.Helper()
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			return read
		}
	}
	t.Fatalf("no snapshot read in %#v", cmds)
	return model.ReadSnapshot{}
}

// A result answers the query it was asked; once the Collection or the filters move on it is dropped, on every path that reads.
func TestSnapshotForAnotherQueryIsIgnoredOnEveryPath(t *testing.T) {
	today := model.Collection{View: "today", Title: "Today"}
	for _, tt := range []struct {
		name  string
		start func(t *testing.T) (model.Model, []model.Cmd)
	}{
		{"first load", func(t *testing.T) (model.Model, []model.Cmd) {
			m, _ := model.Update(model.Model{Workspace: "/repo"}, model.Start{Workspace: "/repo"})
			return model.Update(m, model.ProjectLoaded{Project: workProject})
		}},
		{"Collection switch", func(t *testing.T) (model.Model, []model.Cmd) {
			m := loadedWork(t)
			m.Collections = append(m.Collections, today)
			return model.Update(m, model.CollectionSelected{Collection: today})
		}},
		{"filter change", func(t *testing.T) (model.Model, []model.Cmd) {
			return model.Update(loadedWork(t), model.KeyMsg{Chord: "s"})
		}},
		{"live refresh", func(t *testing.T) (model.Model, []model.Cmd) {
			m := loadedWork(t)
			return model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.updated"})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := tt.start(t)
			read := readIn(t, cmds)
			before := uids(m)

			// The reader moves on while the read is out: its query no longer answers the current one.
			moved := m
			moved.Filters.Owner = "someone"
			got, _ := model.Update(moved, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: workSnapshot("late")})
			if !reflect.DeepEqual(uids(got), before) {
				t.Fatalf("a result for the old query was applied: %v, want %v", uids(got), before)
			}

			// A result of a read that was superseded by a newer one is dropped too.
			got, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request + 1, Query: read.Query, Snapshot: workSnapshot("late")})
			if !reflect.DeepEqual(uids(got), before) {
				t.Fatalf("a result of another request was applied: %v, want %v", uids(got), before)
			}
		})
	}
}

func TestFirstLoadAndRefreshApplyASnapshotIdentically(t *testing.T) {
	snapshot := workSnapshot("second", "first")
	snapshot.Cursor = 4

	first, _ := loadFirst(model.Model{Workspace: "/repo", Cols: 160}, model.ProjectLoaded{Project: workProject}, model.SnapshotLoaded{Snapshot: snapshot})

	refreshed := loadedWork(t)
	refreshed, cmds := model.Update(refreshed, model.StreamEvent{Generation: refreshed.StreamGeneration, ID: 4, Type: "issue.updated"})
	read := readIn(t, cmds)
	refreshed, _ = model.Update(refreshed, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: snapshot})

	if !reflect.DeepEqual(uids(first), uids(refreshed)) || first.Cursor != refreshed.Cursor ||
		!reflect.DeepEqual(first.Collections, refreshed.Collections) || !reflect.DeepEqual(first.Groups, refreshed.Groups) || first.Loading || refreshed.Loading {
		t.Fatalf("first load %v cursor %d; refresh %v cursor %d", uids(first), first.Cursor, uids(refreshed), refreshed.Cursor)
	}
}

func TestFailedSnapshotShowsAnErrorWhileLoadingAndGoesStaleOtherwise(t *testing.T) {
	boom := errors.New("daemon down")
	m, _ := model.Update(model.Model{Workspace: "/repo"}, model.Start{Workspace: "/repo"})
	m, cmds := model.Update(m, model.ProjectLoaded{Project: workProject})
	read := readIn(t, cmds)
	if m, _ = model.Update(m, model.SnapshotFailed{Request: read.Request, Err: boom}); m.Loading || !errors.Is(m.Err, boom) || m.Stale {
		t.Fatalf("first load failed: loading=%v err=%v stale=%v", m.Loading, m.Err, m.Stale)
	}

	m = loadedWork(t)
	m, cmds = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.updated"})
	read = readIn(t, cmds)
	if m, _ = model.Update(m, model.SnapshotFailed{Request: read.Request, Err: boom}); m.Err != nil || !m.Stale || !errors.Is(m.StreamErr, boom) {
		t.Fatalf("refresh failed: err=%v stale=%v stream err=%v", m.Err, m.Stale, m.StreamErr)
	}
}

// What Create selects is kept until the snapshot lists it, and dropped by anything the reader does meanwhile.
func TestPendingSelectionAfterCreate(t *testing.T) {
	created := func(t *testing.T) model.Model {
		t.Helper()
		m := loadedWork(t)
		m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
		m, _ = model.Update(m, model.KeyMsg{Text: "Made here"})
		m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
		m, _ = model.Update(m, model.IssueCreated{UID: "created"})
		if m.SelectedUID != "created" {
			t.Fatalf("Create did not select the new Issue: %q", m.SelectedUID)
		}
		return m
	}
	arrives := func(m model.Model) model.Model {
		m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.created"})
		if len(cmds) == 0 {
			// the read Create asked for is still out: answer that one
			m.Refreshing = false
			m, cmds = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 2, Type: "issue.created"})
		}
		read := readIn(t, cmds)
		m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: workSnapshot("first", "second", "created")})
		return m
	}

	t.Run("consumed when the Issue appears", func(t *testing.T) {
		m := arrives(created(t))
		if m.SelectedUID != "created" {
			t.Fatalf("selected %q after the Issue appeared", m.SelectedUID)
		}
		// consumed: a later snapshot does not pull the cursor back
		m, _ = model.Update(m, model.SelectMsg{UID: "first"})
		m = arrives(m)
		if m.SelectedUID != "first" {
			t.Fatalf("a consumed selection moved the cursor to %q", m.SelectedUID)
		}
	})

	for _, tt := range []struct {
		name string
		do   func(model.Model) (model.Model, []model.Cmd)
	}{
		{"keyboard navigation", func(m model.Model) (model.Model, []model.Cmd) {
			m, _ = model.Update(m, model.SelectMsg{UID: "second"})
			return model.Update(m, model.KeyMsg{Chord: "k"})
		}},
		{"pointer navigation", func(m model.Model) (model.Model, []model.Cmd) { return model.Update(m, model.SelectMsg{UID: "first"}) }},
		{"Collection switch", func(m model.Model) (model.Model, []model.Cmd) {
			return model.Update(m, model.CollectionSelected{Collection: m.Collections[4]})
		}},
		{"filter change", func(m model.Model) (model.Model, []model.Cmd) { return model.Update(m, model.KeyMsg{Chord: "s"}) }},
		{"opening another Issue", func(m model.Model) (model.Model, []model.Cmd) {
			return model.Update(m, model.ActionMsg{Action: model.SelectIssueAction, Value: "work/first00"})
		}},
	} {
		t.Run("cancelled by "+tt.name, func(t *testing.T) {
			m, _ := tt.do(created(t))
			selected := m.SelectedUID
			m.Collection, m.Filters = loadedWork(t).Collection, model.Filters{} // the snapshot below answers the Project again
			m.Refreshing = false
			m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.created"})
			read := readIn(t, cmds)
			m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: workSnapshot("first", "second", "created")})
			if m.SelectedUID == "created" && selected != "created" {
				t.Fatalf("the cancelled selection came back after %s", tt.name)
			}
		})
	}
}
