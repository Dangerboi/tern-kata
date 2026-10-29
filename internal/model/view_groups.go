package model

// GroupCollection groups the Issues a Collection and its filters selected, as the Pane lists them and `tern-kata views show`
// prints them: a search groups as Results, an Issue list whose Project base and Project filter name two Projects is empty,
// and every group shows a heading when the base Collection's groups do.
func GroupCollection(c Collection, f Filters, issues []Issue, cal PaneCalendar) []IssueGroup {
	grouping := c
	if f.Search != "" {
		grouping = ResultsCollection()
	}
	// Snapshot accepts one Project UID; incompatible Project scopes intersect to empty.
	if c.ProjectUID != "" && f.ProjectUID != "" && c.ProjectUID != f.ProjectUID {
		issues = nil
	}
	groups := GroupIssues(grouping, issues, cal)
	for i := range groups {
		groups[i].Heading = c.Def().Headings
	}
	return groups
}
