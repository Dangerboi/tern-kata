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
			t.Fatalf("no heading for %v", section)
		}
		headings = append(headings, text(nodes[id]))
	}
	want := []string{"Properties", "Labels", "Body", "Links", "Comments", "Change history"}
	if !slices.Equal(headings, want) {
		t.Fatalf("headings %v, want %v", headings, want)
	}
	root := nodes[view.DetailID]
	var order []string
	for _, c := range root.C {
		if strings.Contains(c.ID, ".section") {
			order = append(order, strings.TrimPrefix(c.ID, view.DetailID+"."))
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
