package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func TestLiveSearch(t *testing.T) {
	for _, tt := range []struct {
		name  string
		keys  []string
		mode  model.Mode
		query string
		uid   string
	}{
		{"typing requests live results", []string{"/", "q"}, model.ModeSearch, "q", "a"},
		{"arrow enters results before commit", []string{"/", "q", "down"}, model.ModeSearchResults, "q", "b"},
		{"result letters are swallowed", []string{"/", "q", "down", "j"}, model.ModeSearchResults, "q", "b"},
		{"enter enters results", []string{"/", "q", "enter"}, model.ModeSearchResults, "q", "a"},
		{"result enter commits", []string{"/", "q", "down", "enter"}, model.ModeList, "q", "b"},
		{"escape returns to query", []string{"/", "q", "down", "escape"}, model.ModeSearch, "q", "b"},
		{"query escape cancels and restores selection", []string{"/", "q", "down", "escape", "escape"}, model.ModeList, "", "b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Model{Collection: model.Collection{View: "project", ProjectUID: "p", Title: "Work"}, SelectedUID: "b"}
			m, _ = model.Update(m, model.CollectionLoaded{Collection: m.Collection, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "a", Title: "Alpha", Status: "open"}, {UID: "b", Title: "Beta", Status: "open"}}}, ETag: "original"})
			for _, chord := range tt.keys {
				var cmds []model.Cmd
				m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
				for _, cmd := range cmds {
					if detail, ok := cmd.(model.LoadDetail); ok {
						if detail.UID != m.SelectedUID {
							t.Fatalf("wrong result detail: %+v", detail)
						}
						continue
					}
					refresh, ok := cmd.(model.RefreshSnapshot)
					query := refresh.Query
					query.SelectedIssueUID, query.IncludeHistory = nil, nil
					if !ok || !reflect.DeepEqual(query, m.SnapshotQuery()) || refresh.ETag != "" {
						t.Fatalf("search command: %#v", cmds)
					}
					m, _ = model.Update(m, model.SnapshotRefreshed{Request: refresh.Request, Query: refresh.Query, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "a", Title: "Alpha", Status: "open"}, {UID: "b", Title: "Beta", Status: "open"}}}})
				}
			}
			if m.CurrentMode() != tt.mode || m.Filters.Search != tt.query || m.SelectedUID != tt.uid || m.Quitting {
				t.Fatalf("search: mode %s, filters %+v, uid %s, quitting %v", m.CurrentMode(), m.Filters, m.SelectedUID, m.Quitting)
			}
		})
	}
}

func TestStatusFilterCycle(t *testing.T) {
	m := model.Model{Collection: model.Collection{View: "all-open", Title: "All Open"}}
	for _, tt := range []struct {
		status, header string
	}{
		{"open", "All Open · status=open"},
		{"closed", "All Open · status=closed"},
		{"all", "All Open · status=all"},
		{"open", "All Open · status=open"},
	} {
		var cmds []model.Cmd
		m, cmds = model.Update(m, model.KeyMsg{Chord: "s"})
		if len(cmds) != 1 || m.Filters.Status != tt.status || m.ListHeader() != tt.header {
			t.Fatalf("status cycle: %+v, header %q, commands %#v", m.Filters, m.ListHeader(), cmds)
		}
		refresh := cmds[0].(model.RefreshSnapshot)
		want := []string(nil)
		if tt.status != "" {
			want = []string{tt.status}
		}
		if !reflect.DeepEqual(refresh.Query.Status, want) || *refresh.Query.View != "all-open" {
			t.Fatalf("status query: %+v", refresh.Query)
		}
		m, _ = model.Update(m, model.SnapshotRefreshed{Request: refresh.Request, Query: refresh.Query, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "a", Status: tt.status}}}})
		if tt.status == "closed" && (len(m.VisibleRows()) != 1 || m.VisibleRows()[0].Status != "closed") {
			t.Fatalf("Collection regrouping dropped status-filter match: %+v", m.Issues)
		}
	}
}

