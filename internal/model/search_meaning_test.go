package model_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// meaningPane is a Pane browsing collection with a, b and c listed (filters set on it) and the search box open on `typed`.
func meaningPane(collection model.Collection, filters model.Filters, typed string) model.Model {
	m := model.Model{Workspace: "/repo", Project: idProject, Collection: collection, Bound: true, Filters: filters}
	m = model.WithSnapshotRead(m)
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest(), Query: m.SnapshotQuery(), Snapshot: idSnapshot("a", "b", "c")})
	m.Filters.Search = typed
	m, _ = model.Update(m, model.KeyMsg{Chord: "/"})
	return m
}

// meaningOn is ctrl+t in the search box, with the snapshot read it asks for answered, so later changes ask for their own.
func meaningOn(t *testing.T, m model.Model) model.Model {
	t.Helper()
	m, cmds := press(m, "ctrl+t")
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a", "b", "c")})
	return m
}

func meaningTyping(m model.Model, text string) (model.Model, []model.Cmd) {
	var all []model.Cmd
	for _, r := range text {
		key := model.KeyMsg{Text: string(r)}
		if r == ' ' {
			key = model.KeyMsg{Chord: "space"}
		}
		var cmds []model.Cmd
		m, cmds = model.Update(m, key)
		all = append(all, cmds...)
	}
	return m, all
}

// searched is a Pane in the Project, by meaning, with `query` typed and enter pressed: the search is out.
func searched(t *testing.T, filters model.Filters, query string) (model.Model, model.SearchByMeaning) {
	t.Helper()
	m := meaningPane(idCollection, filters, "")
	m = meaningOn(t, m)
	m, _ = meaningTyping(m, query)
	m, cmds := press(m, "enter")
	return m, cmdOf[model.SearchByMeaning](t, cmds)
}

// answered delivers kata's answer to search and the snapshot read that follows it.
func answered(t *testing.T, m model.Model, search model.SearchByMeaning, done model.SearchByMeaningDone) model.Model {
	t.Helper()
	done.Request, done.Query = search.Request, search.Query
	m, cmds := model.Update(m, done)
	for _, cmd := range cmds {
		if refresh, ok := cmd.(model.ReadSnapshot); ok {
			m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a", "b", "c")})
		}
	}
	return m
}

// ranking is kata's answer holding uids, in its order.
func ranking(uids ...string) model.SearchByMeaningDone {
	return model.SearchByMeaningDone{RankedAnswer: model.RankedAnswer{UIDs: uids}}
}

func TestCtrlTSwitchesBetweenWordsAndMeaning(t *testing.T) {
	for _, toggle := range []string{"ctrl+t", "meta+t"} {
		t.Run(toggle, func(t *testing.T) {
			m := meaningPane(idCollection, model.Filters{}, "q")
			label := func(m model.Model) string {
				b, ok := model.LookupBindingForMode("ctrl+t", m.CurrentMode())
				if !ok {
					t.Fatalf("ctrl+t is not bound in %s", m.CurrentMode())
				}
				return b.Described(m)
			}
			if label(m) != "by meaning" || m.Search.Meaning {
				t.Fatalf("fresh search: meaning %v, help %q", m.Search.Meaning, label(m))
			}
			m, _ = press(m, toggle)
			if !m.Search.Meaning || label(m) != "by words" || m.Filters.Search != "q" {
				t.Fatalf("by meaning: %v, help %q, query %q", m.Search.Meaning, label(m), m.Filters.Search)
			}
			m, cmds := press(m, "down")
			if m.CurrentMode() != model.ModeSearchResults || len(cmds) > 0 && !onlyReads(cmds) {
				t.Fatalf("down: mode %s, %#v", m.CurrentMode(), cmds)
			}
			m, _ = press(m, toggle)
			if m.Search.Meaning || m.CurrentMode() != model.ModeSearchResults || label(m) != "by meaning" {
				t.Fatalf("back to words in the results: %v, mode %s", m.Search.Meaning, m.CurrentMode())
			}
		})
	}
}

// onlyReads says cmds are the reads any selection change brings, never a search by meaning.
func onlyReads(cmds []model.Cmd) bool {
	for _, cmd := range cmds {
		if _, search := cmd.(model.SearchByMeaning); search {
			return false
		}
	}
	return true
}

