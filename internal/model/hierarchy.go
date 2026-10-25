package model

import "slices"

type IssueRow struct {
	Issue
	Depth       int
	Guide       string // tree connectors before the id: "├─ ", "│  └─ "
	HasChildren bool
	Expanded    bool
	OpenKids    int
	TotalKids   int
}

// issueHierarchy is each loaded Issue's loaded parent, and each parent's loaded children as indexes into m.Issues, in list
// order or, under GraphOrder, blockers first.
func (m Model) issueHierarchy() (map[string]string, map[string][]int) {
	loaded := make(map[string]bool, len(m.Issues))
	for _, issue := range m.Issues {
		loaded[issue.UID] = true
	}
	parents := make(map[string]string)
	for _, link := range m.Snapshot.Links {
		if link.Type == "parent" && loaded[link.FromIssueUID] && loaded[link.ToIssueUID] && link.FromIssueUID != link.ToIssueUID {
			parents[link.FromIssueUID] = link.ToIssueUID
		}
	}
	// Break cyclic parent paths so every loaded Issue still has a reachable root.
	state := make(map[string]int, len(m.Issues))
	var visit func(string)
	visit = func(uid string) {
		if state[uid] == 2 {
			return
		}
		if state[uid] == 1 {
			delete(parents, uid)
			return
		}
		state[uid] = 1
		if parent := parents[uid]; parent != "" {
			visit(parent)
		}
		state[uid] = 2
	}
	for _, issue := range m.Issues {
		visit(issue.UID)
	}
	children := make(map[string][]int)
	for i, issue := range m.Issues {
		if parent := parents[issue.UID]; parent != "" {
			children[parent] = append(children[parent], i)
		}
	}
	if m.GraphOrder {
		blocked := m.blockedBy()
		for parent, kin := range children {
			children[parent] = blockersFirst(m.Issues, kin, blocked)
		}
	}
	return parents, children
}

// blockedBy maps each Issue to the Issues it blocks. A blocks link runs from the blocker to the Issue it blocks, a
// blocked_by link the other way round.
func (m Model) blockedBy() map[string][]string {
	blocks := make(map[string][]string)
	for _, link := range m.Snapshot.Links {
		switch link.Type {
		case "blocks":
			blocks[link.FromIssueUID] = append(blocks[link.FromIssueUID], link.ToIssueUID)
		case "blocked_by":
			blocks[link.ToIssueUID] = append(blocks[link.ToIssueUID], link.FromIssueUID)
		}
	}
	return blocks
}

// blockersFirst orders siblings, indexes into issues, topologically by blocks, blockers first. Among Issues free to go next the
// earliest in creation order (the order given) goes first; Issues caught in a blocking cycle follow the rest in creation order.
func blockersFirst(issues []Issue, siblings []int, blocks map[string][]string) []int {
	if len(siblings) < 2 {
		return siblings
	}
	index := make(map[string]int, len(siblings))
	for i, at := range siblings {
		index[issues[at].UID] = i
	}
	blocking := make([][]int, len(siblings))
	waiting := make([]int, len(siblings))
	for i, at := range siblings {
		for _, uid := range blocks[issues[at].UID] {
			if j, ok := index[uid]; ok && j != i {
				blocking[i] = append(blocking[i], j)
				waiting[j]++
			}
		}
	}
	ordered := make([]int, 0, len(siblings))
	done := make([]bool, len(siblings))
	for len(ordered) < len(siblings) {
		next := -1
		for i := range siblings {
			if !done[i] && waiting[i] == 0 {
				next = i
				break
			}
		}
		if next < 0 {
			for i := range siblings {
				if !done[i] {
					ordered = append(ordered, siblings[i])
				}
			}
			break
		}
		done[next] = true
		ordered = append(ordered, siblings[next])
		for _, j := range blocking[next] {
			waiting[j]--
		}
	}
	return ordered
}

// VisibleRows are the rows the selection moves over: the Board's cards on the Board Page, else the list's rows, nested unless
// Flat. The slice is shared with the kept rows; see collectionRows.
func (m Model) VisibleRows() []IssueRow {
	if m.collection.answers(m) {
		return m.collection.rows
	}
	if m.OnBoard() {
		return m.boardRows()
	}
	return m.listRows()
}

