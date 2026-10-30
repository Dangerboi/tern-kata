package data

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ListAttention reads every open Issue of the daemon, in every Project, that holds a request for cmd.Address, as kata's own TUI
// does: one read of all Projects with the address's slot key as a presence filter and no limit. The slot is read with the model's
// reader; one that does not read is counted, not listed. The requests come newest Issue update first.
func (c *Client) ListAttention(ctx context.Context, cmd model.LoadAttention) (model.AttentionLoaded, error) {
	key := model.NotificationKey(cmd.Address)
	open, unlimited := g.ListAllIssuesQueryStatusOpen, int64(0)
	listed, err := c.ListAllIssuesWithResponse(ctx, &g.ListAllIssuesRequestOptions{
		Query: &g.ListAllIssuesQuery{Status: &open, Limit: &unlimited, Meta: []string{key}},
	})
	if err != nil {
		return model.AttentionLoaded{}, daemonError(err)
	}
	if listed.JSON200 == nil {
		return model.AttentionLoaded{}, errors.New("kata returned no Issues")
	}
	// The slot is read from the body as sent, so its raw value is what kata holds, not a re-encoding of a decoded one.
	var body struct {
		Issues []struct {
			UID         string                     `json:"uid"`
			ShortID     string                     `json:"short_id"`
			Title       string                     `json:"title"`
			Status      string                     `json:"status"`
			ProjectID   int64                      `json:"project_id"`
			ProjectName string                     `json:"project_name"`
			UpdatedAt   time.Time                  `json:"updated_at"`
			Metadata    map[string]json.RawMessage `json:"metadata"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(listed.Body, &body); err != nil {
		return model.AttentionLoaded{}, err
	}
	out := model.AttentionLoaded{Request: cmd.Request, Requests: make([]model.AttentionRequest, 0, len(body.Issues))}
	for _, issue := range body.Issues {
		raw, held := issue.Metadata[key]
		if !held || issue.Status != "open" {
			continue
		}
		slot, err := model.ReadNotificationSlot(key, string(raw))
		if err != nil {
			out.Skipped++
			continue
		}
		out.Requests = append(out.Requests, model.AttentionRequest{
			IssueUID: issue.UID, ProjectID: issue.ProjectID, ProjectName: issue.ProjectName, ShortID: issue.ShortID,
			Title: issue.Title, Updated: issue.UpdatedAt, Slot: slot,
		})
	}
	model.OrderAttention(out.Requests)
	return out, nil
}

// ClearAttention clears the request at cmd.Key through the guarded metadata patch the Dates card uses, guarded on the value the
// Page showed, as the Pane's actor. A slot that no longer holds that value is left alone and reported as changed.
func (c *Client) ClearAttention(ctx context.Context, cmd model.ClearAttention) (model.AttentionCleared, error) {
	seen := cmd.Seen
	if _, err := c.patchMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, cmd.Key, nil, &seen); err != nil {
		if daemonCode(err) == "metadata_guard_failed" {
			return model.AttentionCleared{UID: cmd.UID, Changed: true}, nil
		}
		return model.AttentionCleared{UID: cmd.UID}, daemonError(err)
	}
	return model.AttentionCleared{UID: cmd.UID}, nil
}

// RaiseAgentAttention writes the request for cmd.Address on an Issue as `kata notify` does, as the Pane's actor and without a
// guard: it replaces an earlier request, which is how a newly waiting agent's message lands.
func (c *Client) RaiseAgentAttention(ctx context.Context, cmd model.RaiseAgentAttention) error {
	value := map[string]any{"from": cmd.Actor, "message": cmd.Message}
	return c.writeAttentionSlot(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, model.NotificationKey(cmd.Address), value)
}

// ClearAgentAttention removes the request for cmd.Address on an Issue as `kata notify --clear` does, without a guard.
func (c *Client) ClearAgentAttention(ctx context.Context, cmd model.ClearAgentAttention) error {
	return c.writeAttentionSlot(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, model.NotificationKey(cmd.Address), nil)
}

// writeAttentionSlot patches one notify slot, a nil value clearing it, with no guard.
func (c *Client) writeAttentionSlot(ctx context.Context, projectID int64, uid, actor, key string, value any) error {
	_, err := c.PatchIssueMetadataWithResponse(ctx, &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: projectID, Ref: uid},
		Body:       &g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{key: value}},
	})
	return daemonError(err)
}
