package model

type IssueRow struct {
	Issue
	Depth       int
	Guide       string // tree connectors before the id: "├─ ", "│  └─ "
	HasChildren bool
	Expanded    bool
	OpenKids    int
	TotalKids   int
}

func (m Model) issueHierarchy() (map[string]string, map[string][]Issue) {
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
	children := make(map[string][]Issue)
	for _, issue := range m.Issues {
		if parent := parents[issue.UID]; parent != "" {
			children[parent] = append(children[parent], issue)
		}
	}
	if m.GraphOrder {
		blocked := m.blockedBy()
		for parent, kin := range children {
			children[parent] = blockersFirst(kin, blocked)
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

// blockersFirst orders siblings topologically by blocks, blockers first. Among Issues free to go next the earliest in
// creation order (the order given) goes first; Issues caught in a blocking cycle follow the rest in creation order.
func blockersFirst(siblings []Issue, blocks map[string][]string) []Issue {
	if len(siblings) < 2 {
		return siblings
	}
	index := make(map[string]int, len(siblings))
	for i, issue := range siblings {
		index[issue.UID] = i
	}
	blocking := make([][]int, len(siblings))
	waiting := make([]int, len(siblings))
	for i, issue := range siblings {
		for _, uid := range blocks[issue.UID] {
			if j, ok := index[uid]; ok && j != i {
				blocking[i] = append(blocking[i], j)
				waiting[j]++
			}
		}
	}
	ordered := make([]Issue, 0, len(siblings))
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

func (m Model) VisibleRows() []IssueRow {
	if m.OnBoard() {
		return m.boardRows()
	}
	rows := make([]IssueRow, 0, len(m.Issues))
	parents, children := m.issueHierarchy()
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
	// guide is this row's connector; indent is what its children's guides start with.
	var appendIssue func(issue Issue, depth int, guide, indent string)
	appendIssue = func(issue Issue, depth int, guide, indent string) {
		expanded := m.ExpandedUIDs[issue.UID]
		rows = append(rows, kids(IssueRow{Issue: issue, Depth: depth, Guide: guide, HasChildren: len(children[issue.UID]) > 0, Expanded: expanded}))
		if expanded {
			kin := children[issue.UID]
			for i, child := range kin {
				if i == len(kin)-1 {
					appendIssue(child, depth+1, indent+"└─\u00a0", indent+"\u00a0\u00a0\u00a0")
				} else {
					appendIssue(child, depth+1, indent+"├─\u00a0", indent+"│\u00a0\u00a0")
				}
			}
		}
	}
	for _, issue := range m.Issues {
		if parents[issue.UID] == "" {
			appendIssue(issue, 0, "", "")
		}
	}
	return rows
}

func (m Model) WithVisibleSelection() Model {
	rows := m.VisibleRows()
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
