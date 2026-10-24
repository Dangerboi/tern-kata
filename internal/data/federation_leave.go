package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// leaveBudget is how long a leave may take in all, as in kata tui.
const leaveBudget = 15 * time.Second

// LeaveFederation leaves a spoke's federation the way kata tui does: the spoke's leave preflight first, so a refusal comes before
// anything is revoked; then, unless the leave is local-only, the revoke of this spoke's enrollments on the hub; then the leave itself.
// A failure of a step stops there, worded as kata tui words it.
func (c *Client) LeaveFederation(ctx context.Context, cmd model.LeaveFederation) (model.FederationLeft, error) {
	ctx, cancel := context.WithTimeout(ctx, leaveBudget)
	defer cancel()
	draft := cmd.Draft
	outcome := model.FederationLeaveOutcome{Draft: draft}

	preflight, err := c.leaveReplica(ctx, draft, true)
	if err != nil {
		return model.FederationLeft{}, fmt.Errorf("spoke: leave preflight failed: %w", err)
	}
	providerManaged := preflight.PendingEnrollment != nil && preflight.PendingEnrollment.ProviderManaged != nil && *preflight.PendingEnrollment.ProviderManaged
	if providerManaged && draft.LocalOnly {
		return model.FederationLeft{}, errors.New("provider cleanup cannot be discarded; turn off local-only and retry leave")
	}
	switch {
	case draft.LocalOnly:
		outcome.SkippedRevoke = true
	case !providerManaged:
		revoked, globals, err := revokeLeaveEnrollments(ctx, draft)
		if err != nil {
			return model.FederationLeft{}, err
		}
		outcome.Revoked, outcome.GlobalEnrollmentIDs = revoked, globals
	}
	left, err := c.leaveReplica(ctx, draft, false)
	if err != nil {
		return model.FederationLeft{}, fmt.Errorf("spoke: leave failed: %w", err)
	}
	outcome.Archived = left.Archived != nil && *left.Archived
	outcome.ProviderReleased = providerManaged
	return model.FederationLeft{Attempt: cmd.Attempt, Outcome: outcome}, nil
}

func (c *Client) leaveReplica(ctx context.Context, draft model.FederationLeaveDraft, preflight bool) (g.LeaveFederationReplicaResultBody, error) {
	disposition, actor := draft.Disposition(), draft.Actor
	body := g.LeaveFederationReplicaBody{Disposition: &disposition}
	if actor != "" {
		body.Actor = &actor
	}
	if preflight {
		body.Preflight = new(true)
	}
	resp, err := c.LeaveFederationReplicaWithResponse(ctx, &g.LeaveFederationReplicaRequestOptions{
		PathParams: &g.LeaveFederationReplicaPath{ProjectID: draft.ProjectID}, Body: &body})
	if err != nil {
		return g.LeaveFederationReplicaResultBody{}, daemonError(err)
	}
	if resp.JSON200 == nil {
		return g.LeaveFederationReplicaResultBody{}, errors.New("kata daemon returned no leave result")
	}
	return *resp.JSON200, nil
}

