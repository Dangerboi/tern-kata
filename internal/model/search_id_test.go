package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

var (
	idProject    = model.Project{ID: 7, UID: "p", Name: "work"}
	idCollection = model.Collection{View: "project", ProjectUID: "p", Title: "work"}
	idDeadlines  = model.Collection{View: "deadlines", Title: "Deadlines"}
)

func idIssues(uids ...string) []model.Issue {
	issues := make([]model.Issue, len(uids))
	for i, uid := range uids {
		issues[i] = model.Issue{UID: uid, ShortID: uid, Title: "Issue " + uid, Status: "open"}
	}
	return issues
}

func idSnapshot(uids ...string) g.UISnapshotResponseBody {
	return g.UISnapshotResponseBody{Catalog: []g.UIProject{{Project: idProject}}, Collection: idIssues(uids...)}
}

// idSearching is a Pane browsing collection with a, b and c listed and `typed` all but its last character in the search box.
func idSearching(collection model.Collection, typed string) model.Model {
	m := model.Model{Workspace: "/repo", Project: idProject, Collection: collection, Bound: true}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: idSnapshot("a", "b", "c")})
	m.Filters.Search = typed[:len(typed)-1]
	m, _ = model.Update(m, model.KeyMsg{Chord: "/"})
	return m
}

// cmdOf is the one command of type T among cmds (detail loads and scrolls come with any change of selection).
func cmdOf[T model.Cmd](t *testing.T, cmds []model.Cmd) T {
	t.Helper()
	var found []T
	for _, cmd := range cmds {
		if cmd, ok := cmd.(T); ok {
			found = append(found, cmd)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one %T, got %#v", *new(T), cmds)
	}
	return found[0]
}

func listed(m model.Model) []string {
	var uids []string
	for _, row := range m.VisibleRows() {
		uids = append(uids, row.UID)
	}
	return uids
}

// found finishes the lookup for lookup with uid ("" for none) and the snapshot that follows it with uids.
func found(t *testing.T, m model.Model, lookup model.LookupSearchID, uid string, uids ...string) (model.Model, model.ReadSnapshot) {
	t.Helper()
	m, cmds := model.Update(m, model.SearchIDLooked{Request: lookup.Request, ShortID: lookup.ShortID, UID: uid})
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot(uids...)})
	return m, refresh
}

func TestSearchByID(t *testing.T) {
	for _, typed := range []string{"0zpn", "#0zpn", "0ZPN"} {
		t.Run(typed, func(t *testing.T) {
			m := idSearching(idCollection, typed)
			m, cmds := model.Update(m, model.KeyMsg{Text: typed[len(typed)-1:]})
			lookup := cmdOf[model.LookupSearchID](t, cmds)
			if want := (model.LookupSearchID{Workspace: "/repo", ProjectID: 7, ShortID: "0zpn", Request: m.RefreshRequest}); lookup != want {
				t.Fatalf("lookup: %+v, want %+v", lookup, want)
			}
			if !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
				t.Fatalf("list changed before the lookup answered: %v", listed(m))
			}
			m, refresh := found(t, m, lookup, "b", "a", "b", "c")
			if refresh.Query.Text != nil {
				t.Fatalf("found id still asks for full text: %q", *refresh.Query.Text)
			}
			if !reflect.DeepEqual(listed(m), []string{"b"}) || m.SelectedUID != "b" || m.Filters.Search != typed {
				t.Fatalf("id result: rows %v, selected %q, search %q", listed(m), m.SelectedUID, m.Filters.Search)
			}
		})
	}
}

func TestSearchThatIsNotAnIDRunsFullText(t *testing.T) {
	for _, typed := range []string{"Search target", "abc", "0zpl", "0zpn link"} {
		t.Run(typed, func(t *testing.T) {
			m := idSearching(idCollection, typed)
			_, cmds := model.Update(m, model.KeyMsg{Text: typed[len(typed)-1:]})
			if refresh := cmdOf[model.ReadSnapshot](t, cmds); refresh.Query.Text == nil || *refresh.Query.Text != typed {
				t.Fatalf("full text query: %+v", refresh.Query)
			}
		})
	}
}

