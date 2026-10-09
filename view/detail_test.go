package view_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	g "go.kenn.io/kata/pkg/client/generated"
)

func detailNodes(t *testing.T, issue model.Issue) shown {
	t.Helper()
	m := apply(browsing(), detailOf(issue))
	return render(t, m)
}

func tabLabels(nodes shown) []string {
	var labels []string
	for tab := model.TabComments; tab <= model.TabLinks; tab++ {
		labels = append(labels, text(nodes[view.DetailActivityID+".tabs.tab"+strconv.Itoa(int(tab))]))
	}
	return labels
}

// detailWithActivity is alpha with 2 comments, 3 events and 1 Link.
func detailWithActivity(tab model.ActivityTab) model.Model {
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	selected := &g.UISelectedAuthority{Issue: &alpha,
		Comments: []g.Comment{{UID: "c1", Author: "dana", CreatedAt: at, Body: "first"}, {UID: "c2", Author: "dana", CreatedAt: at, Body: "second"}},
		History:  []g.Event{{UID: "e1", Actor: "dana", Type: "issue.created", Payload: `{}`, CreatedAt: at}, {UID: "e2", Actor: "dana", Type: "issue.reopened", Payload: `{}`, CreatedAt: at}, {UID: "e3", Actor: "dana", Type: "issue.reopened", Payload: `{}`, CreatedAt: at}},
		Links:    []g.UILink{{ID: 7, FromIssueUID: "alpha", ToIssueUID: "beta", ToQualifiedID: "work#bbbb", Type: "related"}},
	}
	return apply(browsing(), model.DetailLoaded{UID: "alpha", Selected: selected}, model.ActivityTabMsg{Tab: tab})
}

func TestActivityTabsShowCountsAndMarkTheActiveOne(t *testing.T) {
	for _, tt := range []struct {
		tab  model.ActivityTab
		want []string
	}{
		{model.TabComments, []string{"[ Comments (2) ]", "Events (3)", "Links (1)"}},
		{model.TabEvents, []string{"Comments (2)", "[ Events (3) ]", "Links (1)"}},
		{model.TabLinks, []string{"Comments (2)", "Events (3)", "[ Links (1) ]"}},
	} {
		nodes := render(t, detailWithActivity(tt.tab))
		if got := tabLabels(nodes); !slices.Equal(got, tt.want) {
			t.Errorf("tab %v: labels %v, want %v", tt.tab, got, tt.want)
		}
		if text(nodes[view.DetailActivityID+".tabs.name"]) != "Activity" {
			t.Error("no Activity heading")
		}
		for tab := model.TabComments; tab <= model.TabLinks; tab++ {
			n := nodes[view.DetailActivityID+".tabs.tab"+strconv.Itoa(int(tab))]
			style := spansOf(n.P)[0][1]
			if (tab == tt.tab) != strings.Contains(style, "accent") {
				t.Errorf("tab %v active=%v is styled %q", tab, tab == tt.tab, style)
			}
			if click := n.P["actions"].(map[string]any)["click"]; click != "activity-tab="+strconv.Itoa(int(tab)) {
				t.Errorf("tab %v click %v", tab, click)
			}
		}
	}
}

func TestOnlyTheActiveTabsRowsRender(t *testing.T) {
	for _, tt := range []struct {
		tab  model.ActivityTab
		rows []string
	}{
		{model.TabComments, []string{"c1", "c2"}},
		{model.TabEvents, []string{"e1", "e2", "e3"}},
		{model.TabLinks, []string{"7"}},
	} {
		nodes := render(t, detailWithActivity(tt.tab))
		var got []string
		for _, c := range nodes[view.DetailActivityID].C {
			if id := strings.TrimPrefix(c.ID, view.DetailActivityID+"."); id != "tabs" {
				got = append(got, id)
			}
		}
		if !slices.Equal(got, tt.rows) {
			t.Errorf("tab %v renders %v, want %v", tt.tab, got, tt.rows)
		}
	}
}

func TestEmptyActivityTabsSaySo(t *testing.T) {
	for tab, want := range map[model.ActivityTab]string{model.TabComments: "No comments.", model.TabEvents: "No events.", model.TabLinks: "No links."} {
		nodes := render(t, apply(browsing(), model.ActivityTabMsg{Tab: tab}))
		if got := text(nodes[view.DetailActivityID+".empty"]); got != want {
			t.Errorf("tab %v says %q, want %q", tab, got, want)
		}
	}
}