// revokeLeaveEnrollments lists the hub's enrollments and revokes every active one limited to the hub Project and made for this spoke's
// instance. None matching is success: it was revoked already. An enrollment for this spoke that is not limited to a Project is
// returned, not revoked, since it may serve other Projects. One limited to the Project but made for another instance blocks the leave
// when none matches. Any failure to reach the hub stops the leave, advising local-only.
func revokeLeaveEnrollments(ctx context.Context, draft model.FederationLeaveDraft) (revoked int, globals []int64, err error) {
	if strings.TrimSpace(draft.InstanceUID) == "" {
		return 0, nil, errors.New("spoke instance UID is not loaded; refresh federation status before leaving")
	}
	hub, err := hubAdminClient(ctx, draft.HubURL, draft.AllowInsecure)
	if err != nil {
		return 0, nil, leaveHubError(err)
	}
	listed, err := hub.ListFederationEnrollmentsWithResponse(ctx)
	if err != nil {
		return 0, nil, leaveHubError(daemonError(err))
	}
	if listed.JSON200 == nil {
		return 0, nil, leaveHubError(errors.New("kata daemon returned no federation enrollments"))
	}
	ids, globals, foreign := matchLeaveEnrollments(listed.JSON200.Enrollments, draft.InstanceUID, draft.HubProjectID)
	if len(ids) == 0 && len(foreign) > 0 {
		return 0, nil, fmt.Errorf("no active enrollment matches this spoke's instance UID, but enrollment(s) %s still authorize the hub project — "+
			"the instance UID can change after a clone/import, or the enrollment may belong to another spoke instance; "+
			"revoke the right one with `kata federation revoke <id>` on the hub, or retry with local-only to tear down locally without revoking",
			enrollmentIDs(foreign))
	}
	for _, id := range ids {
		if _, err := hub.RevokeFederationEnrollmentWithResponse(ctx, &g.RevokeFederationEnrollmentRequestOptions{
			PathParams: &g.RevokeFederationEnrollmentPath{EnrollmentID: id}}); err != nil {
			return 0, nil, leaveHubError(daemonError(err))
		}
	}
	return len(ids), globals, nil
}

// matchLeaveEnrollments sorts the hub's active enrollments: ids are those limited to the hub Project and made for instanceUID,
// globals those for instanceUID not limited to a Project, foreign those limited to the Project and made for another instance.
// The wire has no Project for a global enrollment, which reads as 0.
func matchLeaveEnrollments(enrollments []g.FederationEnrollmentOut, instanceUID string, hubProjectID int64) (ids, globals, foreign []int64) {
	for _, e := range enrollments {
		switch {
		case e.RevokedAt != nil:
		case e.ProjectID == 0:
			if e.SpokeInstanceUID == instanceUID {
				globals = append(globals, e.ID)
			}
		case e.ProjectID != hubProjectID:
		case e.SpokeInstanceUID == instanceUID:
			ids = append(ids, e.ID)
		default:
			foreign = append(foreign, e.ID)
		}
	}
	return ids, globals, foreign
}

func enrollmentIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("#%d", id)
	}
	return strings.Join(parts, ", ")
}

// leaveHubError words a failure on the hub's side as kata tui does. It carries the text only: nothing was sent to the spoke's daemon
// for the failure to be about, so the gateway must not take it for a failed dial there and run the leave again.
func leaveHubError(err error) error {
	return fmt.Errorf("hub revoke failed: %s; retry with local-only to tear down locally, then revoke on the hub later", err)
}

// hubAdminClient is the client for a binding's hub. Its token is the one of the catalog entry whose URL is the hub's, ignoring a
// trailing slash; that entry gives the token and nothing else, so the request goes to the binding's URL. allow_insecure is the
// binding's or the entry's. Without a matching entry the client sends no token: the global one is never sent to a hub.
func hubAdminClient(ctx context.Context, hubURL string, bindingInsecure bool) (*client.Client, error) {
	want := strings.TrimRight(hubURL, "/")
	auth := client.TargetAuth{AllowInsecure: bindingInsecure}
	// An unreadable catalog leaves the hub unmatched; the hub then refuses the unauthenticated request in words the Pane shows.
	catalog, _ := ReadCatalog()
	for _, entry := range catalog.Daemons {
		if entry.Local || strings.TrimRight(entry.URL, "/") != want {
			continue
		}
		token, err := entry.Token()
		if err != nil {
			return nil, err
		}
		auth.Token = token
		auth.AllowInsecure = bindingInsecure || entry.AllowInsecure
		break
	}
	hub, err := client.NewForTarget(ctx, want, auth)
	if err != nil {
		return nil, err
	}
	return hub, nil
}