func TestUnknownIDRunsFullText(t *testing.T) {
	m := idSearching(idCollection, "0zpn")
	m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
	m, refresh := found(t, m, cmdOf[model.LookupSearchID](t, cmds), "", "a", "c")
	if refresh.Query.Text == nil || *refresh.Query.Text != "0zpn" || !reflect.DeepEqual(listed(m), []string{"a", "c"}) {
		t.Fatalf("unknown id: query %+v, rows %v", refresh.Query, listed(m))
	}
}

func TestTypingPastAnIDReturnsToFullText(t *testing.T) {
	m := idSearching(idCollection, "0zpn")
	m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
	m, _ = found(t, m, cmdOf[model.LookupSearchID](t, cmds), "b", "a", "b", "c")
	for _, key := range []model.KeyMsg{{Chord: "space"}, {Text: "l"}} {
		m, cmds = model.Update(m, key)
		refresh := cmdOf[model.ReadSnapshot](t, cmds)
		m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a", "c")})
	}
	if m.Filters.Search != "0zpn l" || !reflect.DeepEqual(listed(m), []string{"a", "c"}) {
		t.Fatalf("typing past the id: search %q, rows %v", m.Filters.Search, listed(m))
	}
}

func TestStaleIDLookupNeverReplacesNewerQuery(t *testing.T) {
	m := idSearching(idCollection, "0zpn")
	m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
	lookup := cmdOf[model.LookupSearchID](t, cmds)
	for _, key := range []model.KeyMsg{{Chord: "space"}, {Text: "l"}} {
		if m, cmds = model.Update(m, key); len(cmds) != 0 {
			t.Fatalf("parallel request instead of coalescing: %#v", cmds)
		}
	}
	superseded := lookup
	superseded.Request--
	if m, cmds = model.Update(m, model.SearchIDLooked{Request: superseded.Request, ShortID: "0zpn", UID: "b"}); len(cmds) != 0 {
		t.Fatalf("a superseded lookup was answered: %#v", cmds)
	}
	m, cmds = model.Update(m, model.SearchIDLooked{Request: lookup.Request, ShortID: lookup.ShortID, UID: "b"})
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	if refresh.Query.Text == nil || *refresh.Query.Text != "0zpn l" || !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
		t.Fatalf("stale lookup: query %+v, rows %v", refresh.Query, listed(m))
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a")})
	if !reflect.DeepEqual(listed(m), []string{"a"}) {
		t.Fatalf("newer query's results: %v", listed(m))
	}
}

func TestFullTextResultCoalescedOverAnIDQuery(t *testing.T) {
	m := idSearching(idCollection, "abc")
	m, cmds := model.Update(m, model.KeyMsg{Text: "c"})
	stale := cmdOf[model.ReadSnapshot](t, cmds)
	if m, cmds = model.Update(m, model.KeyMsg{Text: "d"}); len(cmds) != 0 {
		t.Fatalf("parallel request instead of coalescing: %#v", cmds)
	}
	m, cmds = model.Update(m, model.SnapshotLoaded{Request: stale.Request, Query: stale.Query, Snapshot: idSnapshot("a")})
	lookup := cmdOf[model.LookupSearchID](t, cmds)
	if lookup.ShortID != "abcd" || !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
		t.Fatalf("superseded full-text result: lookup %+v, rows %v", lookup, listed(m))
	}
}