func TestDetailBadgesAndChipsAreToned(t *testing.T) {
	p1, p3 := int64(1), int64(3)
	for _, tt := range []struct {
		name             string
		issue            model.Issue
		status, priority string
	}{
		{"open P1", model.Issue{UID: "a", ShortID: "aaaa", Title: "A", Status: "open", Priority: &p1, Labels: []string{"bug", "ui"}}, "accent", "warning"},
		{"closed P3", model.Issue{UID: "a", ShortID: "aaaa", Title: "A", Status: "closed", Priority: &p3, Labels: []string{"bug", "ui"}}, "muted", "muted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := apply(browsing(), model.DetailLoaded{UID: "alpha", Selected: &g.UISelectedAuthority{Issue: &tt.issue}})
			nodes := render(t, m)
			props := view.DetailContentID + ".properties"
			if n := nodes[view.DetailContentID+".heading.status"]; n.P["tone"] != tt.status || text(n) != tt.issue.Status {
				t.Errorf("status badge: %v", n)
			}
			if n := nodes[props+".badges.priority"]; n.P["tone"] != tt.priority || !strings.HasPrefix(text(n), "P") {
				t.Errorf("priority badge: %v", n)
			}
			labels := view.DetailContentID + ".labels"
			for _, l := range tt.issue.Labels {
				if n, ok := nodes[labels+".label-"+l]; !ok || text(n) != l {
					t.Errorf("label chip %s: %v", l, n)
				}
			}
		})
	}
}

func TestDetailHeaderIsTheTitleAndStatusWithoutButtonsOrBodyHeading(t *testing.T) {
	nodes := detailNodes(t, alpha)
	if got := text(nodes[view.DetailContentID+".heading.title"]); got != "#aaaa - Alpha" {
		t.Errorf("title %q, want %q", got, "#aaaa - Alpha")
	}
	if badge := nodes[view.DetailContentID+".heading.status"]; text(badge) != "open" || badge.P["tone"] != "accent" {
		t.Errorf("status badge after the title: %v", badge)
	}
	if ids := nodes.under(view.DetailID); slices.ContainsFunc(ids, func(id string) bool {
		return strings.HasSuffix(id, ".navigation") || strings.Contains(id, ".navigation.") || strings.HasSuffix(id, ".back") || strings.HasSuffix(id, ".toggle")
	}) {
		t.Errorf("Back or Hide detail buttons are still drawn: %v", ids)
	}
	for _, id := range nodes.under(view.DetailID) {
		if text(nodes[id]) == "Body" || text(nodes[id]) == "Back" || text(nodes[id]) == "Hide detail" {
			t.Errorf("%s shows %q", id, text(nodes[id]))
		}
	}
	if _, ok := nodes[view.DetailContentID+".body"]; !ok {
		t.Error("the body itself is gone")
	}
}

// Tern's scroll op reaches a List's scroller only through a node inside it, so the detail is one List
// whose single child holds everything: nothing of the detail sits outside the scroll container.
func TestDetailIsOneScrollContainerWithOneChild(t *testing.T) {
	nodes := detailNodes(t, alpha)
	if root := nodes[view.DetailScrollID]; root.K != "list" || len(root.C) != 1 || root.C[0].ID != view.DetailContentID {
		t.Fatalf("the detail scroller is %q with children %v, want one list child %s", root.K, root.C, view.DetailContentID)
	}
	for _, id := range nodes.under(view.DetailID) {
		if id != view.DetailID && id != view.DetailScrollID && !strings.HasPrefix(id, view.DetailScrollID+".") {
			t.Errorf("%s is outside the scroll container", id)
		}
	}
	for _, id := range []string{view.DetailContentID + ".heading.title", view.DetailContentID + ".properties", view.DetailContentID + ".body", view.DetailActivityID} {
		if _, ok := nodes[id]; !ok {
			t.Errorf("%s is not in the scroll container", id)
		}
	}
}

func TestDetailTitleShowsIDInAccentThenTitleStrong(t *testing.T) {
	nodes := detailNodes(t, alpha)
	title := nodes[view.DetailID+".viewport.content.heading.title"]
	want := [][2]string{{"#" + alpha.ShortID, "accent"}, {" - " + alpha.Title, "strong"}}
	if got := styled(title); !slices.Equal(got, want) {
		t.Errorf("title spans = %v, want %v", got, want)
	}
}

func TestCommentHeadingShowsTheAuthorAndTheTimeInTheEventsFormat(t *testing.T) {
	nodes := render(t, detailWithActivity(model.TabComments))
	want := "dana / " + time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC).In(time.Local).Format("2006-01-02 15:04")
	if got := text(nodes[view.DetailActivityID+".c1.heading"]); got != want {
		t.Errorf("comment heading %q, want %q", got, want)
	}
}
