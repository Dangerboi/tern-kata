package data

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ReadFederation reads the daemon's instance and its federation status, as kata tui does, and keeps the spoke rows. The active
// daemon is named from kata's catalog entry for it: the client holds neither a name nor an endpoint.
func (c *Client) ReadFederation(ctx context.Context, cmd model.LoadFederation) (model.FederationLoaded, error) {
	instance, err := c.InstanceWithResponse(ctx)
	if err != nil {
		return model.FederationLoaded{}, daemonError(err)
	}
	if instance.JSON200 == nil {
		return model.FederationLoaded{}, errors.New("kata daemon returned no instance")
	}
	status, err := c.GetFederationStatusWithResponse(ctx, nil)
	if err != nil {
		return model.FederationLoaded{}, daemonError(err)
	}
	if status.JSON200 == nil {
		return model.FederationLoaded{}, errors.New("kata daemon returned no federation status")
	}
	loaded := model.FederationLoaded{Request: cmd.Request, InstanceUID: instance.JSON200.InstanceUID}
	loaded.Daemon, loaded.Endpoint, loaded.Auth = c.activeDaemon(instance.JSON200.Auth)
	for _, wire := range status.JSON200.Statuses {
		if wire.Role == "spoke" {
			loaded.Spokes = append(loaded.Spokes, translateSpoke(wire))
		}
	}
	return loaded, nil
}

// activeDaemon is the name, endpoint and auth of the daemon this client talks to, worded as kata tui words them. An unreadable
// catalog leaves the daemon unnamed by it: discovery already read the file and would have refused it.
func (c *Client) activeDaemon(auth g.AuthInfoOut) (name, endpoint, authText string) {
	catalog, _ := ReadCatalog()
	entry, named := catalog.Entry(catalog.Active)
	endpoint = "local"
	if c.kind == model.DaemonRemote {
		endpoint = firstNonEmpty(entry.URL, os.Getenv("KATA_SERVER"), "remote")
	}
	name = "local"
	switch {
	case named:
		name = entry.Name
	case c.kind == model.DaemonRemote:
		name = endpoint
		if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
			name = u.Host
		}
	}
	return name, endpoint, federationAuth(named && entry.TokenEnv != "", auth)
}

// federationAuth is kata tui's auth display: how the connection authenticates, then who the daemon says it is. configuredToken says
// the catalog gives the connection a token, which is all the display says of auth the daemon does not describe.
func federationAuth(configuredToken bool, auth g.AuthInfoOut) string {
	base := "no token"
	if configuredToken {
		base = "token"
	}
	kind := strings.TrimSpace(auth.Kind)
	switch kind {
	case "db_token", "static_token", "bootstrap":
		base = "token"
	case "trusted_proxy", "trusted_proxy_absent":
		base = "trusted-proxy"
	}
	if auth.Actor != nil && strings.TrimSpace(*auth.Actor) != "" {
		return base + " actor " + strings.TrimSpace(*auth.Actor)
	}
	switch {
	case kind == "trusted_proxy_absent":
		return "trusted-proxy missing actor"
	case base != "token" || kind == "" || kind == "none":
		return base
	case kind == "static_token":
		return base + " static"
	case kind == "bootstrap":
		return base + " bootstrap"
	}
	return base + " " + kind
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func translateSpoke(wire g.FederationProjectStatus) model.FederationSpoke {
	spoke := model.FederationSpoke{
		ProjectID:                   wire.ProjectID,
		ProjectName:                 wire.ProjectName,
		HubURL:                      stringOrEmpty(wire.HubURL),
		HubProjectUID:               stringOrEmpty(wire.HubProjectUID),
		BoundActor:                  stringOrEmpty(wire.BoundActor),
		Capabilities:                displayCapabilities(stringOrEmpty(wire.Capabilities)),
		PushEnabled:                 wire.PushEnabled,
		CredentialStatus:            stringOrEmpty(wire.CredentialStatus),
		PendingPushCount:            wire.PendingPushCount,
		PendingPushHighWaterEventID: wire.PendingPushHighWaterEventID,
		PullCursorEventID:           wire.PullCursorEventID,
		PushCursorEventID:           wire.PushCursorEventID,
		PendingClaimCount:           wire.PendingClaimCount,
		LiveClaimCount:              wire.LiveClaimCount,
		ActiveQuarantineCount:       wire.ActiveQuarantineCount,
		ResetBlocker:                stringOrEmpty(wire.ResetBlocker),
		UnresolvedViolationCount:    wire.UnresolvedViolationCount,
		RecentViolationCount:        wire.RecentViolationCount,
		LastPullSuccessAt:           timeOrZero(wire.LastPullSuccessAt),
		LastPushSuccessAt:           timeOrZero(wire.LastPushSuccessAt),
		LastSuccessfulSyncAt:        timeOrZero(wire.LastSuccessfulSyncAt),
		LastErrorAt:                 timeOrZero(wire.LastErrorAt),
		HasLastError:                wire.LastError != nil,
		LastError:                   stringOrEmpty(wire.LastError),
	}
	if wire.AllowInsecure != nil {
		spoke.AllowInsecure = *wire.AllowInsecure
	}
	if wire.HubProjectID != nil {
		spoke.HubProjectID = *wire.HubProjectID
	}
	for _, q := range wire.ActiveQuarantines {
		spoke.ActiveQuarantines = append(spoke.ActiveQuarantines, model.FederationQuarantine{ID: q.ID, Direction: q.Direction,
			FirstEventID: q.FirstEventID, LastEventID: q.LastEventID, CreatedAt: q.CreatedAt, Error: q.ErrorData})
	}
	return spoke
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// displayCapabilities is kata's display spelling of a hub grant's capabilities: pull, push, lease (stored as claim), in that order.
func displayCapabilities(stored string) string {
	seen := map[string]bool{}
	for part := range strings.SplitSeq(stored, ",") {
		capability := strings.TrimSpace(part)
		if capability == "claim" {
			capability = "lease"
		}
		seen[capability] = true
	}
	var out []string
	for _, capability := range []string{"pull", "push", "lease"} {
		if seen[capability] {
			out = append(out, capability)
		}
	}
	return strings.Join(out, ",")
}
