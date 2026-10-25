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

// GroupIssues follows kata web view.ts: calendar strings are not timezone-converted.
func GroupIssues(collection Collection, issues []Issue, cal PaneCalendar) []IssueGroup {
	grouping := collection.Def().Grouping
	today := cal.Date
	// members are each group's Issues as indexes into issues: sorted as ints and copied once, not moved about while sorting.
	members := map[string][]int{}
	titles := map[string]string{}
	for i, issue := range issues {
		id := collection.View
		title := collection.Title
		schedule := IssueDate(issue.ScheduledOnDate, issue.Metadata, ScheduledKey)
		deadline := IssueDate(issue.DeadlineOnDate, issue.Metadata, DeadlineKey)
		switch grouping {
		case GroupScheduledDate:
			if issue.Status != "open" || schedule == "" || schedule <= today {
				continue
			}
			id, title = schedule, schedule
		case GroupClosedDate:
			if !IsClosed(issue) || issue.ClosedAt == nil {
				continue
			}
			id = cal.DayOf(*issue.ClosedAt)
			title = id
		case GroupToday:
			if issue.Status != "open" || !((schedule != "" && schedule <= today) || (deadline != "" && deadline <= today)) {
				continue
			}
			id, title = "1-today", "Today"
			if (schedule != "" && schedule < today) || (deadline != "" && deadline < today) {
				id, title = "0-overdue", "Overdue"
			}
		case GroupDeadline:
			if issue.Status != "open" || deadline == "" {
				continue
			}
			id, title = deadline, deadline
			if deadline < today {
				id, title = "0-overdue", "Overdue"
			} else if deadline == today {
				id, title = "1-today", "Today"
			}
		case GroupProject:
			if issue.Status != "open" {
				continue
			}
			id, title = issue.ProjectName, issue.ProjectName
			if issue.ProjectUID != nil {
				id = issue.ProjectName + "/" + *issue.ProjectUID
			}
		}
		members[id] = append(members[id], i)
		titles[id] = title
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
		return cmp.Compare(a.ID, b.ID)
	})
	return result
}
