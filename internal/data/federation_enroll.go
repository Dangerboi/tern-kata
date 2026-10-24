package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// Enrolling a Project in a hub's federation, as kata tui does it. The hub clients come from kata's daemon catalog, each with its own
// token and no other: the global token is never sent to a hub.

const (
	// hubReadBudget is how long reading one hub may take, as in kata tui.
	hubReadBudget = 5 * time.Second
	// enrollBudget is how long an enrollment may take in all, as in kata tui.
	enrollBudget = 15 * time.Second
)

// catalogHub is a catalog entry as the hub step lists it.
func catalogHub(entry CatalogEntry, active bool) model.FederationHub {
	endpoint, auth := "local", "no token"
	if !entry.Local {
		endpoint = strings.TrimRight(entry.URL, "/")
	}
	if entry.TokenEnv != "" {
		auth = "token"
	}
	return model.FederationHub{Name: entry.Name, Endpoint: endpoint, Auth: auth, AllowInsecure: entry.AllowInsecure, Active: active, Local: entry.Local}
}

// ListFederationHubs reads kata's catalog for the hub step: every daemon in file order, the active one marked. An unreadable catalog
// is the answer's error.
func (c *Client) ListFederationHubs(_ context.Context, cmd model.LoadFederationHubs) (model.FederationHubsLoaded, error) {
	catalog, err := ReadCatalog()
	if err != nil {
		return model.FederationHubsLoaded{Request: cmd.Request, Err: err}, nil
	}
	loaded := model.FederationHubsLoaded{Request: cmd.Request}
	for _, entry := range catalog.Daemons {
		loaded.Hubs = append(loaded.Hubs, catalogHub(entry, entry.Name == c.name))
	}
	return loaded, nil
}

// hubEntry is the catalog daemon called name when it can be a hub: not the connected one and not local.
func (c *Client) hubEntry(name string) (CatalogEntry, error) {
	catalog, err := ReadCatalog()
	if err != nil {
		return CatalogEntry{}, err
	}
	entry, ok := catalog.Entry(name)
	switch {
	case !ok:
		return CatalogEntry{}, fmt.Errorf("daemon %q is not in kata's daemon catalog", name)
	case entry.Name == c.name:
		return CatalogEntry{}, errors.New("active daemon cannot be selected as hub")
	case entry.Local:
		return CatalogEntry{}, errors.New("local hub targets cannot be used for federation enrollment; select a hub daemon with a spoke-reachable URL")
	}
	return entry, nil
}

// hubEntryClient connects to a catalog daemon with its token_env token, honouring its allow_insecure.
func hubEntryClient(ctx context.Context, entry CatalogEntry) (*client.Client, error) {
	token, err := entry.Token()
	if err != nil {
		return nil, err
	}
	return client.NewForTarget(ctx, strings.TrimRight(entry.URL, "/"), client.TargetAuth{Token: token, AllowInsecure: entry.AllowInsecure})
}

// readHub reads a hub's instance and Projects: how it words its authentication, the account it says it acted as, and its Projects.
func readHub(ctx context.Context, hub *client.Client, entry CatalogEntry) (auth, actor string, projects []model.Project, err error) {
	instance, err := hub.InstanceWithResponse(ctx)
	if err != nil {
		return "", "", nil, daemonError(err)
	}
	if instance.JSON200 == nil {
		return "", "", nil, errors.New("kata daemon returned no instance")
	}
	listed, err := hub.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{})
	if err != nil {
		return "", "", nil, daemonError(err)
	}
	if listed.JSON200 == nil {
		return "", "", nil, errors.New("kata daemon returned no projects")
	}
	for _, p := range listed.JSON200.Projects {
		projects = append(projects, translateProject(p))
	}
	if instance.JSON200.Auth.Actor != nil {
		actor = strings.TrimSpace(*instance.JSON200.Auth.Actor)
	}
	return federationAuth(entry.TokenEnv != "", instance.JSON200.Auth), actor, projects, nil
}

// ReadFederationHubProjects reads the Projects of one catalog hub for the hub Project step. A hub that is refused before anything is
// sent (the active daemon, a local entry, a token_env that is unset) answers Early.
func (c *Client) ReadFederationHubProjects(ctx context.Context, cmd model.LoadFederationHubProjects) (model.FederationHubProjectsLoaded, error) {
	loaded := model.FederationHubProjectsLoaded{Request: cmd.Request}
	entry, err := c.hubEntry(cmd.Hub)
	if err != nil {
		loaded.Early, loaded.Err = true, err
		return loaded, nil
	}
	loaded.Hub = catalogHub(entry, false)
	hub, err := hubEntryClient(ctx, entry)
	if err != nil {
		loaded.Early, loaded.Err = true, err
		return loaded, nil
	}
	ctx, cancel := context.WithTimeout(ctx, hubReadBudget)
	defer cancel()
	loaded.Auth, loaded.Actor, loaded.Projects, loaded.Err = readHub(ctx, hub, entry)
	return loaded, nil
}

