package model_test

import (
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTopBarDaemonAndCounts(t *testing.T) {
	project := model.Project{UID: "p", Name: "work"}
	issues := []model.Issue{{UID: "a", Status: "open"}, {UID: "b", Status: "closed"}}
	load := func(m model.Model, stats *model.ProjectStats) model.Model {
		m, _ = loadFirst(m, model.ProjectLoaded{Project: project, Daemon: model.Daemon{Kind: model.DaemonRemote, Version: "v0.18.0"}}, model.SnapshotLoaded{Snapshot: model.Snapshot{
			Catalog: []model.CatalogProject{{Project: project, Stats: stats}},
			Issues:  issues,
		}})
		return m
	}
	m := load(model.Model{Workspace: "/repo"}, &model.ProjectStats{Open: 5, Closed: 7})
	if got := m.CountsLine(); got != "5 open · 7 closed" {
		t.Fatalf("catalog counts %q", got)
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest(), Query: m.SnapshotQuery(), Snapshot: model.Snapshot{
		Catalog: []model.CatalogProject{{Project: project, Stats: &model.ProjectStats{Open: 6, Closed: 7}}},
		Issues:  issues,
	}})
	if got := m.CountsLine(); got != "6 open · 7 closed" {
		t.Fatalf("counts after refresh %q", got)
	}
	m = load(model.Model{Workspace: "/repo"}, nil)
	if got := m.CountsLine(); got != "1 open · 1 closed" {
		t.Fatalf("counted from Issues %q", got)
	}
}

// A Project Collection shows the Project's catalog stats whether or not the
// Pane is bound to it; any other Collection counts the Issues it lists.
func TestCountsLineForAnUnboundProjectCollection(t *testing.T) {
	project := model.Project{UID: "p", Name: "work"}
	snapshot := model.Snapshot{
		Catalog: []model.CatalogProject{{Project: project, Stats: &model.ProjectStats{Open: 5, Closed: 7}}},
		Issues:  []model.Issue{{UID: "a", Status: "open"}},
	}
	m, _ := loadFirst(model.Model{Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snapshot})
	if m.Bound || m.CountsLine() != "1 open · 0 closed" {
		t.Fatalf("Inbox: bound %v, counts %q", m.Bound, m.CountsLine())
	}
	picked := model.Collection{View: "project", ProjectUID: "p", Title: "work"}
	m, _ = model.Update(m, model.CollectionSelected{Collection: picked})
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest(), Query: m.SnapshotQuery(), Snapshot: snapshot})
	if got := m.CountsLine(); got != "5 open · 7 closed" {
		t.Fatalf("picked Project Collection counts %q", got)
	}
}

func TestDaemonFactsAndWhereTheCountsShow(t *testing.T) {
	m := model.Model{Daemon: model.Daemon{Name: "home", Kind: model.DaemonLocal, Version: "v0.23.0", Account: "alice"}}
	want := []model.Fact{{Text: "home"}, {Text: "local"}, {Text: "kata", Version: "v0.23.0"}, {Text: "as alice"}}
	if got := m.DaemonFacts(); !slices.Equal(got, want) {
		t.Fatalf("facts %v, want %v", got, want)
	}
	if got := (model.Model{}).DaemonFacts(); len(got) != 0 {
		t.Fatalf("facts before the Daemon is read %v", got)
	}

	project := model.Project{UID: "p", Name: "work"}
	loaded, _ := loadFirst(model.Model{Workspace: "/repo"}, model.ProjectLoaded{Project: project}, model.SnapshotLoaded{Snapshot: model.Snapshot{
		Issues: []model.Issue{{UID: "a", Status: "open"}},
	}})
	if !loaded.ShowCounts() {
		t.Fatal("counts hidden on the Issues Page")
	}
	loaded.Page = model.PageBoard
	if loaded.ShowCounts() {
		t.Fatal("counts shown off the Issues Page")
	}
	empty, _ := loadFirst(model.Model{Workspace: "/repo"}, model.ProjectLoaded{Project: project}, model.SnapshotLoaded{})
	if empty.ShowCounts() {
		t.Fatal("counts shown with nothing to count")
	}
}
