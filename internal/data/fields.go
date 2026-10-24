package data

import (
	"context"
	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
	"strconv"
)

func (c *Client) WriteField(ctx context.Context, cmd model.WriteField) error {
	actor := &cmd.Actor
	switch cmd.Action {
	case model.AddLabelAction:
		_, e := c.AddLabelWithResponse(ctx, &g.AddLabelRequestOptions{PathParams: &g.AddLabelPath{ProjectID: strconv.FormatInt(cmd.ProjectID, 10), Ref: cmd.UID}, Body: &g.AddLabelBody{Actor: actor, Label: cmd.Value}})
		return daemonError(e)
	case model.RemoveLabelAction:
		_, e := c.RemoveLabelWithResponse(ctx, &g.RemoveLabelRequestOptions{PathParams: &g.RemoveLabelPath{ProjectID: cmd.ProjectID, Ref: cmd.UID, Label: cmd.Value}, Query: &g.RemoveLabelQuery{Actor: actor}})
		return daemonError(e)
	case model.AssignOwnerAction:
		_, e := c.AssignIssueWithResponse(ctx, &g.AssignIssueRequestOptions{PathParams: &g.AssignIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.AssignIssueBody{Actor: actor, Owner: cmd.Value}})
		return daemonError(e)
	case model.ClearOwnerAction:
		_, e := c.UnassignIssueWithResponse(ctx, &g.UnassignIssueRequestOptions{PathParams: &g.UnassignIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.UnassignIssueBody{Actor: actor}})
		return daemonError(e)
	case model.PriorityAction:
		var priority *int64
		if cmd.Value != "-" {
			n, e := strconv.ParseInt(cmd.Value, 10, 64)
			if e != nil {
				return e
			}
			priority = &n
		}
		_, e := c.SetIssuePriorityWithResponse(ctx, &g.SetIssuePriorityRequestOptions{PathParams: &g.SetIssuePriorityPath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.SetIssuePriorityBody{Actor: actor, Priority: priority}})
		return daemonError(e)
	case model.SetParentAction:
		_, e := c.CreateLinkWithResponse(ctx, &g.CreateLinkRequestOptions{PathParams: &g.CreateLinkPath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.CreateLinkBody{Actor: actor, Type: g.CreateLinkRequestBodyTypeParent, ToRef: cmd.Value}})
		return daemonError(e)
	case model.AddBlockerAction, model.AddRelatedAction:
		linkType := g.CreateLinkRequestBodyTypeBlocks
		if cmd.Action == model.AddRelatedAction {
			linkType = g.CreateLinkRequestBodyTypeRelated
		}
		_, e := c.CreateLinkWithResponse(ctx, &g.CreateLinkRequestOptions{PathParams: &g.CreateLinkPath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.CreateLinkBody{Actor: actor, Type: linkType, ToRef: cmd.Value}})
		return daemonError(e)
	}
	return nil
}

// RemoveLink deletes the link with the id from the Issue it is on.
func (c *Client) RemoveLink(ctx context.Context, cmd model.RemoveLinkCmd) error {
	_, e := c.DeleteLinkWithResponse(ctx, &g.DeleteLinkRequestOptions{PathParams: &g.DeleteLinkPath{ProjectID: cmd.ProjectID, Ref: cmd.UID, LinkID: cmd.LinkID}, Query: &g.DeleteLinkQuery{Actor: cmd.Actor}})
	return daemonError(e)
}

// Move adds the label to add, then removes each label to remove, and stops at the first failure.
func (c *Client) Move(ctx context.Context, cmd model.MoveIssueCmd) error {
	write := model.WriteField{Workspace: cmd.Workspace, UID: cmd.UID, ProjectID: cmd.ProjectID, Actor: cmd.Actor}
	if cmd.Add != "" {
		write.Action, write.Value = model.AddLabelAction, cmd.Add
		if err := c.WriteField(ctx, write); err != nil {
			return err
		}
	}
	write.Action = model.RemoveLabelAction
	for _, label := range cmd.Remove {
		write.Value = label
		if err := c.WriteField(ctx, write); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) FieldLabels(ctx context.Context, cmd model.LoadFieldLabels) (model.FieldLabelsLoaded, error) {
	r, e := c.ListLabelsWithResponse(ctx, &g.ListLabelsRequestOptions{PathParams: &g.ListLabelsPath{ProjectID: cmd.ProjectID}})
	if e != nil {
		return model.FieldLabelsLoaded{}, daemonError(e)
	}
	labels := []string{}
	if r.JSON200 != nil {
		for _, l := range r.JSON200.Labels {
			labels = append(labels, l.Label)
		}
	}
	return model.FieldLabelsLoaded{UID: cmd.UID, Labels: labels}, nil
}