func TestHelpSheetAndSearchFooterListCtrlT(t *testing.T) {
	var found bool
	for _, group := range model.HelpSheetOn(model.PageIssues, model.Model{}) {
		for _, line := range group.Lines {
			if line.Text == "by meaning" && slices.Contains(line.Keys, "ctrl+t") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the help sheet has no ctrl+t line for search by meaning")
	}
	var footer bool
	for binding := range model.BindingsForMode(model.ModeSearch, true) {
		footer = footer || binding.Chord == "ctrl+t"
	}
	if !footer {
		t.Fatal("the search footer does not offer ctrl+t")
	}
}

func TestTypingByMeaningSendsNothingAndKeepsTheList(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{}, "")
	m = meaningOn(t, m)
	m, cmds := meaningTyping(m, "slow login")
	if len(cmds) != 0 {
		t.Fatalf("typing sent %#v", cmds)
	}
	m, cmds = press(m, "backspace")
	if len(cmds) != 0 || m.Filters.Search != "slow logi" || !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
		t.Fatalf("erasing: %#v, query %q, rows %v", cmds, m.Filters.Search, listed(m))
	}
}

func TestEnterByMeaningSendsOneSearchWithTheStatusAndLabelFilters(t *testing.T) {
	m, search := searched(t, model.Filters{Status: "open", Label: "bug, ui", Owner: "alice"}, "slow login")
	want := model.SearchByMeaning{Workspace: "/repo", ProjectID: 7, Query: "slow login", Status: "open", Labels: []string{"bug", "ui"}, Request: search.Request}
	if !reflect.DeepEqual(search, want) {
		t.Fatalf("search %+v, want %+v", search, want)
	}
	if m.CurrentMode() != model.ModeSearchResults {
		t.Fatalf("mode %s", m.CurrentMode())
	}
	if _, cmds := press(m, "enter"); !onlyReads(cmds) {
		t.Fatalf("committing ran another search: %#v", cmds)
	}
}

func TestEnterByWordsSendsNoSearch(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{}, "")
	m, _ = meaningTyping(m, "slow")
	if _, cmds := press(m, "enter"); !onlyReads(cmds) {
		t.Fatalf("by words ran a search by meaning: %#v", cmds)
	}
}

func TestMeaningResultsListInKatasOrderUnderResults(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "gone", "a"))
	if want := []string{"c", "a"}; !reflect.DeepEqual(listed(m), want) {
		t.Fatalf("rows %v, want %v", listed(m), want)
	}
	if len(m.Groups) != 1 || m.Groups[0].ID != "results" || m.Groups[0].Title != "Results" {
		t.Fatalf("groups %+v", m.Groups)
	}
	if m.SearchLine() != "" {
		t.Fatalf("search line %q", m.SearchLine())
	}
	m, _ = press(m, "enter")
	if m.CurrentMode() != model.ModeList || !reflect.DeepEqual(listed(m), []string{"c", "a"}) {
		t.Fatalf("committed: mode %s, rows %v", m.CurrentMode(), listed(m))
	}
}

func TestMeaningResultsAreFlatWhenIssuesNest(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m.Snapshot.Links = []model.Link{{Type: "parent", FromIssueUID: "b", ToIssueUID: "a"}}
	if m, _ = model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query, UIDs: []string{"b", "a"}}}); !m.Ranked.Have {
		t.Fatal("the answer was not taken")
	}
	if want := []string{"b", "a"}; !reflect.DeepEqual(listed(m), want) {
		t.Fatalf("rows %v, want %v", listed(m), want)
	}
}

