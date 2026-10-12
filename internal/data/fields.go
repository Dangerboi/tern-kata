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
		r, e := c.AddLabelWithResponse(ctx, &g.AddLabelRequestOptions{PathParams: &g.AddLabelPath{ProjectID: strconv.FormatInt(cmd.ProjectID, 10), Ref: cmd.UID}, Body: &g.AddLabelBody{Actor: actor, Label: cmd.Value}})
		if r != nil && e != nil {
			return ReadError(r.Body, e)
		}
		return e
	case model.RemoveLabelAction:
		r, e := c.RemoveLabelWithResponse(ctx, &g.RemoveLabelRequestOptions{PathParams: &g.RemoveLabelPath{ProjectID: cmd.ProjectID, Ref: cmd.UID, Label: cmd.Value}, Query: &g.RemoveLabelQuery{Actor: actor}})
		if r != nil && e != nil {
			return ReadError(r.Body, e)
		}
		return e
	case model.AssignOwnerAction:
		r, e := c.AssignIssueWithResponse(ctx, &g.AssignIssueRequestOptions{PathParams: &g.AssignIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.AssignIssueBody{Actor: actor, Owner: cmd.Value}})
		if r != nil && e != nil {
			return ReadError(r.Body, e)
		}
		return e
	case model.ClearOwnerAction:
		r, e := c.UnassignIssueWithResponse(ctx, &g.UnassignIssueRequestOptions{PathParams: &g.UnassignIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.UnassignIssueBody{Actor: actor}})
		if r != nil && e != nil {
			return ReadError(r.Body, e)
		}
		return e
	case model.PriorityAction:
		var priority *int64
		if cmd.Value != "-" {
			n, e := strconv.ParseInt(cmd.Value, 10, 64)
			if e != nil {
				return e
			}
			priority = &n
		}
		r, e := c.SetIssuePriorityWithResponse(ctx, &g.SetIssuePriorityRequestOptions{PathParams: &g.SetIssuePriorityPath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.SetIssuePriorityBody{Actor: actor, Priority: priority}})
		if r != nil && e != nil {
			return ReadError(r.Body, e)
		}
		return e
	}
	return nil
}
func (c *Client) FieldLabels(ctx context.Context, cmd model.LoadFieldLabels) (model.FieldLabelsLoaded, error) {
	r, e := c.ListLabelsWithResponse(ctx, &g.ListLabelsRequestOptions{PathParams: &g.ListLabelsPath{ProjectID: cmd.ProjectID}})
	if e != nil {
		if r != nil {
			e = ReadError(r.Body, e)
		}
		return model.FieldLabelsLoaded{}, e
	}
	labels := []string{}
	if r.JSON200 != nil {
		for _, l := range r.JSON200.Labels {
			labels = append(labels, l.Label)
		}
	}
	return model.FieldLabelsLoaded{UID: cmd.UID, Labels: labels}, nil
}
