package model

import "slices"

type Grouping int

const (
	GroupFlat Grouping = iota
	GroupScheduledDate
	GroupClosedDate
	GroupToday
	GroupDeadline
	GroupProject
)

type CollectionDef struct {
	Key        string
	Title      string
	DaemonView string
	Members    Membership
	// Dated says the read needs the local calendar: the daemon projects planning dates into the zone, and a dated daemon view
	// or Membership compares them with the day.
	Dated    bool
	Grouping Grouping
	Headings bool
}

// Membership is the rule that keeps a Collection's Issues out of its daemon view's read. Kata keeps its daemon's inbox,
// upcoming and deadlines views only for older web UIs, so those Collections read all-open and keep their Issues here, by the
// rules those views apply.
type Membership int

const (
	EveryIssue           Membership = iota
	InboxProjectIssues              // the Issues of a Project whose metadata role is "inbox"
	LaterScheduledIssues            // the Issues scheduled for a day after today, in the calendar the read was asked for
	DeadlineIssues                  // the Issues with a deadline
)

var (
	inboxDef   = CollectionDef{Key: "inbox", Title: "Inbox", DaemonView: "all-open", Members: InboxProjectIssues, Headings: true}
	projectDef = CollectionDef{Key: "project", DaemonView: "project", Grouping: GroupFlat}
	resultsDef = CollectionDef{Key: "results", Title: "Results", Headings: true}
	allOpenDef = CollectionDef{Key: "all-open", Title: "All Open", DaemonView: "all-open", Grouping: GroupProject, Headings: true}
	fixedDefs  = []CollectionDef{
		inboxDef,
		{Key: "today", Title: "Today", DaemonView: "today", Dated: true, Grouping: GroupToday, Headings: true},
		{Key: "upcoming", Title: "Upcoming", DaemonView: "all-open", Members: LaterScheduledIssues, Dated: true, Grouping: GroupScheduledDate, Headings: true},
		{Key: "deadlines", Title: "Deadlines", DaemonView: "all-open", Members: DeadlineIssues, Dated: true, Grouping: GroupDeadline, Headings: true},
		allOpenDef,
		{Key: "logbook", Title: "Logbook", DaemonView: "logbook", Grouping: GroupClosedDate, Headings: true},
	}
)

func InboxCollection() Collection { return Collection{View: inboxDef.Key, Title: inboxDef.Title} }

func ProjectCollection(uid, name string) Collection {
	return Collection{View: projectDef.Key, ProjectUID: uid, Title: name}
}

func ResultsCollection() Collection { return Collection{View: resultsDef.Key, Title: resultsDef.Title} }

// Def is the definition of the Collection's kind; an unknown kind groups flat without headings.
func (c Collection) Def() CollectionDef {
	switch c.View {
	case projectDef.Key:
		return projectDef
	case resultsDef.Key:
		return resultsDef
	}
	if i := slices.IndexFunc(fixedDefs, func(d CollectionDef) bool { return d.Key == c.View }); i >= 0 {
		return fixedDefs[i]
	}
	return CollectionDef{}
}

func FixedCollections() []Collection {
	out := make([]Collection, len(fixedDefs))
	for i, d := range fixedDefs {
		out[i] = Collection{View: d.Key, Title: d.Title}
	}
	return out
}

// Keep is the part of a snapshot read through the Collection's daemon view that belongs to the Collection on the local day
// today: the Issues its Membership keeps and the Links touching them, as the daemon sends them for a view of its own. It
// reuses s's slices.
func (d CollectionDef) Keep(s Snapshot, today string) Snapshot {
	if d.Members == EveryIssue {
		return s
	}
	inboxProjects := map[int64]bool{}
	for _, entry := range s.Catalog {
		if entry.Project.Inbox {
			inboxProjects[entry.Project.ID] = true
		}
	}
	kept := make(map[string]bool, len(s.Issues))
	s.Issues = slices.DeleteFunc(s.Issues, func(issue Issue) bool {
		member := d.Members.has(issue, inboxProjects, today)
		if member {
			kept[issue.UID] = true
		}
		return !member
	})
	s.Links = slices.DeleteFunc(s.Links, func(l Link) bool { return !kept[l.FromIssueUID] && !kept[l.ToIssueUID] })
	return s
}

func (r Membership) has(issue Issue, inboxProjects map[int64]bool, today string) bool {
	switch r {
	case InboxProjectIssues:
		return inboxProjects[issue.ProjectID]
	case LaterScheduledIssues:
		return issue.ScheduledOnDate != nil && *issue.ScheduledOnDate > today
	case DeadlineIssues:
		return issue.Metadata[DeadlineKey] != nil
	}
	return true
}

func DetailDaemonView() string { return allOpenDef.DaemonView }