func TestFilterForm(t *testing.T) {
	for _, tt := range []struct {
		name    string
		ending  string
		applied bool
	}{
		{"apply", "ctrl+o", true},
		{"save alias", "ctrl+s", true},
		{"cancel", "escape", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Model{Collection: model.Collection{View: "today", Title: "Today"}, LocalDate: "2026-10-08", TimeZone: "UTC", Filters: model.Filters{Search: "existing"}, Snapshot: g.UISnapshotResponseBody{Catalog: []g.UIProject{{Project: model.Project{UID: "p", Name: "Work"}}}}}
			keys := []string{"f", "right", "tab", "right", "tab", "q", "tab", "b", "u", "g", ",", "u", "x", "tab", "right", tt.ending}
			var cmds []model.Cmd
			for _, chord := range keys {
				m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
			}
			if m.CurrentMode() != model.ModeList || m.Quitting {
				t.Fatalf("form exit: %+v", m)
			}
			if !tt.applied {
				if len(cmds) != 0 || m.Filters != (model.Filters{Search: "existing"}) {
					t.Fatalf("cancel changed filters: %+v %#v", m.Filters, cmds)
				}
				return
			}
			want := model.Filters{ProjectUID: "p", Status: "open", Owner: "q", Label: "bug,ux", Relationship: "parent", Search: "existing"}
			if m.Filters != want || len(cmds) != 1 || m.ListHeader() != "Today · project=Work · status=open · owner=q · label=bug,ux · relationship=parent · search=existing" {
				t.Fatalf("form apply: %+v %q %#v", m.Filters, m.ListHeader(), cmds)
			}
			refresh := cmds[0].(model.RefreshSnapshot)
			q := refresh.Query
			if *q.View != "today" || *q.ProjectUID != "p" || *q.LocalDate != "2026-10-08" || *q.TimeZone != "UTC" || !reflect.DeepEqual(q.Status, []string{"open"}) || !reflect.DeepEqual(q.Owner, []string{"q"}) || !reflect.DeepEqual(q.Label, []string{"bug", "ux"}) || !reflect.DeepEqual(q.Relationship, []g.ReadUISnapshotQueryRelationship{"parent"}) || *q.Text != "existing" {
				t.Fatalf("form query: %+v", q)
			}
		})
	}
}

func TestFiltersSurviveRefreshAndClear(t *testing.T) {
	filters := model.Filters{ProjectUID: "p", Status: "closed", Owner: "alex", Label: "bug", Relationship: "related", Search: "fix"}
	collection := model.Collection{View: "all-open", Title: "All Open"}
	m := model.Model{Collection: collection, Filters: filters, SelectedUID: "a"}
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.RefreshSnapshot)
	query := refresh.Query
	query.SelectedIssueUID, query.IncludeHistory = nil, nil
	if !reflect.DeepEqual(query, m.SnapshotQuery()) || *refresh.Query.ProjectUID != "p" || *refresh.Query.Text != "fix" {
		t.Fatalf("live refresh loses filter intent: %+v", refresh.Query)
	}
	m, _ = model.Update(m, model.SnapshotRefreshed{Request: refresh.Request, Query: refresh.Query, Snapshot: g.UISnapshotResponseBody{Cursor: 1, Collection: []model.Issue{{UID: "a", Status: "closed"}}}})
	if m.Filters != filters || m.Collection != collection || m.SelectedUID != "a" || len(m.VisibleRows()) != 1 {
		t.Fatalf("live refresh loses filters/results: %+v", m)
	}
	m, cmds = model.Update(m, model.KeyMsg{Chord: "c"})
	if m.Filters != (model.Filters{}) || m.ListHeader() != "All Open" || len(cmds) != 1 {
		t.Fatalf("clear: %+v %#v", m.Filters, cmds)
	}
	refresh = cmds[0].(model.RefreshSnapshot)
	if refresh.Query.ProjectUID != nil || refresh.Query.Text != nil || len(refresh.Query.Status)+len(refresh.Query.Owner)+len(refresh.Query.Label)+len(refresh.Query.Relationship) != 0 || refresh.ETag != "" {
		t.Fatalf("clear query retained filter: %+v", refresh)
	}
}

