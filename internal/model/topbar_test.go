package model_test

import (
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
	if got := m.DaemonLine(); got != "Daemon: remote · kata v0.18.0" {
		t.Fatalf("daemon line %q", got)
	}
	if got := (model.Model{}).DaemonLine(); got != "Daemon: " {
		t.Fatalf("daemon line before the Daemon is read %q", got)
	}
	if got := m.CountsLine(); got != "open: 5 · closed: 7 · all: 12" {
		t.Fatalf("catalog counts %q", got)
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: model.Snapshot{
		Catalog: []model.CatalogProject{{Project: project, Stats: &model.ProjectStats{Open: 6, Closed: 7}}},
		Issues:  issues,
	}})
	if got := m.CountsLine(); got != "open: 6 · closed: 7 · all: 13" {
		t.Fatalf("counts after refresh %q", got)
	}
	m = load(model.Model{Workspace: "/repo"}, nil)
	if got := m.CountsLine(); got != "open: 1 · closed: 1 · all: 2" {
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
	if m.Bound || m.CountsLine() != "open: 1 · closed: 0 · all: 1" {
		t.Fatalf("Inbox: bound %v, counts %q", m.Bound, m.CountsLine())
	}
	picked := model.Collection{View: "project", ProjectUID: "p", Title: "work"}
	m, _ = model.Update(m, model.CollectionSelected{Collection: picked})
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: snapshot})
	if got := m.CountsLine(); got != "open: 5 · closed: 7 · all: 12" {
		t.Fatalf("picked Project Collection counts %q", got)
	}
}