func TestSearchLineNamesWhatKataSaid(t *testing.T) {
	for _, tt := range []struct {
		name string
		done model.SearchByMeaningDone
		want string
	}{
		{"words with kata's reason", model.SearchByMeaningDone{RankedAnswer: model.RankedAnswer{UIDs: []string{"a"}, Lexical: true, Reason: "embedding model is loading"}}, "kata searched by words: embedding model is loading"},
		{"words without a reason", model.SearchByMeaningDone{RankedAnswer: model.RankedAnswer{UIDs: []string{"a"}, Lexical: true}}, "kata searched by words: turn on search by meaning with `kata daemon local-embeddings`"},
		{"truncated", model.SearchByMeaningDone{RankedAnswer: model.RankedAnswer{UIDs: []string{"a"}, Truncated: true}}, "showing the best 50"},
		{"hybrid and complete", ranking("a"), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, search := searched(t, model.Filters{}, "slow login")
			m = answered(t, m, search, tt.done)
			if got := m.SearchLine(); got != tt.want {
				t.Fatalf("search line %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(listed(m), []string{"a"}) {
				t.Fatalf("rows %v", listed(m))
			}
		})
	}
}

func TestOwnerAndRelationshipFiltersDoNotApplyByMeaning(t *testing.T) {
	m, search := searched(t, model.Filters{Owner: "alice"}, "slow login")
	if got := m.SearchLine(); got != "the Owner filter doesn't apply to search by meaning" {
		t.Fatalf("search line %q", got)
	}
	m = answered(t, m, search, ranking("b"))
	if refresh := m.SnapshotQuery(); refresh.Owner != nil || refresh.Text != nil {
		t.Fatalf("snapshot query %+v still carries the Owner filter or the text", refresh)
	}
	m, _ = searched(t, model.Filters{Owner: "alice", Relationship: "blocked"}, "x")
	if got := m.SearchLine(); got != "the Owner and Relationship filters don't apply to search by meaning" {
		t.Fatalf("search line %q", got)
	}
}

func TestFailedSearchByMeaningShowsKatasErrorAndKeepsTheRows(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "a"))
	m, _ = press(m, "escape")
	m, again := press(m, "enter")
	search = cmdOf[model.SearchByMeaning](t, again)
	m, _ = model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query}, Err: errors.New("search is not available")})
	if got := m.SearchLine(); got != "search is not available" {
		t.Fatalf("search line %q", got)
	}
	if want := []string{"c", "a"}; !reflect.DeepEqual(listed(m), want) {
		t.Fatalf("rows %v, want the last results %v", listed(m), want)
	}
}

func TestEnterAfterAFailedSearchByMeaningCommitsTheSearch(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m, _ = model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query}, Err: errors.New("search is not available")})
	m, cmds := press(m, "enter")
	for _, cmd := range cmds {
		if _, ok := cmd.(model.SearchByMeaning); ok {
			t.Fatalf("enter after a failed answer searched again: %#v", cmds)
		}
	}
	if m.CurrentMode() != model.ModeList || m.Search.Open {
		t.Fatalf("the search stayed open: mode %s", m.CurrentMode())
	}
}

func TestLiveEventsDoNotRerunSearchByMeaning(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "a"))
	m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.updated"})
	if !onlyReads(cmds) {
		t.Fatalf("a live event ran a search: %#v", cmds)
	}
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: idSnapshot("a", "b", "c")})
	if want := []string{"c", "a"}; !reflect.DeepEqual(listed(m), want) {
		t.Fatalf("rows after the event %v, want %v", listed(m), want)
	}
}

func TestSearchByMeaningNeedsOneProject(t *testing.T) {
	m := meaningPane(idDeadlines, model.Filters{}, "")
	m, cmds := press(m, "ctrl+t")
	if m.Search.Meaning || len(cmds) != 0 || m.SearchLine() != "search by meaning works inside one Project" {
		t.Fatalf("outside a Project: meaning %v, %#v, line %q", m.Search.Meaning, cmds, m.SearchLine())
	}
	m, cmds = meaningTyping(m, "q")
	if refresh := cmdOf[model.ReadSnapshot](t, cmds); refresh.Query.Text == nil || *refresh.Query.Text != "q" {
		t.Fatalf("the search stayed by words: %+v", refresh.Query)
	}
	if m.SearchLine() != "" {
		t.Fatalf("the refusal outlived the next key: %q", m.SearchLine())
	}
	if _, cmds = press(m, "enter"); !onlyReads(cmds) {
		t.Fatalf("enter ran a search by meaning: %#v", cmds)
	}
}

