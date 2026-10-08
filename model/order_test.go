package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// ago returns an update time n hours before a fixed instant: a smaller n is more recently updated.
func ago(n int) time.Time {
	return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC).Add(-time.Duration(n) * time.Hour)
}

func uidsOf(m model.Model) []string {
	var uids []string
	for _, row := range m.VisibleRows() {
		uids = append(uids, row.UID)
	}
	return uids
}

func TestProjectListIsOrderedByUpdatedDescendingWithUIDTies(t *testing.T) {
	priority := new(int64(0))
	m, _ := model.Update(model.Model{}, model.ProjectLoaded{
		Project: model.Project{UID: "p", Name: "Work"},
		Snapshot: g.UISnapshotResponseBody{
			Collection: []model.Issue{
				{UID: "old", Title: "A", Priority: priority, UpdatedAt: ago(5)},
				{UID: "tie-b", Title: "B", UpdatedAt: ago(1)},
				{UID: "parent", Title: "C", UpdatedAt: ago(3)},
				{UID: "tie-a", Title: "Z", UpdatedAt: ago(1)},
				{UID: "kid-old", Title: "A", UpdatedAt: ago(4)},
				{UID: "kid-new", Title: "Z", UpdatedAt: ago(2)},
			},
			CollectionLinks: []g.UILink{
				{Type: "parent", FromIssueUID: "kid-old", ToIssueUID: "parent"},
				{Type: "parent", FromIssueUID: "kid-new", ToIssueUID: "parent"},
			},
		},
	})
	if got, want := uidsOf(m), []string{"tie-a", "tie-b", "parent", "old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roots: %v, want %v", got, want)
	}
	m, _ = model.Update(m, model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"})
	if got, want := uidsOf(m), []string{"tie-a", "tie-b", "parent", "kid-new", "kid-old", "old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children: %v, want %v", got, want)
	}
}

func TestCollectionGroupsAreOrderedByUpdatedDescending(t *testing.T) {
	today := "2026-10-09"
	for _, tt := range []struct {
		name   string
		view   string
		issues []model.Issue
		want   []string
	}{
		{"today", "today", []model.Issue{
			{UID: "a", Status: "open", ScheduledOnDate: &today, UpdatedAt: ago(3)},
			{UID: "b", Status: "open", ScheduledOnDate: &today, UpdatedAt: ago(1)},
		}, []string{"b", "a"}},
		{"all-open groups by project", "all-open", []model.Issue{
			{UID: "a", Status: "open", ProjectName: "P", UpdatedAt: ago(3)},
			{UID: "b", Status: "open", ProjectName: "P", UpdatedAt: ago(1)},
			{UID: "c", Status: "open", ProjectName: "Q", UpdatedAt: ago(0)},
		}, []string{"b", "a", "c"}},
		{"deadlines today group is deadline order first", "deadlines", []model.Issue{
			{UID: "late", Status: "open", DeadlineOnDate: new("2026-10-09"), UpdatedAt: ago(1)},
			{UID: "early-overdue", Status: "open", DeadlineOnDate: new("2026-10-01"), UpdatedAt: ago(9)},
			{UID: "later-overdue", Status: "open", DeadlineOnDate: new("2026-10-02"), UpdatedAt: ago(0)},
		}, []string{"early-overdue", "later-overdue", "late"}},
		{"deadlines future group is updated order", "deadlines", []model.Issue{
			{UID: "a", Status: "open", DeadlineOnDate: new("2026-11-01"), UpdatedAt: ago(3)},
			{UID: "b", Status: "open", DeadlineOnDate: new("2026-11-01"), UpdatedAt: ago(1)},
		}, []string{"b", "a"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := model.Collection{View: tt.view, Title: tt.view}
			m, _ := model.Update(model.Model{Collection: c}, model.CollectionLoaded{Collection: c, Snapshot: g.UISnapshotResponseBody{Collection: tt.issues}, LocalDate: today})
			if got := uidsOf(m); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("%v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectionFollowsUIDAcrossLiveReorder(t *testing.T) {
	m, _ := model.Update(model.Model{Workspace: "/repo", SelectedUID: "old"}, model.ProjectLoaded{
		Project:  model.Project{UID: "p"},
		Snapshot: g.UISnapshotResponseBody{Cursor: 1, Collection: []model.Issue{{UID: "new", UpdatedAt: ago(1)}, {UID: "old", UpdatedAt: ago(2)}}},
	})
	m, cmds := model.Update(m, model.StreamEvent{Generation: 1, ID: 2, Type: "issue.updated"})
	refresh := cmds[0].(model.RefreshSnapshot)
	m, _ = model.Update(m, model.SnapshotRefreshed{
		Request: refresh.Request, Query: refresh.Query,
		Snapshot: g.UISnapshotResponseBody{Cursor: 2, Collection: []model.Issue{{UID: "new", UpdatedAt: ago(1)}, {UID: "old", UpdatedAt: ago(0)}}},
	})
	if got := uidsOf(m); !reflect.DeepEqual(got, []string{"old", "new"}) || m.SelectedUID != "old" {
		t.Fatalf("rows %v selected %q", got, m.SelectedUID)
	}
}
