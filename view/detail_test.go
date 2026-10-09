package view_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	g "go.kenn.io/kata/pkg/client/generated"
)

func detailNodes(t *testing.T, issue model.Issue) shown {
	t.Helper()
	m := apply(browsing(), detailOf(issue))
	return render(t, m)
}

func TestDetailSectionsRenderInOrder(t *testing.T) {
	nodes := detailNodes(t, alpha)
	ids := nodes.under(view.DetailID)
	var headings []string
	for section := model.SectionProperties; section <= model.SectionHistory; section++ {
		id := view.DetailSectionID(section) + ".heading"
		if _, ok := nodes[id]; !ok {
			if section == model.SectionBody {
				continue
			}
			t.Fatalf("no heading for %v", section)
		}
		headings = append(headings, text(nodes[id]))
	}
	want := []string{"Properties", "Labels", "Links", "Comments", "Change history"}
	if !slices.Equal(headings, want) {
		t.Fatalf("headings %v, want %v", headings, want)
	}
	root := nodes[view.DetailContentID]
	var order []string
	for _, c := range root.C {
		if id := strings.TrimPrefix(c.ID, view.DetailContentID+"."); strings.HasPrefix(id, "section") {
			order = append(order, id)
		}
	}
	if !slices.IsSorted(order) || len(order) != 6 || order[0] != "section0" {
		t.Fatalf("on-screen order %v of %v", order, ids)
	}
	if text(nodes[view.DetailSectionID(model.SectionProperties)+".heading"]) != "Properties" {
		t.Fatal("properties is not first")
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
			props := view.DetailSectionID(model.SectionProperties) + ".properties"
			if n := nodes[props+".badges.status"]; n.P["tone"] != tt.status || text(n) != tt.issue.Status {
				t.Errorf("status badge: %v", n)
			}
			if n := nodes[props+".badges.priority"]; n.P["tone"] != tt.priority || !strings.HasPrefix(text(n), "P") {
				t.Errorf("priority badge: %v", n)
			}
			labels := view.DetailSectionID(model.SectionLabels) + ".labels"
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
	if got := text(nodes[view.DetailContentID+".heading.title"]); got != "aaaa - Alpha" {
		t.Errorf("title %q, want %q", got, "aaaa - Alpha")
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
	if _, ok := nodes[view.DetailSectionID(model.SectionBody)+".body"]; !ok {
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
	for _, id := range []string{view.DetailContentID + ".heading.title", view.DetailSectionID(model.SectionProperties), view.DetailSectionID(model.SectionBody), view.DetailSectionID(model.SectionHistory)} {
		if _, ok := nodes[id]; !ok {
			t.Errorf("%s is not in the scroll container", id)
		}
	}
}
