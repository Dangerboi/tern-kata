package data

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

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
	status, err := c.GetFederationStatusWithResponse(ctx, nil)
	if err != nil {
		return model.FederationLoaded{}, daemonError(err)
	}
	loaded := model.FederationLoaded{Request: cmd.Request, InstanceUID: instance.JSON200.InstanceUID}
	loaded.Daemon, loaded.Endpoint, loaded.Auth = c.activeDaemon(instance.JSON200.Auth)
	for _, wire := range status.JSON200.Statuses {
		if wire.Role != "" {
			loaded.Bindings = append(loaded.Bindings, model.FederationBinding{ProjectID: wire.ProjectID, ProjectName: wire.ProjectName, Role: wire.Role})
		}
		if wire.Role == "spoke" {
			loaded.Spokes = append(loaded.Spokes, translateSpoke(wire))
		}
	}
	nameSpokes(ctx, loaded.Spokes, loaded.InstanceUID)
	return loaded, nil
}

// nameSpokes fills in the name each spoke's hub gives this instance. The name lives on the hub, per instance, so each hub is read once,
// with its catalog entry's token as a leave does; a hub that cannot be read leaves its spokes unnamed rather than failing the status.
func nameSpokes(ctx context.Context, spokes []model.FederationSpoke, instanceUID string) {
	if instanceUID == "" {
		return
	}
	seen, names := map[string]bool{}, map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, s := range spokes {
		if seen[s.HubURL] || s.HubURL == "" {
			continue
		}
		seen[s.HubURL] = true
		wg.Go(func() {
			name := hubSpokeName(ctx, s.HubURL, s.AllowInsecure, instanceUID)
			mu.Lock()
			names[s.HubURL] = name
			mu.Unlock()
		})
	}
	wg.Wait()
	for i := range spokes {
		spokes[i].SpokeName = names[spokes[i].HubURL]
	}
}

func hubSpokeName(ctx context.Context, hubURL string, allowInsecure bool, instanceUID string) string {
	ctx, cancel := context.WithTimeout(ctx, hubReadBudget)
	defer cancel()
	hub, err := hubAdminClient(ctx, hubURL, allowInsecure)
	if err != nil {
		return ""
	}
	listed, err := hub.ListFederationEnrollmentsWithResponse(ctx)
	if err != nil {
		return ""
	}
	for _, e := range listed.JSON200.Enrollments {
		if e.SpokeInstanceUID == instanceUID && e.SpokeName != nil && *e.SpokeName != "" {
			return *e.SpokeName
		}
	}
	return ""
}

// SetFederationSpokeName names or, with an empty name, unnames this instance on its hub, which keeps the label. kata's refusal of
// a name is the answer's Err, in kata's words.
func (c *Client) SetFederationSpokeName(ctx context.Context, cmd model.SetFederationSpokeName) (model.FederationSpokeNamed, error) {
	ctx, cancel := context.WithTimeout(ctx, hubReadBudget)
	defer cancel()
	named := model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL}
	fail := func(err error) (model.FederationSpokeNamed, error) {
		named.Err = err
		return named, nil
	}
	if strings.TrimSpace(cmd.InstanceUID) == "" {
		return fail(errors.New("spoke instance UID is not loaded; refresh federation status before naming"))
	}
	hub, err := hubAdminClient(ctx, cmd.HubURL, cmd.AllowInsecure)
	if err != nil {
		return fail(err)
	}
	if cmd.Name == "" {
		_, err = hub.ClearFederationSpokeNameWithResponse(ctx, &g.ClearFederationSpokeNameRequestOptions{PathParams: &g.ClearFederationSpokeNamePath{SpokeInstanceUID: cmd.InstanceUID}})
	} else {
		_, err = hub.SetFederationSpokeNameWithResponse(ctx, &g.SetFederationSpokeNameRequestOptions{
			PathParams: &g.SetFederationSpokeNamePath{SpokeInstanceUID: cmd.InstanceUID}, Body: &g.SetFederationSpokeNameBody{Name: cmd.Name}})
	}
	if err != nil {
		return fail(daemonError(err))
	}
	named.Name = cmd.Name
	return named, nil
}