func TestAViewScopedToOneProjectSearchesByMeaning(t *testing.T) {
	all := model.Collection{View: "all-open", Title: "All Open"}
	m := meaningPane(all, model.Filters{ProjectUID: "p"}, "slow")
	m, _ = press(m, "ctrl+t", "enter")
	if !m.Search.Meaning {
		t.Fatalf("a view scoped to a Project refused: %q", m.SearchLine())
	}
	m = meaningPane(all, model.Filters{}, "")
	if m, _ = press(m, "ctrl+t"); m.Search.Meaning {
		t.Fatal("All Open searched by meaning without a Project")
	}
}

func TestEscapeOutOfTheQueryResetsToWords(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "a"))
	m, _ = press(m, "escape", "escape")
	if m.CurrentMode() != model.ModeList || m.Filters.Search != "" || m.Search.Meaning || !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
		t.Fatalf("after esc: mode %s, query %q, rows %v", m.CurrentMode(), m.Filters.Search, listed(m))
	}
	if m, _ = press(m, "/"); m.Search.Meaning {
		t.Fatal("the next search started by meaning")
	}
}

func TestCommittedMeaningResultsStayUntilTheSearchIsCleared(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "a"))
	m, _ = press(m, "enter")
	if !reflect.DeepEqual(listed(m), []string{"c", "a"}) {
		t.Fatalf("committed rows %v", listed(m))
	}
	m, _ = press(m, "/", "escape")
	if !reflect.DeepEqual(listed(m), []string{"c", "a"}) || m.Filters.Search != "slow login" {
		t.Fatalf("esc out of a reopened search: rows %v, query %q", listed(m), m.Filters.Search)
	}
	m, _ = press(m, "c")
	if !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) || m.Filters.Search != "" {
		t.Fatalf("cleared: rows %v, query %q", listed(m), m.Filters.Search)
	}
}

func TestSwitchingBackToWordsDropsTheRankedList(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "a"))
	m, cmds := press(m, "ctrl+t")
	refresh := cmdOf[model.ReadSnapshot](t, cmds)
	if refresh.Query.Text == nil || *refresh.Query.Text != "slow login" || m.Ranked.Have {
		t.Fatalf("by words again: %+v, ranked %+v", refresh.Query, m.Ranked)
	}
}

func TestCollectionSwitchEndsTheSearchByMeaning(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m = answered(t, m, search, ranking("c", "a"))
	m, _ = press(m, "enter", "/")
	m.Collections = []model.Collection{idDeadlines}
	m, _ = model.Update(m, model.CollectionSelected{Collection: idDeadlines})
	if m.Search.Meaning || m.Search.InitialRanked.Have || m.Ranked.Have {
		t.Fatalf("after the switch: search %+v, ranked %+v", m.Search, m.Ranked)
	}
	m, _ = press(m, "escape")
	if m.Ranked.Have || m.Search.Meaning {
		t.Fatalf("esc brought the old Collection's answer back: ranked %+v", m.Ranked)
	}
	if query := m.SnapshotQuery(); query.Text == nil || *query.Text != "slow login" {
		t.Fatalf("snapshot query %+v lost the search text", query)
	}
}

func TestEnterByMeaningAfterMovingIntoTheResultsRunsTheSearch(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{}, "")
	m = meaningOn(t, m)
	m, _ = meaningTyping(m, "foo")
	m, cmds := press(m, "down")
	if !onlyReads(cmds) {
		t.Fatalf("down ran a search: %#v", cmds)
	}
	m, cmds = press(m, "enter")
	search := cmdOf[model.SearchByMeaning](t, cmds)
	if search.Query != "foo" || m.CurrentMode() != model.ModeSearchResults {
		t.Fatalf("search %+v, mode %s; want the search for foo and the box still open", search, m.CurrentMode())
	}
	m = answered(t, m, search, ranking("c", "a"))
	m, cmds = press(m, "enter")
	if !onlyReads(cmds) || m.CurrentMode() != model.ModeList || !reflect.DeepEqual(listed(m), []string{"c", "a"}) {
		t.Fatalf("commit: mode %s, rows %v, %#v", m.CurrentMode(), listed(m), cmds)
	}
}

