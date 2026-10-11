package data

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func localCalendar() (string, string, error) {
	zone := strings.TrimPrefix(os.Getenv("TZ"), ":")
	if zone == "" {
		path, err := filepath.EvalSymlinks("/etc/localtime")
		if err == nil {
			_, zone, _ = strings.Cut(path, "/zoneinfo/")
		}
	}
	if zone == "" {
		raw, _ := os.ReadFile("/etc/timezone")
		zone = strings.TrimSpace(string(raw))
	}
	if zone == "" {
		zone = "UTC"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", "", fmt.Errorf("local time zone %q: %w", zone, err)
	}
	return time.Now().In(location).Format("2006-01-02"), zone, nil
}

// ReadSnapshot is the one read of a Collection snapshot, for the first load, a Collection switch, a filter change and a live
// refresh. A dated Collection is asked for today's calendar date; the result reports the calendar. A matching ETag answers 304.
func (c *Client) ReadSnapshot(ctx context.Context, cmd model.ReadSnapshot) (model.SnapshotLoaded, error) {
	date, zone, err := localCalendar()
	result := model.SnapshotLoaded{Request: cmd.Request, Query: cmd.Query, LocalDate: date, TimeZone: zone}
	if err != nil {
		return result, err
	}
	query := cmd.Query
	if model.DatedView(*query.View) {
		query.LocalDate, query.TimeZone = &date, &zone
	}
	options := &g.ReadUISnapshotRequestOptions{Query: &query}
	if cmd.ETag != "" {
		options.Header = &g.ReadUISnapshotHeaders{IfNoneMatch: &cmd.ETag}
	}
	response, err := c.ReadUISnapshotWithResponse(ctx, options)
	if response != nil && response.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if err != nil {
		if response != nil {
			return result, ReadError(response.Body, err)
		}
		return result, err
	}
	if response.JSON200 == nil {
		return result, fmt.Errorf("kata daemon returned no Issue collection")
	}
	result.Snapshot = *response.JSON200
	if response.Headers200 != nil {
		result.ETag = response.Headers200.ETag
	}
	result.ChildCounts, err = c.readChildCounts(ctx, result.Snapshot, query.ProjectUID)
	return result, err
}
