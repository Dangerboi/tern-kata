package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func TestTopBarDaemonAndCounts(t *testing.T) {
	project := model.Project{UID: "p", Name: "work"}
	issues := []model.Issue{{UID: "a", Status: "open"}, {UID: "b", Status: "closed"}}
	load := func(m model.Model, stats *g.ProjectStats) model.Model {
		m, _ = model.Update(m, model.ProjectLoaded{
			Project: project,
			Daemon:  model.Daemon{Remote: true, Version: "v0.18.0"},
			Snapshot: g.UISnapshotResponseBody{
				Catalog:    []g.UIProject{{Project: project, Stats: stats}},
				Collection: issues,
			},
		})
		return m
	}
	m := load(model.Model{Workspace: "/repo"}, &g.ProjectStats{Open: 5, Closed: 7})
	if got := m.DaemonLine(); got != "Daemon: remote · kata v0.18.0" {
		t.Fatalf("daemon line %q", got)
	}
	if got := m.CountsLine(); got != "open: 5 · closed: 7 · all: 12" {
		t.Fatalf("catalog counts %q", got)
	}
	m, _ = model.Update(m, model.CollectionLoaded{Collection: m.Collection, Snapshot: g.UISnapshotResponseBody{
		Catalog:    []g.UIProject{{Project: project, Stats: &g.ProjectStats{Open: 6, Closed: 7}}},
		Collection: issues,
	}})
	if got := m.CountsLine(); got != "open: 6 · closed: 7 · all: 13" {
		t.Fatalf("counts after refresh %q", got)
	}
	m = load(model.Model{Workspace: "/repo"}, nil)
	if got := m.CountsLine(); got != "open: 1 · closed: 1 · all: 2" {
		t.Fatalf("counted from Issues %q", got)
	}
}
