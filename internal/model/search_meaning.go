package model

import (
	"strconv"
	"strings"
)

// SearchLimit is how many hits kata's ranked search is asked for: the best fifty, however many Issues match.
const SearchLimit = 50

// SearchByMeaning asks kata's ranked search of one Project: mode auto, so kata ranks by words and meaning together when its
// embeddings work and by words alone otherwise. Status and Labels are the Pane's filters; the Board sends no Status.
type SearchByMeaning struct {
	Workspace string
	ProjectID int64
	Query     string
	Status    string
	Labels    []string
	Request   int64
}

func (SearchByMeaning) cmd() {}

// RankedAnswer is what kata answered to a ranked search: the hits' Issue UIDs in kata's order, for Query. Lexical says kata
// answered by words and Reason is the note kata gave when it marked the answer degraded, whatever the mode; Truncated says more
// Issues matched than SearchLimit. The message that brings it and the state that keeps it both hold it.
type RankedAnswer struct {
	Query     string
	UIDs      []string
	Lexical   bool
	Reason    string
	Truncated bool
}

// SearchByMeaningDone answers SearchByMeaning with kata's answer, or with Err when kata gave none.
type SearchByMeaningDone struct {
	Request int64
	RankedAnswer
	Err error
}

func (SearchByMeaningDone) msg() {}

// rankedFilters is the Project, Status and Label filters a ranked search was asked with: kata applies all three, so an answer
// holds only while the Pane's filters are still these. Status is empty on the Board, which ignores it.
type rankedFilters struct{ ProjectUID, Status, Label string }

// rankedAsk is one ranked search as it was asked: the query and the filters. The zero value is no ask.
type rankedAsk struct {
	Query   string
	Filters rankedFilters
}

// RankedSearch is kata's last ranked answer for the Pane: the Issues in its order, and what it said about the answer. It lists
// under the Results grouping while by meaning is on in the search box, and after the search is committed while the search text is
// still the query it answers and the filters still fit it (rankedShown). Err is the last failed run, which leaves the answer in
// place.
type RankedSearch struct {
	Have bool
	RankedAnswer
	Err error
	// SearchedWith is the filters the answer was asked with.
	SearchedWith rankedFilters
}

// currentAsk is the query typed and the filters set now.
func (m Model) currentAsk() rankedAsk {
	status := m.Filters.Status
	if m.OnBoard() {
		status = ""
	}
	return rankedAsk{m.Filters.Search, rankedFilters{m.Filters.ProjectUID, status, m.Filters.Label}}
}

// askedNow says kata was asked, whatever it answered, the query typed and the filters set now.
func (m Model) askedNow() bool { return m.rankedAsked == m.currentAsk() }

// withoutRanked drops the ranked answer and the ask, and makes any answer still out stale.
func (m Model) withoutRanked() Model {
	m.Ranked, m.rankedAsked = RankedSearch{}, rankedAsk{}
	m.rankedReads.Discard()
	return m
}

const searchMeaningRefusal = "search by meaning works inside one Project"

// rankedShown says the list is kata's ranked answer: not one the short-id search names, asked with the filters now set, and
// either by meaning is on or the committed search text is the query the answer is for. An answer asked without a Status (as
// on the Board) that kata did not cut stays under a Status set later: the snapshot's own Status trims its hits to what kata
// would answer. A cut answer, or one asked with a Status, ends when the Status changes.
func (m Model) rankedShown() bool {
	asked, now := m.Ranked.SearchedWith, m.currentAsk().Filters
	if asked.Status == "" && !m.Ranked.Truncated {
		asked.Status = now.Status
	}
	if !m.Ranked.Have || m.idMatch() != "" || asked != now {
		return false
	}
	return m.Search.Meaning || m.Filters.Search != "" && m.Ranked.Query == m.Filters.Search
}

// meaningProject is the one Project the Collection shows, which kata's search covers: a Project Collection, or a Collection
// whose Project filter names one.
func (m Model) meaningProject() (int64, bool) {
	uid := m.Collection.ProjectUID
	if f := m.Filters.ProjectUID; f != "" {
		if uid != "" && uid != f {
			return 0, false
		}
		uid = f
	}
	if uid == "" {
		return 0, false
	}
	entry, ok := CatalogEntry(m.Snapshot, uid)
	return entry.Project.ID, ok
}

func searchMeaningLabel(m Model) string {
	if m.Search.Meaning {
		return "by words"
	}
	return "by meaning"
}

