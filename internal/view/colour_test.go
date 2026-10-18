package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// styled is a node's spans as text -> style, in order, for a row or heading.
func styled(n tern.WireNode) [][2]string {
	var out [][2]string
	for _, prop := range []string{"label", "spans", "value"} {
		if spans, ok := n.P[prop].([]any); ok {
			for _, span := range spans {
				s := span.(map[string]any)
				style, _ := s["s"].(string)
				out = append(out, [2]string{s["t"].(string), style})
			}
		}
	}
	return out
}

// style is the style of the span whose text contains want.
func style(t testing.TB, n tern.WireNode, want string) string {
	t.Helper()
	for _, span := range styled(n) {
		if strings.Contains(span[0], want) {
			return span[1]
		}
	}
	t.Fatalf("no span containing %q in %v", want, styled(n))
	return ""
}

func ptr[T any](v T) *T { return &v }

func TestRowsToneStatusAndPriority(t *testing.T) {
	urgent := model.Issue{UID: "urgent", ShortID: "uuuu", Title: "Urgent", Status: "open", Priority: ptr(int64(0)), Labels: []string{"bug-label", "ui-label"}, ProjectName: "work"}
	high := model.Issue{UID: "high", ShortID: "hhhh", Title: "High", Status: "open", Priority: ptr(int64(1)), ProjectName: "work"}
	normal := model.Issue{UID: "normal", ShortID: "nnnn", Title: "Normal", Status: "open", Priority: ptr(int64(2)), ProjectName: "work"}
	low := model.Issue{UID: "low", ShortID: "llll", Title: "Low", Status: "closed", Priority: ptr(int64(3)), ProjectName: "work"}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{urgent, high, normal, low}}}},
	)
	nodes := render(t, m)
	row := func(issue model.Issue) tern.WireNode { return nodes[view.IssueID(issue.UID)] }

	for _, tc := range []struct {
		issue         model.Issue
		status, badge string
		style         string
	}{
		{urgent, "accent", "P0", "error"},
		{high, "accent", "P1", "warning"},
		{normal, "accent", "P2", "info"},
		{low, "muted", "P3", "muted"},
	} {
		if got := style(t, row(tc.issue), tc.issue.Status); got != tc.status {
			t.Errorf("%s status style = %q, want %q", tc.issue.Title, got, tc.status)
		}
		if got := style(t, row(tc.issue), tc.badge); got != tc.style {
			t.Errorf("%s priority style = %q, want %q", tc.issue.Title, got, tc.style)
		}
	}

	if got := text(row(urgent)); strings.Contains(got, "-label") {
		t.Errorf("row shows labels: %q", got)
	}
}

func TestSelectedRowIsAccent(t *testing.T) {
	nodes := render(t, browsing())
	if tone := nodes[view.IssueID(alpha.UID)].P["tone"]; tone != "accent" {
		t.Errorf("selected row tone = %v, want accent", tone)
	}
	if tone := nodes[view.IssueID(beta.UID)].P["tone"]; tone == "accent" {
		t.Errorf("unselected row tone = accent")
	}
}

func TestFocusedTopBarTitleIsAccent(t *testing.T) {
	m := browsing()
	heading := render(t, m)["main.topbar.left.heading"]
	if heading.P["tone"] != "accent" || style(t, heading, m.Collection.Title) != "accent strong" {
		t.Errorf("focused heading: tone=%v spans=%v", heading.P["tone"], styled(heading))
	}
	m = apply(m, model.KeyMsg{Chord: "enter"})
	heading = render(t, m)["main.topbar.left.heading"]
	if heading.P["tone"] == "accent" || style(t, heading, m.Collection.Title) != "strong" {
		t.Errorf("unfocused heading: tone=%v spans=%v", heading.P["tone"], styled(heading))
	}
}

func TestGroupHeadingsAreStrong(t *testing.T) {
	m := browsing()
	allOpen := m.Collections[4]
	m = apply(m, model.CollectionSelected{Collection: allOpen}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha, beta}}}})
	heading, ok := render(t, m)[view.IssueListID+".group.work"]
	if !ok {
		t.Fatal("no group heading for the work Project")
	}
	if got := styled(heading); len(got) != 1 || got[0] != [2]string{"work", "strong"} {
		t.Errorf("group heading = %v", got)
	}
}