// activeDaemon is the name, endpoint and auth of the daemon this client talks to, worded as kata tui words them: the catalog entry
// the connection was made through, not whichever entry active_daemon names now.
func (c *Client) activeDaemon(auth g.AuthInfoOut) (name, endpoint, authText string) {
	entry, named := c.entry, c.entry.Name != ""
	endpoint = "local"
	if c.kind == model.DaemonRemote {
		endpoint = cmp.Or(entry.URL, os.Getenv("KATA_SERVER"), c.pinURL, "remote")
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
	return name, endpoint, federationAuth(named && entry.hasToken(), auth)
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

func translateSpoke(wire g.FederationProjectStatus) model.FederationSpoke {
	spoke := model.FederationSpoke{
		ProjectID:                   wire.ProjectID,
		ProjectName:                 wire.ProjectName,
		HubURL:                      derefString(wire.HubURL),
		HubProjectUID:               derefString(wire.HubProjectUID),
		BoundActor:                  derefString(wire.BoundActor),
		Capabilities:                displayCapabilities(derefString(wire.Capabilities)),
		PushEnabled:                 wire.PushEnabled,
		CredentialStatus:            derefString(wire.CredentialStatus),
		PendingPushCount:            wire.PendingPushCount,
		PendingPushHighWaterEventID: wire.PendingPushHighWaterEventID,
		PullCursorEventID:           wire.PullCursorEventID,
		PushCursorEventID:           wire.PushCursorEventID,
		PendingClaimCount:           wire.PendingClaimCount,
		LiveClaimCount:              wire.LiveClaimCount,
		ActiveQuarantineCount:       wire.ActiveQuarantineCount,
		ResetBlocker:                derefString(wire.ResetBlocker),
		UnresolvedViolationCount:    wire.UnresolvedViolationCount,
		RecentViolationCount:        wire.RecentViolationCount,
		LastPullSuccessAt:           timeOrZero(wire.LastPullSuccessAt),
		LastPushSuccessAt:           timeOrZero(wire.LastPushSuccessAt),
		LastSuccessfulSyncAt:        timeOrZero(wire.LastSuccessfulSyncAt),
		LastErrorAt:                 timeOrZero(wire.LastErrorAt),
		HasLastError:                wire.LastError != nil,
		LastError:                   derefString(wire.LastError),
	}
	if wire.Embedding != nil {
		spoke.Embedding = embeddingFields(*wire.Embedding)
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

// embeddingFields is the spoke's embedding status as kata's FederationEmbeddingStatus.DisplayFields words it (kata's package is internal).
func embeddingFields(s g.FederationEmbeddingStatus) []string {
	fields := []string{"embedding: " + s.State}
	if r := derefString(s.Reason); r != "" {
		fields = append(fields, "embedding reason: "+r)
	}
	if e := derefString(s.LastError); e != "" {
		fields = append(fields, "embedding last error: "+e)
	}
	if s.Producer != nil {
		fields = append(fields, "embedding producer: "+s.Producer.ProducerInstanceUID,
			fmt.Sprintf("embedding recipe: %s / %d dimensions", s.Producer.Recipe.Model, s.Producer.Recipe.Dimensions))
	} else {
		fields = append(fields, "embedding producer: none")
	}
	counts := map[string]int{}
	for _, a := range s.Artifacts {
		counts[a.State]++
	}
	fields = append(fields, fmt.Sprintf("embedding artifacts: generated=%d reused=%d incompatible=%d stored_unindexed=%d",
		counts["generated"], counts["reused"], counts["incompatible"], counts["stored_unindexed"]))
	inventory := fmt.Sprintf("embedding inventory: %d retained / %d limit", len(s.Artifacts), s.ArtifactLimit)
	if s.Limited {
		inventory += " (limited)"
	}
	return append(fields, inventory)
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
