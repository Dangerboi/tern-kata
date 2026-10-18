package model

import "slices"

// Grouping is how a Collection arranges its Issues into groups.
type Grouping int

const (
	GroupFlat Grouping = iota
	GroupScheduledDate
	GroupClosedDate
	GroupToday
	GroupDeadline
	GroupProject
)

// CollectionDef is everything the Pane knows about a kind of Collection: what it is called, the daemon view it
// queries, whether the query needs the local calendar date, how it groups, and whether groups show headings.
type CollectionDef struct {
	Key        string
	Title      string
	DaemonView string
	Dated      bool
	Grouping   Grouping
	Headings   bool
}

var (
	inboxDef    = CollectionDef{Key: "inbox", Title: "Inbox", DaemonView: "inbox", Headings: true}
	projectDef  = CollectionDef{Key: "project", DaemonView: "project", Grouping: GroupFlat}
	resultsDef  = CollectionDef{Key: "results", Title: "Results", Headings: true}
	fixedDefs = []CollectionDef{
		inboxDef,
		{Key: "today", Title: "Today", DaemonView: "today", Dated: true, Grouping: GroupToday, Headings: true},
		{Key: "upcoming", Title: "Upcoming", DaemonView: "upcoming", Dated: true, Grouping: GroupScheduledDate, Headings: true},
		{Key: "deadlines", Title: "Deadlines", DaemonView: "deadlines", Dated: true, Grouping: GroupDeadline, Headings: true},
		{Key: "all-open", Title: "All Open", DaemonView: "all-open", Grouping: GroupProject, Headings: true},
		{Key: "logbook", Title: "Logbook", DaemonView: "logbook", Grouping: GroupClosedDate, Headings: true},
	}
)

// InboxCollection is the Collection a Pane without a Project opens on.
func InboxCollection() Collection { return Collection{View: inboxDef.Key, Title: inboxDef.Title} }

// ProjectCollection is the Collection of one Project's Issues.
func ProjectCollection(uid, name string) Collection {
	return Collection{View: projectDef.Key, ProjectUID: uid, Title: name}
}

// ResultsCollection is the stand-in grouping for a filtered Collection.
func ResultsCollection() Collection { return Collection{View: resultsDef.Key, Title: resultsDef.Title} }

// Def is the definition of the Collection's kind; an unknown kind groups flat without headings.
func (c Collection) Def() CollectionDef {
	switch c.View {
	case "project":
		return projectDef
	case "results":
		return resultsDef
	}
	if i := slices.IndexFunc(fixedDefs, func(d CollectionDef) bool { return d.Key == c.View }); i >= 0 {
		return fixedDefs[i]
	}
	return CollectionDef{}
}

// FixedCollections lists the named Collections in Menu order.
func FixedCollections() []Collection {
	out := make([]Collection, len(fixedDefs))
	for i, d := range fixedDefs {
		out[i] = Collection{View: d.Key, Title: d.Title}
	}
	return out
}

// DatedView says the daemon view's query needs the local calendar date.
func DatedView(daemonView string) bool {
	return slices.ContainsFunc(fixedDefs, func(d CollectionDef) bool { return d.DaemonView == daemonView && d.Dated })
}
