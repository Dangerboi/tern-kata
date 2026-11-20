package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// linkTitleFixture is a parent with closed children and a related Issue in another Project, as the live daemon holds them.
func linkTitleFixture(t *testing.T) (gateway *Gateway, workspace string, demo model.Project, parent string) {
	t.Helper()
	gateway = newGateway(t, newLocator(startKata(t, "0.21.0").target()))
	workspace, demo = project(t, gateway, "demo")
	otherWorkspace, other := project(t, gateway, "other")
	parent = createIssue(t, gateway, workspace, demo, "The parent")
	for _, title := range []string{"First child", "Second child", "Third child"} {
		created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: title, ParentUID: parent}))
		if created.Err != nil {
			t.Fatal(created.Err)
		}
		if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: created.UID, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-" + title})); closed.Err != nil {
			t.Fatal(closed.Err)
		}
	}
	far := createIssue(t, gateway, otherWorkspace, other, "Far away peer")
	shown := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: otherWorkspace, UID: far}))
	written := as[model.FieldWritten](t, run(t, gateway, model.WriteField{Workspace: workspace, UID: parent, ProjectID: demo.ID, Actor: "tester", Action: model.AddRelatedAction, Value: "other#" + shown.Selected.Issue.ShortID}))
	if written.Err != nil {
		t.Fatalf("link %+v", written)
	}
	return gateway, workspace, demo, parent
}

// titlesOf is the titles the detail's Links carry for the Issues at their far ends, by title.
func titlesOf(d *model.IssueDetailData) map[string]bool {
	got := map[string]bool{}
	for _, l := range d.Links {
		for _, title := range []string{l.FromTitle, l.ToTitle} {
			if title != "" {
				got[title] = true
			}
		}
	}
	return got
}

func wantTitles(t *testing.T, where string, d *model.IssueDetailData) {
	t.Helper()
	got := titlesOf(d)
	for _, want := range []string{"First child", "Second child", "Third child", "Far away peer"} {
		if !got[want] {
			t.Errorf("%s: no link carries the title %q (have %v)", where, want, got)
		}
	}
}

// The opening read and every live refresh rebuild the detail; each must carry the titles of closed children and of Issues in
// other Projects, since the model shows whatever the last read held.
func TestEveryReadOfADetailCarriesTheTitlesOfItsLinkedIssues(t *testing.T) {
	gateway, workspace, demo, parent := linkTitleFixture(t)

	opened := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: parent}))
	wantTitles(t, "opening read", opened.Selected)

	query := model.CollectionQuery(model.ProjectCollection(demo.UID, "demo"), model.Filters{})
	query.SelectedIssueUID = &parent
	refreshed := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1}))
	if refreshed.Selected == nil {
		t.Fatal("the snapshot read carries no selected detail")
	}
	wantTitles(t, "live refresh", refreshed.Selected)
}

// A title read once survives a refresh that cannot read it again, so a busy daemon does not blank titles the Pane already showed.
func TestATitleAlreadyReadSurvivesAReadThatCannotRepeatIt(t *testing.T) {
	gateway, workspace, _, parent := linkTitleFixture(t)
	c, err := gateway.client(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.ReadDetailWithLinkTitles(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	wantTitles(t, "first read", first.Selected)

	c.titles.Range(func(uid, v any) bool {
		c.titles.Store(uid, cachedTitle{title: v.(cachedTitle).title, at: time.Now().Add(-2 * linkTitleTTL)})
		return true
	})
	for i := range first.Selected.Links {
		first.Selected.Links[i].FromTitle, first.Selected.Links[i].ToTitle = "", ""
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	c.linkTitles(gone, first.Selected)
	wantTitles(t, "stale titles after reads that failed", first.Selected)
}

// A parent with a hundred children, most of them closed, still gets every title in one read, within the time budget.
func TestAParentWithAHundredChildrenGetsEveryTitle(t *testing.T) {
	gateway := newGateway(t, newLocator(startKata(t, "0.21.0").target()))
	workspace, demo := project(t, gateway, "demo")
	parent := createIssue(t, gateway, workspace, demo, "Big parent")
	const children = 100
	for i := range children {
		title := fmt.Sprintf("Child %03d", i)
		created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: title, ParentUID: parent}))
		if created.Err != nil {
			t.Fatal(created.Err)
		}
		if i%10 != 0 {
			run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: created.UID, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-" + title})
		}
	}
	start := time.Now()
	loaded := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: parent}))
	if got := len(titlesOf(loaded.Selected)); got != children {
		t.Errorf("%d of %d titles read", got, children)
	}
	if took := time.Since(start); took > linkTitleBudget {
		t.Errorf("reading the titles took %v, over the %v budget", took, linkTitleBudget)
	}
}

// Reads that only look at the Issue itself make no request for the titles of its Links.
func TestReadsThatDoNotShowTheDetailSkipLinkTitles(t *testing.T) {
	gateway, workspace, _, parent := linkTitleFixture(t)
	c, err := gateway.client(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := c.ReadDetail(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(plain.Selected); len(got) != 0 {
		t.Errorf("ReadDetail carries link titles %v", got)
	}
	cached := 0
	c.titles.Range(func(_, _ any) bool { cached++; return true })
	if cached != 0 {
		t.Errorf("ReadDetail read %d link titles", cached)
	}
	shown, err := c.ReadDetailWithLinkTitles(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	wantTitles(t, "ReadDetailWithLinkTitles", shown.Selected)
}

// An Issue kata will not show (deleted, no access) is remembered as unreadable for the cache period: the next read of a detail
// that links it makes no request for it, and a title the Pane already had stays.
func TestAnIssueThatCannotBeReadIsNotReadAgainOnEveryRefresh(t *testing.T) {
	gateway, workspace, _, parent := linkTitleFixture(t)
	c, err := gateway.client(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := c.ReadDetail(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	const gone = "01ZZZZZZZZZZZZZZZZZZZZZZZZ"
	detail.Selected.Links = append(detail.Selected.Links, model.Link{FromIssueUID: parent, ToIssueUID: gone})
	c.linkTitles(context.Background(), detail.Selected)
	v, ok := c.titles.Load(gone)
	if !ok {
		t.Fatal("the Issue kata refused is not remembered")
	}
	first := v.(cachedTitle)

	time.Sleep(5 * time.Millisecond)
	c.linkTitles(context.Background(), detail.Selected)
	v, _ = c.titles.Load(gone)
	if again := v.(cachedTitle); !again.at.Equal(first.at) {
		t.Errorf("the unreadable Issue was read again: %v then %v", first.at, again.at)
	}
}
