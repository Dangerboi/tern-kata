package driver_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// benchSizes are the Collection sizes the Pane is measured at: a small Project, a busy one, and a large one.
var benchSizes = []int{50, 500, 2000}

// benchIssues is a Project of n Issues shaped like a working one: most in a Lane, some with other labels, owners and
// priorities, a tenth closed in the last week, and a body of a few paragraphs.
func benchIssues(n int) []model.Issue {
	lanes := model.DefaultLanes()
	body := strings.Repeat("A paragraph of the Issue body, long enough to wrap in the detail column. ", 6)
	issues := make([]model.Issue, n)
	for i := range issues {
		uid := fmt.Sprintf("01K%023d", i)
		labels := []string{}
		if i%5 != 0 {
			labels = append(labels, lanes[i%len(lanes)])
		}
		if i%3 == 0 {
			labels = append(labels, "bug")
		}
		if i%7 == 0 {
			labels = append(labels, "area:ui")
		}
		issue := model.Issue{
			UID: uid, ID: int64(i + 1), ShortID: fmt.Sprintf("%04x", i), QualifiedID: fmt.Sprintf("work#%d", i+1),
			Title: fmt.Sprintf("Issue %d: a title of a typical length for the list", i), Body: body, Status: "open",
			ProjectName: "work", ProjectID: 7, Labels: labels, Author: "tester",
			CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute),
			UpdatedAt: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Minute),
		}
		if i%3 == 1 {
			owner := "agent"
			issue.Owner = &owner
		}
		if i%2 == 0 {
			p := int64(i % 5)
			issue.Priority = &p
		}
		if i%10 == 9 {
			at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Minute)
			issue.Status, issue.ClosedAt = "closed", &at
		}
		issues[i] = issue
	}
	return issues
}

// benchLinks gives every tenth Issue the nine after it as children, and every seventh Issue a blocker.
func benchLinks(issues []model.Issue) []model.Link {
	var links []model.Link
	for i, issue := range issues {
		if i%10 != 0 {
			parent := issues[i-i%10]
			links = append(links, model.Link{ID: int64(len(links) + 1), Type: "parent", FromIssueUID: issue.UID, FromStatus: issue.Status,
				FromQualifiedID: issue.QualifiedID, ToIssueUID: parent.UID, ToStatus: parent.Status, ToQualifiedID: parent.QualifiedID})
		}
		if i%7 == 3 && i > 0 {
			blocker := issues[i-1]
			links = append(links, model.Link{ID: int64(len(links) + 1), Type: "blocks", FromIssueUID: blocker.UID, FromStatus: blocker.Status,
				FromQualifiedID: blocker.QualifiedID, ToIssueUID: issue.UID, ToStatus: issue.Status, ToQualifiedID: issue.QualifiedID})
		}
	}
	return links
}

// benchDetail is a busy Issue's detail: 40 comments, a history of 150 events and the links of its children.
func benchDetail(issue model.Issue, links []model.Link) *model.IssueDetailData {
	detail := &model.IssueDetailData{Issue: &issue}
	for _, link := range links {
		if link.FromIssueUID == issue.UID || link.ToIssueUID == issue.UID {
			detail.Links = append(detail.Links, link)
		}
	}
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := range 40 {
		detail.Comments = append(detail.Comments, model.Comment{UID: fmt.Sprintf("c%03d", i), Author: "agent", CreatedAt: at.Add(time.Duration(i) * time.Hour),
			Body: strings.Repeat("A comment on the work, with a sentence or two of findings. ", 4)})
	}
	for i := range 150 {
		detail.History = append(detail.History, model.Event{UID: fmt.Sprintf("e%03d", i), Type: "issue.labeled", Actor: "agent", ProjectName: "work",
			CreatedAt: at.Add(time.Duration(i) * time.Minute), Payload: `{"label":"needs-triage"}`})
	}
	return detail
}

// benchPane is a 200-column Pane on a Project of n Issues, the first selected with a busy detail loaded. page is "list", "board"
// (the Board Page) or "detail" (the list with the detail column focused).
func benchPane(b testing.TB, n int, page string) driver.State {
	b.Helper()
	issues := benchIssues(n)
	links := benchLinks(issues)
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	snapshot := model.Snapshot{Issues: issues, Links: links, Catalog: []model.CatalogProject{{Project: project}}, Cursor: 1}
	s := benchFeed(b, driver.State{}, model.Start{Workspace: "/repo"}, resize(200))
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: project}, model.SnapshotLoaded{Snapshot: snapshot, LocalDate: boardToday, TimeZone: "UTC"})
	s = benchFeed(b, s, model.DetailLoaded{UID: issues[0].UID, Selected: benchDetail(issues[0], links)})
	switch page {
	case "board":
		s = benchFeed(b, s, key("b"), model.LanesLoaded{ProjectUID: "p"})
	case "detail":
		s = benchFeed(b, s, key("tab"))
	}
	return s
}