// BrowseFederationHubs reads the Projects of every catalog daemon that is neither the connected one nor local, each with its own token.
// A hub that cannot be read keeps its failure in its own row.
func (c *Client) BrowseFederationHubs(ctx context.Context, cmd model.BrowseFederationHubs) (model.FederationBrowsed, error) {
	catalog, err := ReadCatalog()
	if err != nil {
		return model.FederationBrowsed{Request: cmd.Request, Err: err}, nil
	}
	browsed := model.FederationBrowsed{Request: cmd.Request}
	for _, entry := range catalog.Daemons {
		if entry.Local || entry.Name == c.name {
			continue
		}
		row := model.FederationHubProjects{Hub: catalogHub(entry, false)}
		row.Auth = row.Hub.Auth
		if hub, err := hubEntryClient(ctx, entry); err != nil {
			row.Err = err
		} else {
			readCtx, cancel := context.WithTimeout(ctx, hubReadBudget)
			var auth string
			if auth, _, row.Projects, row.Err = readHub(readCtx, hub, entry); row.Err == nil {
				row.Auth = auth
			}
			cancel()
		}
		browsed.Hubs = append(browsed.Hubs, row)
	}
	return browsed, nil
}

// EnrollFederation enrolls a Project the way kata tui does, in its order: on the hub, ensure the Project (same-name adopt only),
// enable federation and create the enrollment; then read the hub Project's federation metadata with the enrollment's token; then
// create the replica on the spoke. Every failure is the answer's Err, never the call's, so the gateway never runs an enrollment
// twice. A failure once the hub granted the enrollment also carries the recovery: the token and what `kata federation join` needs.
func (c *Client) EnrollFederation(ctx context.Context, cmd model.EnrollFederation) (model.FederationEnrolled, error) {
	ctx, cancel := context.WithTimeout(ctx, enrollBudget)
	defer cancel()
	d := cmd.Draft
	failed := func(err error) (model.FederationEnrolled, error) {
		return model.FederationEnrolled{Attempt: cmd.Attempt, Err: err}, nil
	}
	if strings.TrimSpace(cmd.InstanceUID) == "" {
		return failed(errors.New("spoke instance UID is not loaded; refresh federation status before enrollment"))
	}
	entry, err := c.hubEntry(d.Hub.Name)
	if err != nil {
		return failed(err)
	}
	hub, err := hubEntryClient(ctx, entry)
	if err != nil {
		return failed(err)
	}
	hubURL := strings.TrimRight(entry.URL, "/")
	hubProject, err := resolveHubProject(ctx, hub, d)
	if err != nil {
		return failed(err)
	}
	if _, err := hub.EnableProjectFederationWithResponse(ctx, &g.EnableProjectFederationRequestOptions{
		PathParams: &g.EnableProjectFederationPath{ProjectID: hubProject.ID}, Body: &g.EnableProjectFederationBody{Actor: &d.RequestedActor}}); err != nil {
		return failed(daemonError(err))
	}
	enrollment := g.CreateFederationEnrollmentBody{Actor: &d.RequestedActor, Capabilities: d.APICapabilities, ProjectID: hubProject.ID, SpokeInstanceUID: strings.TrimSpace(cmd.InstanceUID)}
	if d.AdoptExisting {
		enrollment.AllowAdoptionSnapshotAuthors = new(true)
	}
	granted, err := hub.CreateFederationEnrollmentWithResponse(ctx, &g.CreateFederationEnrollmentRequestOptions{Body: &enrollment})
	if err != nil {
		return failed(daemonError(err))
	}
	if granted.JSON200 == nil || granted.JSON200.Token == nil || *granted.JSON200.Token == "" {
		return failed(errors.New("hub returned no enrollment token"))
	}
	token, actor := *granted.JSON200.Token, granted.JSON200.Actor

	recovery := c.recoveryBase(d, entry, hubURL, hubProject, token, actor)
	lapse := func(stage string, err error) (model.FederationEnrolled, error) {
		status := client.StatusCode(err)
		recovery.Stage = stage
		recovery.TokenGuess = status == http.StatusUnauthorized || status == http.StatusForbidden
		if stage == "metadata" {
			recovery.Err = fmt.Errorf("hub %s: enrollment metadata fetch failed: %w", entry.Name, daemonError(err))
		} else {
			recovery.Err = fmt.Errorf("spoke: join failed: %w", daemonError(err))
		}
		return model.FederationEnrolled{Attempt: cmd.Attempt, Err: recovery.Err, Recovery: &recovery}, nil
	}
	grant, err := client.NewForTarget(ctx, hubURL, client.TargetAuth{Token: token, AllowInsecure: d.AllowInsecure})
	if err != nil {
		return lapse("metadata", err)
	}
	metadata, err := grant.GetFederationProjectMetadataWithResponse(ctx, &g.GetFederationProjectMetadataRequestOptions{
		PathParams: &g.GetFederationProjectMetadataPath{ProjectID: hubProject.ID}})
	if err != nil {
		return lapse("metadata", err)
	}
	if metadata.JSON200 == nil {
		return lapse("metadata", errors.New("kata daemon returned no federation metadata"))
	}
	meta := *metadata.JSON200
	name := meta.ProjectName
	if strings.TrimSpace(d.SpokeProjectName) != "" && (d.AdoptExisting || d.Operation == model.FederationRejoin) {
		// Adopting and rejoining both bind an existing local Project, which may be named differently from the hub Project.
		name = d.SpokeProjectName
	}
	recovery.Command.HubProjectUID, recovery.Command.ProjectName = meta.ProjectUID, name
	recovery.Command.ReplayHorizonEventID, recovery.Command.BaselineThroughEventID = meta.ReplayHorizonEventID, meta.BaselineThroughEventID

	replica := g.CreateFederationReplicaBody{HubURL: hubURL, HubProjectID: hubProject.ID, HubProjectUID: meta.ProjectUID, ProjectName: name,
		ReplayHorizonEventID: meta.ReplayHorizonEventID, Token: &token, Capabilities: &d.APICapabilities, Actor: &actor}
	if meta.BaselineThroughEventID != 0 {
		replica.BaselineThroughEventID = &meta.BaselineThroughEventID
	}
	if d.AllowInsecure {
		replica.AllowInsecure = new(true)
	}
	if d.PushEnabled {
		replica.PushEnabled = new(true)
	}
	if d.AdoptExisting {
		replica.AdoptExisting = new(true)
	}
	joined, err := c.CreateFederationReplicaWithResponse(ctx, &g.CreateFederationReplicaRequestOptions{Body: &replica})
	if err != nil {
		return lapse("join", err)
	}
	if joined.JSON200 == nil {
		return lapse("join", errors.New("kata daemon returned no replica"))
	}
	outcome := model.FederationEnrollOutcome{Draft: d, HubURL: hubURL, Actor: actor, HubProjectID: meta.ProjectID, HubProjectUID: meta.ProjectUID}
	if outcome.HubProjectID == 0 {
		outcome.HubProjectID = hubProject.ID
	}
	outcome.Adopted = joined.JSON200.Adopted != nil && *joined.JSON200.Adopted
	if joined.JSON200.AdoptionSnapshotCount != nil {
		outcome.SnapshotCount = *joined.JSON200.AdoptionSnapshotCount
	}
	return model.FederationEnrolled{Attempt: cmd.Attempt, Outcome: outcome}, nil
}