func TestAnAnswerAfterEscapeIsDropped(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m, _ = press(m, "escape", "escape")
	m, _ = model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query, UIDs: []string{"c"}}})
	if m.Ranked.Have || !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
		t.Fatalf("a cancelled search answered: %+v, rows %v", m.Ranked, listed(m))
	}
}

func TestShortIDByMeaningStillJumps(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{}, "")
	m = meaningOn(t, m)
	m, cmds := meaningTyping(m, "4kxn")
	lookup := cmdOf[model.LookupSearchID](t, cmds)
	m, _ = found(t, m, lookup, "b", "a", "b", "c")
	if !reflect.DeepEqual(listed(m), []string{"b"}) || m.SelectedUID != "b" {
		t.Fatalf("id result: rows %v, selected %q", listed(m), m.SelectedUID)
	}
	if _, cmds = press(m, "enter"); !onlyReads(cmds) {
		t.Fatalf("enter on a short id ran a search by meaning: %#v", cmds)
	}
}

// loaded delivers the snapshot reads in cmds, answered with the three Issues.
func loaded(m model.Model, cmds []model.Cmd) model.Model {
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: idSnapshot("a", "b", "c")})
		}
	}
	return m
}

func TestAFilterChangeEndsTheCommittedRankedAnswer(t *testing.T) {
	all := model.Collection{View: "all-open", Title: "All Open"}
	for _, tt := range []struct {
		name      string
		start     model.Filters
		truncated bool
		change    func(*model.Filters)
	}{
		{"status", model.Filters{ProjectUID: "p", Status: "open"}, false, func(f *model.Filters) { f.Status = "closed" }},
		{"status set after a cut answer asked without one", model.Filters{ProjectUID: "p"}, true, func(f *model.Filters) { f.Status = "open" }},
		{"label", model.Filters{ProjectUID: "p"}, false, func(f *model.Filters) { f.Label = "bug" }},
		{"project", model.Filters{ProjectUID: "p"}, false, func(f *model.Filters) { f.ProjectUID = "q" }},
		{"project cleared", model.Filters{ProjectUID: "p"}, false, func(f *model.Filters) { f.ProjectUID = "" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := meaningPane(all, tt.start, "")
			m = meaningOn(t, m)
			m, _ = meaningTyping(m, "login")
			m, cmds := press(m, "enter")
			search := cmdOf[model.SearchByMeaning](t, cmds)
			answer := ranking("c", "a")
			answer.Truncated = tt.truncated
			m = answered(t, m, search, answer)
			m, cmds = press(m, "enter")
			m = loaded(m, cmds)
			if !reflect.DeepEqual(listed(m), []string{"c", "a"}) {
				t.Fatalf("committed rows %v", listed(m))
			}
			tt.change(&m.Filters)
			if m.SnapshotQuery().Text == nil {
				t.Fatal("a ranked answer asked under the old filters still stands")
			}
		})
	}
}

func TestEnterByMeaningOnTheBoardSendsNoStatus(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{Status: "open"}, "")
	m.Page = model.PageBoard
	m, _ = press(m, "ctrl+t")
	m, _ = meaningTyping(m, "login")
	_, cmds := press(m, "enter")
	if search := cmdOf[model.SearchByMeaning](t, cmds); search.Status != "" {
		t.Fatalf("the Board sent status %q, which it ignores", search.Status)
	}
}

// committedOn commits a ranked answer for "login" (c, a) asked on page with the filters given.
func committedOn(t *testing.T, page model.Page, filters model.Filters) model.Model {
	t.Helper()
	m := meaningPane(idCollection, filters, "")
	m.Page = page
	m = meaningOn(t, m)
	m, _ = meaningTyping(m, "login")
	m, cmds := press(m, "enter")
	m = answered(t, m, cmdOf[model.SearchByMeaning](t, cmds), ranking("c", "a"))
	m, cmds = press(m, "enter")
	return loaded(m, cmds)
}

func TestAnAnswerAskedOnTheBoardStaysOnTheIssuesPage(t *testing.T) {
	m := committedOn(t, model.PageBoard, model.Filters{Status: "open"})
	m.Page = model.PageIssues
	if m.SnapshotQuery().Text != nil {
		t.Fatal("an uncut answer asked without a Status ended when the Issues Page applied its Status")
	}
}