func TestSearchCoalescesSupersededQueries(t *testing.T) {
	m := model.Model{Collection: model.Collection{View: "project", ProjectUID: "p"}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "/"})
	m, cmds := model.Update(m, model.KeyMsg{Text: "a"})
	first := cmds[0].(model.RefreshSnapshot)
	m, cmds = model.Update(m, model.KeyMsg{Text: "b"})
	if len(cmds) != 0 {
		t.Fatal("parallel refresh instead of coalescing")
	}
	m, cmds = model.Update(m, model.SnapshotRefreshed{Request: first.Request, Query: first.Query, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "stale"}}}})
	if len(m.Issues) != 0 || len(cmds) != 1 {
		t.Fatalf("superseded result applied: %+v %#v", m.Issues, cmds)
	}
	current := cmds[0].(model.RefreshSnapshot)
	if *current.Query.Text != "ab" {
		t.Fatalf("coalesced query: %+v", current.Query)
	}
}

func TestCollectionSwitchKeepsFilterIntent(t *testing.T) {
	collection := model.Collection{View: "today", Title: "Today"}
	filters := model.Filters{Owner: "alex", Search: "fix"}
	m := model.Model{Collection: model.Collection{View: "project", ProjectUID: "p"}, Collections: []model.Collection{collection}, Filters: filters}
	m, cmds := model.Update(m, model.CollectionSelected{Collection: collection})
	if len(cmds) != 1 || cmds[0].(model.LoadCollection).Filters != filters {
		t.Fatalf("Collection query loses filters: %#v", cmds)
	}
	m, _ = model.Update(m, model.CollectionLoaded{Collection: collection, Filters: filters, LocalDate: "2026-10-08", TimeZone: "UTC", Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "a", Status: "open", ScheduledOnDate: new("2026-10-08")}}}})
	if m.Filters != filters || m.SelectedUID != "a" || m.Loading {
		t.Fatalf("Collection load loses filters: %+v", m)
	}
	m, _ = model.Update(m, model.CollectionLoaded{Collection: collection, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "stale"}}}})
	if m.SelectedUID != "a" {
		t.Fatal("superseded unfiltered Collection load applied")
	}
}

func TestFilterFormInputAndChoices(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		field                      int
		keys                       []string
		owner, label, relationship string
		endField                   int
	}{
		{"space is text, arrows do not insert", 2, []string{"q", "space", "x", "right", "left"}, "q x", "", "", 2},
		{"unicode backspace and paste", 3, []string{"λ", "backspace"}, "", "", "", 3},
		{"relationship choices wrap backwards", 4, []string{"left"}, "", "", "related", 4},
		{"tab wraps", 4, []string{"tab"}, "", "", "", 0},
		{"backtab wraps", 0, []string{"shift+tab"}, "", "", "", 4},
		{"reset does not apply", 2, []string{"q", "ctrl+r"}, "", "", "", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Model{}
			m, _ = model.Update(m, model.KeyMsg{Chord: "f"})
			m, _ = model.Update(m, model.FilterFieldMsg{Field: tt.field})
			for _, key := range tt.keys {
				m, _ = model.Update(m, model.KeyMsg{Chord: key})
			}
			form := m.FilterForm
			if form.Field != tt.endField || form.Values.Owner != tt.owner || form.Values.Label != tt.label || form.Values.Relationship != tt.relationship || m.Filters != (model.Filters{}) {
				t.Fatalf("form field input: %+v", form)
			}
		})
	}
}

func TestFilterFormActionsAndProjectScope(t *testing.T) {
	m := model.Model{Collection: model.Collection{View: "project", ProjectUID: "other", Title: "Other"}, Filters: model.Filters{ProjectUID: "p"}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "f"})
	m, cmds := model.Update(m, model.FilterFormActionMsg{Action: model.ApplyFilters})
	if m.FilterForm.Open || len(cmds) != 1 || *cmds[0].(model.RefreshSnapshot).Query.ProjectUID != "other" {
		t.Fatalf("filter form apply action: %+v %#v", m, cmds)
	}
	refresh := cmds[0].(model.RefreshSnapshot)
	m, _ = model.Update(m, model.SnapshotRefreshed{Request: refresh.Request, Query: refresh.Query, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "a", Status: "open", ProjectUID: new("other")}}}})
	if len(m.VisibleRows()) != 0 {
		t.Fatal("Project filter widened current Project Collection")
	}
}
