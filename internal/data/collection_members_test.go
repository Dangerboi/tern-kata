package data

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Inbox, Upcoming and Deadlines read the all-open snapshot and keep their Issues by the rules of the daemon views kata keeps
// only for older web UIs: on a fixed day and zone, each lists the Issues and groups those views gave.
func TestInboxUpcomingAndDeadlinesKeepTheirIssuesFromAllOpen(t *testing.T) {
	t.Setenv("TZ", "America/Los_Angeles")
	now := systemCalendar.Now
	systemCalendar.Now = func() time.Time { return time.Date(2026, 7, 31, 20, 0, 0, 0, time.UTC) } // 13:00 on the 31st in LA
	t.Cleanup(func() { systemCalendar.Now = now })

	kata := startKata(t, "0.20.0")
	var mu sync.Mutex
	var views []string
	hook := func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/ui/snapshot" {
			mu.Lock()
			views = append(views, r.URL.Query().Get("view"))
			mu.Unlock()
		}
		return false
	}
	kata.intercept.Store(&hook)
	gateway := newGateway(t, newLocator(kata.target()))
	client := kata.target().Client
	workspace, capture := project(t, gateway, "capture")
	_, work := project(t, gateway, "work")
	if _, err := client.PatchProjectMetadataWithResponse(context.Background(), &g.PatchProjectMetadataRequestOptions{
		PathParams: &g.PatchProjectMetadataPath{ProjectID: capture.ID},
		Body:       &g.PatchProjectMetadataBody{Actor: "tester", Patch: map[string]any{"role": "inbox"}},
	}); err != nil {
		t.Fatal(err)
	}

	titles := map[string]string{}
	create := func(p model.Project, title, scheduled, deadline, parent string) string {
		created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: p.ID, Title: title, ScheduledOn: scheduled, DeadlineOn: deadline, ParentUID: parent}))
		if created.Err != nil {
			t.Fatal(created.Err)
		}
		titles[created.UID] = title
		return created.UID
	}
	patch := func(p model.Project, uid, key, value string) {
		if _, err := client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
			PathParams: &g.PatchIssueMetadataPath{ProjectID: p.ID, Ref: uid},
			Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{key: value}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	captured := create(capture, "captured", "", "", "")
	create(capture, "captured for later", "2026-08-05", "", "")
	create(work, "child of captured", "", "", captured)
	create(work, "child of plain", "", "", create(work, "plain", "", "", ""))
	create(work, "later", "2026-08-01", "", "")
	create(work, "today", "2026-07-31", "", "")
	patch(work, create(work, "this evening", "", "", ""), model.ScheduledKey, "2026-08-01T05:00:00Z") // 22:00 on the 31st in LA
	create(work, "overdue", "", "2026-07-30", "")
	patch(work, create(work, "due tonight", "", "", ""), model.DeadlineKey, "2026-08-01T03:00:00Z") // 20:00 on the 31st in LA
	create(work, "due later", "", "2026-08-10", "")

	for _, tt := range []struct {
		view   string
		groups map[string][]string
		links  []string // child titles of the parent Links the read keeps
	}{
		{"inbox", map[string][]string{"Inbox": {"captured", "captured for later"}}, []string{"child of captured"}},
		{"upcoming", map[string][]string{"2026-08-01": {"later"}, "2026-08-05": {"captured for later"}}, nil},
		{"deadlines", map[string][]string{"Overdue": {"overdue"}, "Today": {"due tonight"}, "2026-08-10": {"due later"}}, nil},
	} {
		t.Run(tt.view, func(t *testing.T) {
			collection, ok := model.FixedCollectionNamed(tt.view)
			if !ok {
				t.Fatalf("no Collection %q", tt.view)
			}
			views = nil
			loaded := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: model.CollectionQuery(collection, model.Filters{}), Request: 1}))
			if !slices.Equal(views, []string{"all-open"}) {
				t.Fatalf("read the daemon views %q, want all-open", views)
			}
			if loaded.LocalDate != "2026-07-31" || loaded.TimeZone != "America/Los_Angeles" {
				t.Fatalf("calendar %s %s", loaded.LocalDate, loaded.TimeZone)
			}
			got := map[string][]string{}
			listed := 0
			for _, group := range model.GroupCollection(collection, model.Filters{}, loaded.Snapshot.Issues, model.NewPaneCalendar(loaded.LocalDate, loaded.TimeZone, "")) {
				for _, issue := range group.Issues {
					got[group.Title] = append(got[group.Title], issue.Title)
				}
				slices.Sort(got[group.Title])
				listed += len(group.Issues)
			}
			if !maps.EqualFunc(got, tt.groups, slices.Equal[[]string]) || listed != len(loaded.Snapshot.Issues) {
				t.Errorf("groups %v of %d Issues, want %v", got, len(loaded.Snapshot.Issues), tt.groups)
			}
			var links []string
			for _, l := range loaded.Snapshot.Links {
				links = append(links, titles[l.FromIssueUID])
			}
			if !slices.Equal(links, tt.links) {
				t.Errorf("links from %q, want %q", links, tt.links)
			}
		})
	}
}
