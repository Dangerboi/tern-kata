package model

import (
	"cmp"
	"slices"
)

func issueDate(projected *string, metadata map[string]any, key string) string {
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
func GroupIssues(collection Collection, issues []Issue, today string) []IssueGroup {
	groups := map[string][]Issue{}
	titles := map[string]string{}
	for _, issue := range issues {
		id := collection.View
		title := collection.Title
		schedule := issueDate(issue.ScheduledOnDate, issue.Metadata, "scheduled_on")
		deadline := issueDate(issue.DeadlineOnDate, issue.Metadata, "deadline_on")
		switch collection.View {
		case "upcoming":
			if issue.Status != "open" || schedule == "" || schedule <= today {
				continue
			}
			id, title = schedule, schedule
		case "logbook":
			if !IsClosed(issue) || issue.ClosedAt == nil {
				continue
			}
			id = issue.ClosedAt.Format("2006-01-02")
			title = id
		case "today":
			if issue.Status != "open" || !((schedule != "" && schedule <= today) || (deadline != "" && deadline <= today)) {
				continue
			}
			id, title = "1-today", "Today"
			if (schedule != "" && schedule < today) || (deadline != "" && deadline < today) {
				id, title = "0-overdue", "Overdue"
			}
		case "deadlines":
			if issue.Status != "open" || deadline == "" {
				continue
			}
			id, title = deadline, deadline
			if deadline < today {
				id, title = "0-overdue", "Overdue"
			} else if deadline == today {
				id, title = "1-today", "Today"
			}
		case "all-open":
			if issue.Status != "open" {
				continue
			}
			id, title = issue.ProjectName, issue.ProjectName
			if issue.ProjectUID != nil {
				id = issue.ProjectName + "/" + *issue.ProjectUID
			}
		}
		groups[id] = append(groups[id], issue)
		titles[id] = title
	}
	result := make([]IssueGroup, 0, len(groups))
	for id, items := range groups {
		slices.SortStableFunc(items, func(a, b Issue) int {
			if collection.View == "deadlines" && (id == "0-overdue" || id == "1-today") {
				if c := cmp.Compare(issueDate(a.DeadlineOnDate, a.Metadata, "deadline_on"), issueDate(b.DeadlineOnDate, b.Metadata, "deadline_on")); c != 0 {
					return c
				}
			}
			if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
				return c
			}
			return cmp.Compare(a.UID, b.UID)
		})
		result = append(result, IssueGroup{ID: id, Title: titles[id], Issues: items})
	}
	slices.SortFunc(result, func(a, b IssueGroup) int {
		if collection.View == "logbook" {
			return cmp.Compare(b.ID, a.ID)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return result
}