func TestAnAnswerAskedWithAStatusEndsWhenTheBoardOpens(t *testing.T) {
	m := committedOn(t, model.PageIssues, model.Filters{Status: "closed"})
	m.Page = model.PageBoard
	if m.SnapshotQuery().Text == nil {
		t.Fatal("the Board, which ignores Status, kept an answer kata narrowed to closed Issues")
	}
}

func TestEscapeOutOfAnEditedReopenedSearchKeepsTheCommittedAnswer(t *testing.T) {
	for _, tt := range []struct {
		name string
		done model.SearchByMeaningDone
	}{
		{"answered", ranking("b")},
		{"failed", model.SearchByMeaningDone{Err: errors.New("search is not available")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, search := searched(t, model.Filters{}, "foo")
			m = answered(t, m, search, ranking("c", "a"))
			m, cmds := press(m, "enter")
			m = loaded(m, cmds)
			m, _ = press(m, "/")
			if !m.Search.Meaning {
				t.Fatal("the reopened search is by words")
			}
			m, _ = meaningTyping(m, " bar")
			m, cmds = press(m, "enter")
			again := cmdOf[model.SearchByMeaning](t, cmds)
			tt.done.Request, tt.done.Query = again.Request, again.Query
			m, cmds = model.Update(m, tt.done)
			m = loaded(m, cmds)
			m, cmds = press(m, "escape", "escape")
			m = loaded(m, cmds)
			if m.Filters.Search != "foo" || !m.Ranked.Have || m.Ranked.Query != "foo" || !reflect.DeepEqual(listed(m), []string{"c", "a"}) {
				t.Fatalf("query %q, ranked %q, rows %v; want the committed answer for foo", m.Filters.Search, m.Ranked.Query, listed(m))
			}
			if m, _ = press(m, "/"); !m.Search.Meaning {
				t.Fatal("the committed answer's search no longer reopens by meaning")
			}
		})
	}
}

func TestAnAnswerAfterReopeningByWordsIsDropped(t *testing.T) {
	m, search := searched(t, model.Filters{}, "slow login")
	m, _ = press(m, "enter")
	m, _ = press(m, "/")
	if m.Search.Meaning {
		t.Fatal("a search with no answer reopened by meaning")
	}
	m, _ = model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query, UIDs: []string{"c"}}})
	if m.Ranked.Have || !reflect.DeepEqual(listed(m), []string{"a", "b", "c"}) {
		t.Fatalf("a late answer landed on a words search: %+v, rows %v", m.Ranked.Have, listed(m))
	}
}

func TestCtrlTInTheResultsRunsTheSearchByMeaning(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{}, "zebra")
	m, _ = press(m, "down")
	if m.CurrentMode() != model.ModeSearchResults {
		t.Fatalf("mode %s", m.CurrentMode())
	}
	m, cmds := press(m, "ctrl+t")
	search := cmdOf[model.SearchByMeaning](t, cmds)
	if !m.Search.Meaning || search.Query != "zebra" {
		t.Fatalf("meaning %v, search %+v", m.Search.Meaning, search)
	}
	var footer bool
	for binding := range model.BindingsForMode(model.ModeSearchResults, true) {
		footer = footer || binding.Chord == "ctrl+t"
	}
	if !footer {
		t.Fatal("the results footer does not offer ctrl+t")
	}
}

func TestEscapeRestoresTheWordsSearchAfterSwitchingAWordsSearchByMeaning(t *testing.T) {
	m := meaningPane(idCollection, model.Filters{}, "foo")
	m, _ = press(m, "enter", "enter", "/", "ctrl+t")
	m, cmds := press(m, "enter")
	search := cmdOf[model.SearchByMeaning](t, cmds)
	m = answered(t, m, search, ranking("c"))
	m, cmds = press(m, "escape", "escape")
	m = loaded(m, cmds)
	if m.Ranked.Have || m.Search.Meaning || m.Filters.Search != "foo" {
		t.Fatalf("after esc: ranked %v, meaning %v, query %q", m.Ranked.Have, m.Search.Meaning, m.Filters.Search)
	}
	if m, _ = press(m, "/"); m.Search.Meaning {
		t.Fatal("the next search started by meaning")
	}
}
