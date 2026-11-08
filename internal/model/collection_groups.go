package model

import (
	"cmp"
	"slices"
)

func IssueDate(projected *string, metadata map[string]any, key string) string {
	if projected != nil {
		return *projected
	}
	value, _ := metadata[key].(string)
	if len(value) > 10 {
		value = value[:10]
	}
	return value
}

func (g Grouping) groupOf(issue *Issue, cal PaneCalendar, whole IssueGroup) (id, title string, ok bool) {
	id, title = whole.ID, whole.Title
	today := cal.Date
	schedule, deadline := IssueDate(issue.ScheduledOnDate, issue.Metadata, ScheduledKey), IssueDate(issue.DeadlineOnDate, issue.Metadata, DeadlineKey)
	open := issue.Status == "open"
	overdue := func() { id, title = "0-overdue", "Overdue" }
	switch g {
	case GroupScheduledDate:
		if !open || schedule == "" || schedule <= today {
			return "", "", false
		}
		id, title = schedule, schedule
	case GroupClosedDate:
		if !IsClosed(*issue) || issue.ClosedAt == nil {
			return "", "", false
		}
		id = cal.DayOf(*issue.ClosedAt)
		title = id
	case GroupToday:
		if !open || !((schedule != "" && schedule <= today) || (deadline != "" && deadline <= today)) {
			return "", "", false
		}
		id, title = "1-today", "Today"
		if (schedule != "" && schedule < today) || (deadline != "" && deadline < today) {
			overdue()
		}
	case GroupDeadline:
		if !open || deadline == "" {
			return "", "", false
		}
		id, title = deadline, deadline
		if deadline < today {
			overdue()
		} else if deadline == today {
			id, title = "1-today", "Today"
		}
	case GroupProject:
		if !open {
			return "", "", false
		}
		id, title = issue.ProjectName, issue.ProjectName
		if issue.ProjectUID != nil {
			id = issue.ProjectName + "/" + *issue.ProjectUID
		}
	}
	return id, title, true
}

// GroupIssues follows kata web view.ts: calendar strings are not timezone-converted.
func GroupIssues(collection Collection, issues []Issue, cal PaneCalendar) []IssueGroup {
	grouping := collection.Def().Grouping
	whole := IssueGroup{ID: collection.View, Title: collection.Title}
	// members are each group's Issues as indexes into issues: sorted as ints and copied once, not moved about while sorting.
	members := map[string][]int{}
	titles := map[string]string{}
	for i := range issues {
		if id, title, ok := grouping.groupOf(&issues[i], cal, whole); ok {
			members[id] = append(members[id], i)
			titles[id] = title
		}
	}
	result := make([]IssueGroup, 0, len(members))
	for id, at := range members {
		slices.SortStableFunc(at, func(i, j int) int {
			a, b := &issues[i], &issues[j]
			if grouping == GroupDeadline && (id == "0-overdue" || id == "1-today") {
				if c := cmp.Compare(IssueDate(a.DeadlineOnDate, a.Metadata, DeadlineKey), IssueDate(b.DeadlineOnDate, b.Metadata, DeadlineKey)); c != 0 {
					return c
				}
			}
			if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
				return c
			}
			return cmp.Compare(a.UID, b.UID)
		})
		items := make([]Issue, len(at))
		for k, i := range at {
			items[k] = issues[i]
		}
		result = append(result, IssueGroup{ID: id, Title: titles[id], Issues: items})
	}
	slices.SortFunc(result, func(a, b IssueGroup) int {
		if grouping == GroupClosedDate {
			return cmp.Compare(b.ID, a.ID)
		}
		if grouping == GroupProject {
			if c := cmp.Compare(a.Title, b.Title); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return result
}