func benchFeed(b testing.TB, s driver.State, inputs ...any) driver.State {
	b.Helper()
	for _, in := range inputs {
		out := driver.Step(s, in, now)
		if out.Err != nil {
			b.Fatalf("%T: %v", in, out.Err)
		}
		s = out.State
	}
	return s
}

// liveRefresh is s after a stream event asked for a snapshot, and the answer to that read: one Issue retitled, and the
// selected Issue's detail riding along as the live query asks.
func liveRefresh(b testing.TB, s driver.State) (driver.State, model.SnapshotLoaded) {
	b.Helper()
	m := s.Model
	out := driver.Step(s, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.updated"}, now)
	for _, cmd := range out.Cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			issues := slices.Clone(m.Snapshot.Issues)
			issues[len(issues)/2].Title += " (edited)"
			links := benchLinks(issues)
			snapshot := model.Snapshot{Issues: issues, Links: links, Catalog: m.Snapshot.Catalog, Cursor: m.Cursor + 1}
			loaded := model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: snapshot, LocalDate: boardToday, TimeZone: "UTC"}
			if uid := read.Query.SelectedIssueUID; uid != nil {
				at := slices.IndexFunc(issues, func(issue model.Issue) bool { return issue.UID == *uid })
				loaded.Selected = benchDetail(issues[at], links)
			}
			return out.State, loaded
		}
	}
	b.Fatal("the stream event asked for no snapshot")
	return s, model.SnapshotLoaded{}
}

// moved reports whether in moves the list selection, the detail cursor on the "detail" page, or the Jobs Page's selection.
func moved(s driver.State, in tern.Input) bool {
	next := driver.Step(s, in, now).State.Model
	return next.SelectedUID != s.Model.SelectedUID || next.DetailIndex() != s.Model.DetailIndex() || next.Jobs.Selected != s.Model.Jobs.Selected
}

// benchMoves steps s by move, alternately, diffing each view against the one before.
func benchMoves(b *testing.B, s driver.State, move []tern.Input) {
	if !moved(s, move[0]) {
		b.Fatal("the move input moved nothing")
	}
	r := tern.NewReconciler("bench")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		out := driver.Step(s, move[i%2], now)
		if _, err := r.Diff(*out.View); err != nil {
			b.Fatal(err)
		}
		s = out.State
		i++
	}
}

// benchJobs is a Project's 20 cron jobs that each make an Issue per run: two every minute, three hourly, the rest on weekdays.
func benchJobs() []model.CronJob {
	jobs := make([]model.CronJob, 20)
	for i := range jobs {
		job := promptJob(fmt.Sprintf("J%02d", i), fmt.Sprintf("Job %02d", i))
		switch {
		case i < 2:
			job.Definition.Trigger.Cron = "* * * * *"
		case i < 5:
			job.Definition.Trigger.Cron = "0 * * * *"
		}
		jobs[i] = job
	}
	return jobs
}

// BenchmarkStep is one Pane step: model.Update and view.Render, then the SDK reconciler diffing the view against the
// last one drawn, which is what term.Render does before writing. "move" alternates down and up: a selection change each
// step, or on the "detail" page a detail cursor move. "refresh" is a live snapshot arriving with one Issue changed. The
// Jobs Page moves over benchJobs, its inspector on the selected job.
func BenchmarkStep(b *testing.B) {
	for _, page := range []string{"list", "board", "detail"} {
		for _, n := range benchSizes {
			b.Run(fmt.Sprintf("page=%s/n=%d/input=move", page, n), func(b *testing.B) {
				move := []tern.Input{key("down"), key("up")}
				if page == "detail" {
					move = []tern.Input{key("j"), key("k")}
				}
				benchMoves(b, benchPane(b, n, page), move)
			})
			b.Run(fmt.Sprintf("page=%s/n=%d/input=refresh", page, n), func(b *testing.B) {
				s, loaded := liveRefresh(b, benchPane(b, n, page))
				r := tern.NewReconciler("bench")
				if _, err := r.Diff(view.Render(s.Model, now)); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					out := driver.Step(s, loaded, now)
					if _, err := r.Diff(*out.View); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
	b.Run("page=jobs/jobs=20/input=move", func(b *testing.B) {
		out := driver.Step(benchPane(b, 50, "list"), key("3"), now)
		load, ok := askedJobs(out.Cmds)
		if !ok {
			b.Fatal("the Jobs Page asked for no jobs")
		}
		s := benchFeed(b, out.State, model.JobsLoaded{Request: load.Request, Jobs: benchJobs()})
		benchMoves(b, s, []tern.Input{key("down"), key("up")})
	})
}
