package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

var parentCounts = map[string]model.ChildCounts{"parent": {Open: 1, Total: 2}}

func countsLoaded(t *testing.T) model.Model {
	t.Helper()
	m, _ := loadFirst(model.Model{Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "project"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Cursor: 10, Issues: []model.Issue{{UID: "parent"}}, ChildCounts: parentCounts}})
	return m
}

// event delivers one stream event and returns the model and the read it asks for.
func event(t *testing.T, m model.Model, typ string) (model.Model, model.ReadSnapshot) {
	t.Helper()
	m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: typ})
	return m, cmdOf[model.ReadSnapshot](t, cmds)
}

func TestFirstLoadAsksForChildCounts(t *testing.T) {
	_, cmds := model.Update(model.Model{Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "project"}})
	if !cmdOf[model.ReadSnapshot](t, cmds).ChildCounts {
		t.Fatal("first load did not ask for child counts")
	}
}

func TestEventsAskForChildCountsOnlyWhenTheyCouldChangeThem(t *testing.T) {
	kept := []string{"issue.commented", "issue.comment_edited", "issue.assigned", "issue.unassigned", "issue.assignment_expired",
		"issue.assignment_renewed", "issue.labeled", "issue.unlabeled", "issue.priority_set", "issue.priority_cleared",
		"issue.updated", "issue.metadata_updated"}
	changing := []string{"issue.created", "issue.closed", "issue.reopened", "issue.linked", "issue.unlinked",
		"issue.links_changed", "issue.moved", "issue.soft_deleted", "issue.restored", "issue.from_the_future"}
	for _, typ := range kept {
		m, read := event(t, countsLoaded(t), typ)
		if read.ChildCounts {
			t.Errorf("%s asked for child counts", typ)
		}
		m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Cursor: 11, Issues: []model.Issue{{UID: "parent"}}}})
		if m.Snapshot.ChildCounts["parent"] != parentCounts["parent"] {
			t.Errorf("%s dropped the counts: %+v", typ, m.Snapshot.ChildCounts)
		}
	}
	for _, typ := range changing {
		if _, read := event(t, countsLoaded(t), typ); !read.ChildCounts {
			t.Errorf("%s did not ask for child counts", typ)
		}
	}
}

func TestDaemonResetAsksForChildCounts(t *testing.T) {
	_, read := event(t, countsLoaded(t), "sync.reset_required")
	if !read.ChildCounts {
		t.Fatalf("reset read %+v", read)
	}
}

func TestSwitchingProjectScopeAsksForChildCounts(t *testing.T) {
	m, _ := loadFirst(model.Model{Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "project"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Cursor: 10, ChildCounts: parentCounts, Catalog: []model.CatalogProject{{Project: model.Project{UID: "other", Name: "Other"}}}}})
	m, cmds := model.Update(m, model.CollectionSelected{Collection: model.ProjectCollection("other", "Other")})
	read := cmdOf[model.ReadSnapshot](t, cmds)
	if !read.ChildCounts {
		t.Fatal("a Project scope change did not ask for child counts")
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Cursor: 11, ChildCounts: map[string]model.ChildCounts{}}})
	if _, read = event(t, m, "issue.updated"); read.ChildCounts {
		t.Fatal("asked again within the same scope")
	}
}

func TestFailedChildCountsKeepTheCollectionAndAskAgain(t *testing.T) {
	first, cmds := model.Update(model.Model{Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "project"}})
	read := cmdOf[model.ReadSnapshot](t, cmds)
	first, _ = model.Update(first, model.SnapshotLoaded{Request: read.Request, Query: read.Query, ETag: "e1", Snapshot: model.Snapshot{Cursor: 10, Issues: []model.Issue{{UID: "parent"}}}})
	if first.Err != nil || first.Loading || len(first.Issues) != 1 {
		t.Fatalf("first load with failed counts: err %v loading %v issues %d", first.Err, first.Loading, len(first.Issues))
	}
	m, _ := model.Update(first, model.StreamOpened{Generation: first.StreamGeneration})
	m, read = event(t, m, "issue.updated")
	if !read.ChildCounts || read.ETag != "" {
		t.Fatalf("read after a failed count download: %+v, want counts asked and no ETag", read)
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Cursor: 11, Issues: []model.Issue{{UID: "parent"}}}})
	if m.Stale || m.StreamErr != nil || m.ReconnectAttempt != 0 {
		t.Fatalf("failed counts touched the stream: %+v", m.LiveState)
	}
}

func TestLiveRefreshKeepsCountsWhenTheirDownloadFails(t *testing.T) {
	m := countsLoaded(t)
	m, read := event(t, m, "issue.created")
	m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Cursor: 11, Issues: []model.Issue{{UID: "parent"}, {UID: "new"}}}})
	if m.Snapshot.ChildCounts["parent"] != parentCounts["parent"] || len(m.Issues) != 2 || m.Stale || m.ReconnectAttempt != 0 {
		t.Fatalf("after a failed count refresh: %+v", m)
	}
	if _, read = event(t, m, "issue.commented"); !read.ChildCounts {
		t.Fatal("did not ask again after the failure")
	}
}