// toggleSearchMeaning is ctrl+t in the search box: by words and by meaning, on the same query. By meaning needs one Project.
func toggleSearchMeaning(m Model) (Model, []Cmd) {
	if m.Search.Meaning {
		m.Search.Meaning, m.Search.Refused = false, false
		m = m.withoutRanked()
		return relist(m)
	}
	if _, ok := m.meaningProject(); !ok {
		m.Search.Refused = true
		return m, nil
	}
	m.Search.Meaning, m.Search.Refused = true, false
	m, cmds := relist(m)
	if m.Search.Results { // in the results the query is already entered: the switch runs it
		var search []Cmd
		m, search = runSearchByMeaning(m)
		cmds = append(cmds, search...)
	}
	return m, cmds
}

// relist lists again from the snapshot on screen and reads the snapshot the new state asks for.
func relist(m Model) (Model, []Cmd) {
	m = m.applyCollection(m.Snapshot)
	return snapshotChanged(m)
}

// runSearchByMeaning is enter by meaning: one ranked search of the query, unless the query is a short id the id search has
// already resolved. Typing sends nothing, and neither do live events.
func runSearchByMeaning(m Model) (Model, []Cmd) {
	id, ok := m.meaningProject()
	if !m.Search.Meaning || !ok || strings.TrimSpace(m.Filters.Search) == "" || m.idMatch() != "" {
		return m, nil
	}
	ask := m.currentAsk()
	m.rankedAsked = ask
	return m, []Cmd{SearchByMeaning{
		Workspace: m.Workspace, ProjectID: id, Query: m.Filters.Search, Status: ask.Filters.Status,
		Labels: labelList(m.Filters.Label), Request: m.rankedReads.Ask(),
	}}
}

// meaningTyped is a keystroke in the query by meaning: it edits the text and leaves the list as it is. A query that reads as a
// short id still goes to the id search, which jumps to that Issue.
func meaningTyped(m Model, text string) (Model, []Cmd) {
	m.Filters.Search = text
	_, isID := searchShortID(text)
	if !isID {
		m.IDSearch = IDSearch{}
	}
	m = m.applyCollection(m.Snapshot)
	if isID {
		return snapshotChanged(m)
	}
	return m, nil
}

func searchMeaningDone(m Model, msg SearchByMeaningDone) (Model, []Cmd) {
	if !m.rankedReads.IsCurrent(msg.Request) {
		return m, nil
	}
	if msg.Err != nil {
		m.Ranked.Err = msg.Err
		return m, nil
	}
	m.Ranked = RankedSearch{Have: true, RankedAnswer: msg.RankedAnswer, SearchedWith: m.rankedAsked.Filters}
	return relist(m)
}

// rankedGroup is the ranked answer's Issues, those the snapshot lists, in kata's order as one group under Results.
func (m Model) rankedGroup(issues []Issue) IssueGroup {
	byUID := make(map[string]*Issue, len(issues))
	for i := range issues {
		byUID[issues[i].UID] = &issues[i]
	}
	group := IssueGroup{ID: resultsDef.Key, Title: resultsDef.Title, Heading: m.Collection.Def().Headings}
	for _, uid := range m.Ranked.UIDs {
		if issue, ok := byUID[uid]; ok {
			group.Issues = append(group.Issues, *issue)
		}
	}
	return group
}

// SearchLine is what the search line says: why by meaning is refused, what kata said about its answer, and which filters the
// answer ignores. Empty when there is nothing to say.
func (m Model) SearchLine() string {
	var parts []string
	if m.Search.Open && m.Search.Refused {
		parts = append(parts, searchMeaningRefusal)
	}
	if m.Search.Meaning || m.rankedShown() {
		if m.Ranked.Err != nil {
			parts = append(parts, oneLine(m.Ranked.Err.Error()))
		}
		if m.Ranked.Have && !m.Ranked.Lexical && m.Ranked.Reason != "" {
			parts = append(parts, "kata: "+m.Ranked.Reason)
		}
		if m.Ranked.Have && m.Ranked.Lexical {
			reason := m.Ranked.Reason
			if reason == "" {
				reason = "turn on search by meaning with `kata daemon local-embeddings`"
			}
			parts = append(parts, "kata searched by words: "+reason)
		}
		if m.Ranked.Have && m.Ranked.Truncated {
			parts = append(parts, "showing the best "+strconv.Itoa(SearchLimit))
		}
		var ignored []string
		if m.Filters.Owner != "" {
			ignored = append(ignored, "Owner")
		}
		if m.Filters.Relationship != "" {
			ignored = append(ignored, "Relationship")
		}
		switch len(ignored) {
		case 1:
			parts = append(parts, "the "+ignored[0]+" filter doesn't apply to search by meaning")
		case 2:
			parts = append(parts, "the Owner and Relationship filters don't apply to search by meaning")
		}
	}
	return strings.Join(parts, " · ")
}

// labelList is the Label filter's comma-separated labels.
func labelList(filter string) []string {
	var labels []string
	for _, label := range strings.Split(filter, ",") {
		if label = strings.TrimSpace(label); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}
