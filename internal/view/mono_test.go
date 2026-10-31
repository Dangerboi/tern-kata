package view_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// monoRuns are the texts of a node's spans that carry the mono token.
func monoRuns(n tern.WireNode) []string {
	var out []string
	for _, run := range styled(n) {
		if slices.Contains(strings.Fields(run[1]), "mono") {
			out = append(out, run[0])
		}
	}
	return out
}

func wantMono(t *testing.T, what string, n tern.WireNode, want ...string) {
	t.Helper()
	if got := monoRuns(n); !slices.Equal(got, want) {
		t.Errorf("%s: mono spans %q, want %q (spans %v)", what, got, want, spansOf(n.P))
	}
}

// An Issue row draws its id, priority, kids count and updated time in mono, and leaves the status and title in the face of the
// surface.
func TestListRowMachineStringsAreMono(t *testing.T) {
	parent := alpha
	p1 := int64(1)
	parent.Priority = &p1
	kid := beta
	links := []model.Link{{ID: 1, Type: "parent", FromIssueUID: kid.UID, ToIssueUID: parent.UID, FromStatus: "open"}}
	m := apply(browsing(), snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{parent, kid}, Links: links}}})
	nodes := render(t, m)
	row := nodes[view.IssueID(parent.UID)]
	var monos []string
	for _, s := range monoRuns(row) {
		monos = append(monos, strings.TrimSpace(s))
	}
	for _, want := range []string{parent.ShortID, "P1", "1/1"} {
		if !slices.ContainsFunc(monos, func(s string) bool { return strings.HasSuffix(s, want) || s == want }) {
			t.Errorf("%q is not mono in the row: %q", want, monos)
		}
	}
	if len(monos) < 4 {
		t.Errorf("the updated time is not mono: %q", monos)
	}
	for _, run := range styled(row) {
		if strings.Contains(run[0], parent.Title) && strings.Contains(run[1], "mono") {
			t.Errorf("the title is mono: %v", run)
		}
	}
}

func TestTopBarCountsAndPositionAreMono(t *testing.T) {
	nodes := render(t, browsing())
	counts := nodes["main.topbar.counts"]
	got := monoRuns(counts)
	if len(got) != 3 || strings.Join(got, "") == "" {
		t.Errorf("counts mono spans %q, spans %v", got, spansOf(counts.P))
	}
	if got := text(counts); !strings.HasPrefix(got, "open: ") {
		t.Errorf("counts text = %q", got)
	}
	position := nodes["dock.hints.position"]
	if len(monoRuns(position)) != 1 {
		t.Errorf("position is not mono: %v", spansOf(position.P))
	}
}

func TestDetailMachineStringsAreMono(t *testing.T) {
	processZoneIsUTC(t)
	p1, day := int64(1), "2026-11-01"
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	issue := model.Issue{UID: "alpha", ShortID: "aaaa", Title: "A", Status: "open", ProjectName: "work", Author: "ann", CreatedAt: at, UpdatedAt: at, Priority: &p1, ScheduledOnDate: &day}
	links := []model.Link{{ID: 2, Type: "parent", FromIssueUID: "k1", FromStatus: "open", ToIssueUID: "alpha"}}
	nodes := render(t, apply(withZone(browsing()), model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{issue}, Links: links}}},
		model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &issue, Links: links}}))
	props := view.DetailContentID + ".properties"
	wantMono(t, "heading id", nodes[view.DetailContentID+".heading.title"], "#aaaa")
	for _, key := range []string{"created", "updated"} {
		n := nodes[props+"."+key+".v"]
		wantMono(t, key, n, text(n))
	}
	wantMono(t, "children", nodes[props+".children.v"], "1", "1")
	for _, key := range []string{"priority", "scheduled"} {
		if role, _ := nodes[props+"."+key+".v"].P["role"].(string); !strings.Contains(role, view.MonoRole) {
			t.Errorf("%s badge role = %q", key, role)
		}
	}
	if wantAuthor := nodes[props+".author.v"]; len(monoRuns(wantAuthor)) != 0 {
		t.Errorf("the author is mono: %v", spansOf(wantAuthor.P))
	}
}

// Every property key sits in one box the style sheet gives a single width, so the values start at the same x in either face,
// and no key is padded with no-break spaces.
func TestPropertyKeysShareOneBox(t *testing.T) {
	nodes := render(t, browsing())
	props := nodes[view.DetailContentID+".properties"]
	if len(props.C) < 2 {
		t.Fatalf("%d properties", len(props.C))
	}
	for _, row := range props.C {
		key := nodes[row.ID+".k"]
		if role, _ := key.P["role"].(string); role != view.PropertyKeyRole {
			t.Errorf("%s role = %q", row.ID, role)
		}
		if strings.Contains(text(key), "\u00a0") {
			t.Errorf("%s is padded: %q", row.ID, text(key))
		}
	}
	for _, rule := range []string{"[data-role~='" + view.PropertyKeyRole + "']", "[data-role~='" + view.MonoRole + "']"} {
		if !strings.Contains(view.CSS, rule) {
			t.Errorf("CSS lacks a rule for %s", rule)
		}
	}
}

// The header's labels over the mono cells are mono as well, so a column's header and rows share one box width in px.
func TestListHeaderLabelsOverMonoCellsAreMono(t *testing.T) {
	header := render(t, browsing())["main.columns.list.header"]
	var monos []string
	for _, run := range styled(header) {
		if slices.Contains(strings.Fields(run[1]), "mono") {
			monos = append(monos, strings.TrimSpace(run[0]))
		}
	}
	if want := []string{"#", "prio", "kids", "updated"}; !slices.Equal(monos, want) {
		t.Errorf("mono header labels %q, want %q", monos, want)
	}
}