func TestSearchByIDInCollections(t *testing.T) {
	for _, tt := range []struct {
		name       string
		collection model.Collection
		inside     []string
		want       []string
		fullText   bool
	}{
		{"Project", idCollection, []string{"a", "b", "c"}, []string{"b"}, false},
		{"Project the id's Issue is filtered out of", idCollection, []string{"a", "c"}, []string{"a", "c"}, true},
		{"other Collection holding the Issue", idDeadlines, []string{"b", "c"}, []string{"b"}, false},
		{"other Collection without the Issue", idDeadlines, []string{"a", "c"}, []string{"a", "c"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := idSearching(tt.collection, "0zpn")
			m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
			lookup := cmdOf[model.LookupSearchID](t, cmds)
			if lookup.ProjectID != 7 {
				t.Fatalf("lookup outside the Project: %+v", lookup)
			}
			m, cmds = model.Update(m, model.SearchIDLooked{Request: lookup.Request, ShortID: lookup.ShortID, UID: "b"})
			refresh := cmdOf[model.ReadSnapshot](t, cmds)
			m, cmds = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot(tt.inside...)})
			if tt.fullText {
				refresh = cmdOf[model.ReadSnapshot](t, cmds)
				if refresh.Query.Text == nil || *refresh.Query.Text != "0zpn" {
					t.Fatalf("an id outside the Collection did not fall back to full text: %+v", refresh.Query)
				}
				m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot(tt.want...)})
			}
			if !reflect.DeepEqual(listed(m), tt.want) {
				t.Fatalf("rows %v, want %v", listed(m), tt.want)
			}
		})
	}
}

func TestIDResultKeepsSearchKeysAndLiveRefresh(t *testing.T) {
	m := idSearching(idCollection, "0zpn")
	m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
	m, _ = found(t, m, cmdOf[model.LookupSearchID](t, cmds), "b", "a", "b", "c")
	m, cmds = model.Update(m, model.StreamEvent{Generation: 1, ID: 1, Type: "issue.updated"})
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	if refresh.Query.Text != nil {
		t.Fatalf("live refresh looked the id up again as full text: %+v", refresh.Query)
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a", "b", "c")})
	if !reflect.DeepEqual(listed(m), []string{"b"}) {
		t.Fatalf("live refresh widened the id result: %v", listed(m))
	}
	for _, step := range []struct {
		chord string
		mode  model.Mode
	}{{"down", model.ModeSearchResults}, {"escape", model.ModeSearch}, {"enter", model.ModeSearchResults}, {"enter", model.ModeList}} {
		m, cmds = model.Update(m, model.KeyMsg{Chord: step.chord})
		if m.CurrentMode() != step.mode || m.SelectedUID != "b" || m.Filters.Search != "0zpn" || !reflect.DeepEqual(listed(m), []string{"b"}) {
			t.Fatalf("%s: mode %s, selected %q, search %q, rows %v, cmds %#v", step.chord, m.CurrentMode(), m.SelectedUID, m.Filters.Search, listed(m), cmds)
		}
	}
}

func TestCollectionSwitchLooksTheIDUpAgain(t *testing.T) {
	m := idSearching(idCollection, "0zpn")
	m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
	m, _ = found(t, m, cmdOf[model.LookupSearchID](t, cmds), "b", "a", "b", "c")
	m, cmds = model.Update(m, model.CollectionSelected{Collection: idDeadlines})
	m, _ = found(t, m, cmdOf[model.LookupSearchID](t, cmds), "b", "b", "c")
	if !reflect.DeepEqual(listed(m), []string{"b"}) {
		t.Fatalf("id result after switching Collection: %v", listed(m))
	}
}

func TestRetypedIDIsLookedUpAgain(t *testing.T) {
	m := idSearching(idCollection, "0zpn")
	m, cmds := model.Update(m, model.KeyMsg{Text: "n"})
	m, _ = found(t, m, cmdOf[model.LookupSearchID](t, cmds), "", "a")
	m, cmds = model.Update(m, model.KeyMsg{Chord: "space"})
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a")})
	_, cmds = model.Update(m, model.KeyMsg{Chord: "backspace"})
	if lookup := cmdOf[model.LookupSearchID](t, cmds); lookup.ShortID != "0zpn" {
		t.Fatalf("retyped id: %+v", lookup)
	}
}
