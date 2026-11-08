package data

import (
	"context"
	"net/http"
	"slices"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// ReadSnapshot is the one read of a Collection snapshot, for the first load, a Collection switch, a filter change and a live
// refresh, in one daemon request: the snapshot's links carry the child counts. Every read is asked for today's calendar
// date, so no Collection reads kata's UTC-projected days; the result reports the calendar and holds the Collection's part
// of its daemon view's read. A matching ETag answers 304.
func (c *Client) ReadSnapshot(ctx context.Context, cmd model.ReadSnapshot) (model.SnapshotLoaded, error) {
	date, zone, note := systemCalendar.Today()
	result := model.SnapshotLoaded{Request: cmd.Request, Query: cmd.Query, LocalDate: date, TimeZone: zone, TimeZoneNote: note}
	def := model.Collection{View: *cmd.Query.View}.Def()
	query := wireQuery(cmd.Query, def.DaemonView)
	query.LocalDate, query.TimeZone = &date, &zone
	options := &g.ReadUISnapshotRequestOptions{Query: &query}
	if cmd.ETag != "" {
		options.Header = &g.ReadUISnapshotHeaders{IfNoneMatch: &cmd.ETag}
	}
	response, err := c.ReadUISnapshotWithResponse(ctx, options, plainOverSocket)
	if client.StatusCode(err) == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if err != nil {
		return result, daemonError(err)
	}
	result.Snapshot = def.Keep(translateSnapshot(response.JSON200), date)
	result.Selected = translateDetail(response.JSON200.Selected, response.JSON200.Capabilities)
	if response.Headers200 != nil {
		result.ETag = response.Headers200.ETag
	}
	return result, nil
}

// translateSnapshot is the one place kata's snapshot body becomes the model's Collection snapshot; its links carry each
// end's status, so the model counts children from them.
func translateSnapshot(s *g.UISnapshotResponseBody) model.Snapshot {
	snapshot := model.Snapshot{Cursor: s.Cursor, Issues: slices.Grow([]model.Issue(nil), len(s.Collection)), Links: slices.Grow([]model.Link(nil), len(s.CollectionLinks))}
	for i := range s.Collection {
		snapshot.Issues = append(snapshot.Issues, translateIssue(&s.Collection[i]))
	}
	for _, l := range s.CollectionLinks {
		snapshot.Links = append(snapshot.Links, translateLink(l))
	}
	for _, p := range s.Catalog {
		entry := model.CatalogProject{Project: translateProject(p.Project)}
		if p.Stats != nil {
			entry.Stats = &model.ProjectStats{Open: int(p.Stats.Open), Closed: int(p.Stats.Closed)}
		}
		snapshot.Catalog = append(snapshot.Catalog, entry)
	}
	return snapshot
}

func wireQuery(q model.SnapshotQuery, daemonView string) g.ReadUISnapshotQuery {
	w := g.ReadUISnapshotQuery{
		View: &daemonView, ProjectUID: q.ProjectUID, Status: q.Status, Owner: q.Owner, Label: q.Label, Text: q.Text,
		SelectedIssueUID: q.SelectedIssueUID, IncludeHistory: q.IncludeHistory,
	}
	for _, r := range q.Relationship {
		w.Relationship = append(w.Relationship, g.ReadUISnapshotQueryRelationship(r))
	}
	return w
}

func translateIssue(i *g.UIIssue) model.Issue {
	return model.Issue{
		Attribution:    translateAttribution(i.Verification, i.AccountableActor, i.SourceActor, i.Teammate),
		CronOccurrence: translateOccurrence(i.Metadata), AssignmentExpiresOn: i.AssignmentExpiresOn, Author: i.Author, Body: i.Body, ClosedAt: i.ClosedAt, ClosedReason: i.ClosedReason,
		CreatedAt: i.CreatedAt, DeadlineOnDate: i.DeadlineOnDate, ID: i.ID, Labels: i.Labels, Metadata: i.Metadata, Owner: i.Owner,
		Priority: i.Priority, ProjectID: i.ProjectID, ProjectName: i.ProjectName, ProjectUID: i.ProjectUID, QualifiedID: i.QualifiedID,
		RecurrenceID: i.RecurrenceID, Revision: i.Revision, ScheduledOnDate: i.ScheduledOnDate, ShortID: i.ShortID, Status: i.Status,
		Title: i.Title, UID: i.UID, UpdatedAt: i.UpdatedAt,
	}
}

func translateProject(p g.ProjectOut) model.Project {
	return model.Project{ID: p.ID, Name: p.Name, Revision: p.Revision, UID: p.UID, Archived: !p.Active, Inbox: p.Metadata["role"] == "inbox"}
}