// resolveHubProject is the hub Project an enrollment is for. A same-name adoption takes the hub's Project of that name, which the hub
// creates when it has none (an idempotent init); every other operation is for the Project that was chosen.
func resolveHubProject(ctx context.Context, hub *client.Client, d model.FederationEnrollDraft) (model.Project, error) {
	if d.Operation != model.FederationAdoptSameName || d.HubProjectID != 0 {
		return model.Project{ID: d.HubProjectID, Name: d.HubProjectName}, nil
	}
	initialized, err := hub.InitProjectWithResponse(ctx, &g.InitProjectRequestOptions{Body: &g.InitProjectBody{Name: &d.SpokeProjectName, Actor: &d.RequestedActor}})
	if err != nil {
		return model.Project{}, daemonError(err)
	}
	if initialized.JSON200 == nil {
		return model.Project{}, errors.New("kata daemon returned no project")
	}
	return translateProject(initialized.JSON200.Project), nil
}

// recoveryBase is what finishing an enrollment by hand needs once the hub granted it, before the metadata is read: the join command
// with the enrollment's token, and the spoke it is for. A remote connected daemon's own token goes into the command too, as kata tui does:
// the token and allow_insecure of the entry this connection was made through, never another daemon's.
func (c *Client) recoveryBase(d model.FederationEnrollDraft, hub CatalogEntry, hubURL string, hubProject model.Project, token, actor string) model.FederationEnrollRecovery {
	name, endpoint, _ := c.activeDaemon(g.AuthInfoOut{})
	command := model.FederationJoinCommand{HubURL: hubURL, HubProjectID: hubProject.ID, ProjectName: d.SpokeProjectName, Token: token, Actor: actor,
		Capabilities: d.APICapabilities, PushEnabled: d.PushEnabled, AllowInsecure: d.AllowInsecure, AdoptExisting: d.AdoptExisting, SpokeEndpoint: endpoint}
	if command.ProjectName == "" {
		command.ProjectName = hubProject.Name
	}
	if c.kind == model.DaemonRemote {
		if c.entry.Name != "" {
			command.SpokeAllowInsecure = c.entry.AllowInsecure
			command.SpokeToken, _ = c.entry.Token()
		}
		if command.SpokeToken == "" {
			command.SpokeToken = strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN"))
		}
	}
	return model.FederationEnrollRecovery{HubName: hub.Name, SpokeName: name, SpokeEndpoint: endpoint, Command: command}
}
