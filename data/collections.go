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

func (c *Client) ReadCollection(ctx context.Context, cmd model.LoadCollection) (model.CollectionLoaded, error) {
	date, zone, err := localCalendar()
	if err != nil {
		return model.CollectionLoaded{}, err
	}
	intent := model.Model{Collection: cmd.Collection, Filters: cmd.Filters, LocalDate: date, TimeZone: zone}
	query := intent.SnapshotQuery()
	options := &g.ReadUISnapshotRequestOptions{Query: &query}
	if cmd.ETag != "" {
		options.Header = &g.ReadUISnapshotHeaders{IfNoneMatch: new(cmd.ETag)}
	}
	snapshot, err := c.ReadUISnapshotWithResponse(ctx, options)
	result := model.CollectionLoaded{Collection: cmd.Collection, Filters: cmd.Filters, LocalDate: date, TimeZone: zone}
	if snapshot != nil && snapshot.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if err != nil {
		if snapshot != nil {
			return result, ReadError(snapshot.Body, err)
		}
		return result, err
	}
	if snapshot.JSON200 == nil {
		return result, fmt.Errorf("kata daemon returned no Issue collection")
	}
	result.Snapshot = *snapshot.JSON200
	if snapshot.Headers200 != nil {
		result.ETag = snapshot.Headers200.ETag
	}
	return result, nil
}
