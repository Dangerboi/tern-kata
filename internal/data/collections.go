package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// ReadSnapshot is the one read of a Collection snapshot, for the first load, a Collection switch, a filter change and a live
// refresh. A dated Collection is asked for today's calendar date; the result reports the calendar. A matching ETag answers 304.
func (c *Client) ReadSnapshot(ctx context.Context, cmd model.ReadSnapshot) (model.SnapshotLoaded, error) {
	date, zone, note := systemCalendar.Today()
	result := model.SnapshotLoaded{Request: cmd.Request, Query: cmd.Query, LocalDate: date, TimeZone: zone, TimeZoneNote: note}
	query := cmd.Query
	if model.DatedView(*query.View) {
		query.LocalDate, query.TimeZone = &date, &zone
	}
	options := &g.ReadUISnapshotRequestOptions{Query: &query}
	if cmd.ETag != "" {
		options.Header = &g.ReadUISnapshotHeaders{IfNoneMatch: &cmd.ETag}
	}
	response, err := c.ReadUISnapshotWithResponse(ctx, options)
	if client.StatusCode(err) == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if err != nil {
		return result, daemonError(err)
	}
	if response.JSON200 == nil {
		return result, fmt.Errorf("kata daemon returned no Issue collection")
	}
	result.Snapshot = translateSnapshot(response.JSON200)
	result.Selected = translateDetail(response.JSON200.Selected)
	if response.Headers200 != nil {
		result.ETag = response.Headers200.ETag
	}
	result.Snapshot.ChildCounts, err = c.readChildCounts(ctx, result.Snapshot, query.ProjectUID)
	return result, err
}

// translateSnapshot is the one place kata's snapshot body becomes the model's Collection snapshot; the child counts are read
// separately.
func translateSnapshot(s *g.UISnapshotResponseBody) model.Snapshot {
	snapshot := model.Snapshot{Issues: s.Collection, Cursor: s.Cursor}
	for _, l := range s.CollectionLinks {
		snapshot.Links = append(snapshot.Links, model.Link{
			ID: l.ID, Type: l.Type, FromIssueUID: l.FromIssueUID, FromQualifiedID: l.FromQualifiedID,
			ToIssueUID: l.ToIssueUID, ToQualifiedID: l.ToQualifiedID,
		})
	}
	for _, p := range s.Catalog {
		entry := model.CatalogProject{Project: p.Project}
		if p.Stats != nil {
			entry.Stats = &model.ProjectStats{Open: int(p.Stats.Open), Closed: int(p.Stats.Closed)}
		}
		snapshot.Catalog = append(snapshot.Catalog, entry)
	}
	return snapshot
}
