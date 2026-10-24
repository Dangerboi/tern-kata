package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// listNodes is the table of Page list nodes: the node a Page's list scrolls and the node of its selected row. A Page whose list
// is not here (Issues, Board, Workflows) uses the Issue list's node.
var listNodes = map[model.ListNode]struct {
	id  string
	row func(model.Model) string
}{
	model.ListNodeJobs: {JobListID, func(m model.Model) string {
		if m.Jobs.Selected == "" {
			return ""
		}
		return JobID(m.Jobs.Selected)
	}},
	model.ListNodeWorkflows: {WorkflowListID, func(m model.Model) string { return "" }},
	model.ListNodeCredentials: {CredentialListID, func(m model.Model) string {
		if c := m.SelectedCredential(); c != nil {
			return CredentialID(c.ID)
		}
		return ""
	}},
	model.ListNodeFederation: {FederationListID, func(m model.Model) string {
		if s := m.SelectedSpoke(); s != nil {
			return FederationSpokeID(s.ProjectName)
		}
		return ""
	}},
	model.ListNodeDaemons: {DaemonListID, func(m model.Model) string {
		if e := m.SelectedDaemon(); e != nil {
			return DaemonID(e.Name)
		}
		return ""
	}},
	model.ListNodeRuns: {RunListID, func(m model.Model) string {
		if m.Runs.Selected == "" {
			return ""
		}
		return RunID(m.Runs.Selected)
	}},
	model.ListNodeAttention: {AttentionListID, func(m model.Model) string {
		if m.Attention.Selected == "" {
			return ""
		}
		return AttentionID(m.Attention.Selected)
	}},
}

// TextFocusID is the node that should hold Tern's caret, "" for none.
func TextFocusID(m model.Model) string {
	if card, field, ok := m.TextCaret(); ok {
		return FieldID(card, field)
	}
	if m.TopCard() == model.NoCard && m.SearchInputFocused() {
		return SearchID
	}
	return ""
}

// ScrollTarget is the node a scroll of column c goes to: the current Page's list node, the detail content or the Menu. It is ""
// when the column is not drawn, or the detail column draws no scroller (a placeholder or an error line); a column drawn again
// later starts at its top, so a dropped scroll loses nothing.
func ScrollTarget(m model.Model, c model.Column) string {
	if !m.Shows(c) {
		return ""
	}
	switch c {
	case model.ColumnCollections:
		return CollectionsID
	case model.ColumnDetail:
		if !m.DetailDrawn() {
			return ""
		}
		return DetailContentID
	}
	if node, ok := listNodes[m.ListNode()]; ok {
		return node.id
	}
	if m.BoardShown() {
		// Each Lane scrolls inside its own column; the Board has no one list node.
		return ""
	}
	return IssueListID
}

// ActivityReveals are the nodes to reveal to bring the active Activity tab into view, none while the detail draws no Issue.
func ActivityReveals(m model.Model) []string {
	if !m.DetailDrawn() {
		return nil
	}
	return ActivityRevealIDs(m)
}

// SelectionRevealID is the node to reveal after msg moved the selection from previous, "" when none is. A reveal is sent only when
// the list is focused, drawn, settled and the selection changed; not on a click on a row, which Tern has already put under the
// pointer, and not for the answer to a read of the jobs, the Runs or the federation status, whose new cards are unknown to Tern until the next render.
func SelectionRevealID(m model.Model, msg model.Msg, previous string) string {
	if rowClick(msg) || listRead(msg) || m.Loading || m.Focus != model.ColumnList || !m.Shows(model.ColumnList) {
		return ""
	}
	if m.CurrentSelection() == "" || m.CurrentSelection() == previous {
		return ""
	}
	return SelectedRowID(m)
}

func listRead(msg model.Msg) bool {
	switch msg.(type) {
	case model.JobsLoaded, model.RunsLoaded, model.AttentionLoaded, model.FederationLoaded:
		return true
	}
	return false
}

func rowClick(msg model.Msg) bool {
	switch msg := msg.(type) {
	case model.SelectMsg, model.JobSelectMsg, model.RunSelectMsg, model.AttentionSelectMsg:
		return true
	case model.ActionMsg:
		return msg.Action == model.ToggleExpansion
	}
	return false
}

// ExpectedUnknownID reports whether id is a node that can disappear between a queued focus, reveal or scroll op and the next
// render, so that Tern's "unknown id" answer for it is expected.
func ExpectedUnknownID(id string) bool {
	if _, _, ok := FieldFromID(id); ok || id == SearchID {
		return true
	}
	if IsIssueID(id) || IsBoardCardID(id) {
		return true
	}
	// The active tab's rows and its empty line go when the tab switches, and the tab names with them when the detail turns
	// to loading another Issue before Tern runs the reveal.
	row, ok := strings.CutPrefix(id, DetailActivityID+".")
	return ok && !strings.Contains(row, ".")
}