// listRows are the list's rows worked out afresh.
func (m Model) listRows() []IssueRow {
	rows := make([]IssueRow, 0, len(m.Issues))
	counts := childCountsByParent(m.Snapshot.Links)
	kids := func(row IssueRow) IssueRow {
		c := counts[row.UID]
		row.OpenKids, row.TotalKids = c.Open, c.Total
		return row
	}
	if m.Flat {
		for _, issue := range m.Issues {
			rows = append(rows, kids(IssueRow{Issue: issue}))
		}
		return rows
	}
	parents, children := m.issueHierarchy()
	// guide is this row's connector; indent is what its children's guides start with.
	var appendIssue func(issue *Issue, depth int, guide, indent string)
	appendIssue = func(issue *Issue, depth int, guide, indent string) {
		expanded := m.ExpandedUIDs[issue.UID]
		rows = append(rows, kids(IssueRow{Issue: *issue, Depth: depth, Guide: guide, HasChildren: len(children[issue.UID]) > 0, Expanded: expanded}))
		if expanded {
			kin := children[issue.UID]
			for i, child := range kin {
				if i == len(kin)-1 {
					appendIssue(&m.Issues[child], depth+1, indent+"└─\u00a0", indent+"\u00a0\u00a0\u00a0")
				} else {
					appendIssue(&m.Issues[child], depth+1, indent+"├─\u00a0", indent+"│\u00a0\u00a0")
				}
			}
		}
	}
	for i := range m.Issues {
		if parents[m.Issues[i].UID] == "" {
			appendIssue(&m.Issues[i], 0, "", "")
		}
	}
	return rows
}

// WithVisibleSelection keeps the selection on a visible row: the selected Issue, else its nearest visible ancestor, else the
// first row. It keeps the rows it works out, so the rest of the step reads them again for free.
func (m Model) WithVisibleSelection() Model {
	m = m.withCollectionRows()
	rows := m.VisibleRows()
	if m.SelectedUID != "" && slices.ContainsFunc(rows, func(row IssueRow) bool { return row.UID == m.SelectedUID }) {
		return m
	}
	visible := make(map[string]bool, len(rows))
	for _, row := range rows {
		visible[row.UID] = true
	}
	parents, _ := m.issueHierarchy()
	uid := m.SelectedUID
	for uid != "" {
		if visible[uid] {
			m.SelectedUID = uid
			return m
		}
		uid = parents[uid]
	}
	m.SelectedUID = ""
	if len(rows) > 0 {
		m.SelectedUID = rows[0].UID
	}
	return m
}

func updateHierarchy(m Model, action Action) Model {
	if action == ToggleGraphOrder {
		if m.Flat {
			return m
		}
		m.GraphOrder = !m.GraphOrder
		m.Notice = "graph order: temporal"
		if m.GraphOrder {
			m.Notice = "graph order: topological"
		}
		return m.WithVisibleSelection()
	}
	if action == ToggleIssueView {
		m.Flat = !m.Flat
		if !m.Flat {
			m.ExpandedUIDs = nil
		}
		return m.WithVisibleSelection()
	}
	if m.Flat {
		return m
	}
	_, children := m.issueHierarchy()
	if action == ToggleAllExpansion {
		allExpanded := len(children) > 0
		for uid := range children {
			allExpanded = allExpanded && m.ExpandedUIDs[uid]
		}
		m.ExpandedUIDs = make(map[string]bool, len(children))
		if !allExpanded {
			for uid := range children {
				m.ExpandedUIDs[uid] = true
			}
		}
		return m.WithVisibleSelection()
	}
	if len(children[m.SelectedUID]) == 0 {
		return m
	}
	expanded := m.ExpandedUIDs[m.SelectedUID]
	switch action {
	case ToggleExpansion:
		expanded = !expanded
	case ExpandIssue:
		expanded = true
	case CollapseIssue:
		expanded = false
	}
	if expanded == m.ExpandedUIDs[m.SelectedUID] {
		return m
	}
	expansions := make(map[string]bool, len(m.ExpandedUIDs)+1)
	for uid, open := range m.ExpandedUIDs {
		expansions[uid] = open
	}
	expansions[m.SelectedUID] = expanded
	m.ExpandedUIDs = expansions
	return m.WithVisibleSelection()
}
