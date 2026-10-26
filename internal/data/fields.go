package data

import (
	"context"
	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
	"slices"
	"strconv"
)

// issueWrite is what a write to an Issue answered: whether it changed the Issue, the Issue after it, and the link it made.
type issueWrite struct {
	Changed bool
	Issue   *g.Issue
	Link    *g.LinkOut
}

func written(changed bool, issue g.Issue, link *g.LinkOut) issueWrite {
	return issueWrite{Changed: changed, Issue: &issue, Link: link}
}

// writeField is WriteField with kata's own error and answer.
func (c *Client) writeField(ctx context.Context, cmd model.WriteField) (issueWrite, error) {
	actor := &cmd.Actor
	switch cmd.Action {
	case model.AddLabelAction:
		r, e := c.AddLabelWithResponse(ctx, &g.AddLabelRequestOptions{PathParams: &g.AddLabelPath{ProjectID: strconv.FormatInt(cmd.ProjectID, 10), Ref: cmd.UID}, Body: &g.AddLabelBody{Actor: actor, Label: cmd.Value}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
	case model.RemoveLabelAction:
		r, e := c.RemoveLabelWithResponse(ctx, &g.RemoveLabelRequestOptions{PathParams: &g.RemoveLabelPath{ProjectID: cmd.ProjectID, Ref: cmd.UID, Label: cmd.Value}, Query: &g.RemoveLabelQuery{Actor: actor}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
	case model.AssignOwnerAction:
		r, e := c.AssignIssueWithResponse(ctx, &g.AssignIssueRequestOptions{PathParams: &g.AssignIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.AssignIssueBody{Actor: actor, Owner: cmd.Value}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
	case model.TimedAssignAction:
		seconds, e := strconv.ParseInt(cmd.Value, 10, 64)
		if e != nil {
			return issueWrite{}, localFailure{e}
		}
		r, e := c.ClaimIssueWithResponse(ctx, &g.ClaimIssueRequestOptions{PathParams: &g.ClaimIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.ClaimIssueBody{Actor: actor, TTLSeconds: &seconds}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
	case model.ClearOwnerAction:
		r, e := c.UnassignIssueWithResponse(ctx, &g.UnassignIssueRequestOptions{PathParams: &g.UnassignIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.UnassignIssueBody{Actor: actor}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
	case model.PriorityAction:
		var priority *int64
		if cmd.Value != "-" {
			n, e := strconv.ParseInt(cmd.Value, 10, 64)
			if e != nil {
				return issueWrite{}, localFailure{e}
			}
			priority = &n
		}
		r, e := c.SetIssuePriorityWithResponse(ctx, &g.SetIssuePriorityRequestOptions{PathParams: &g.SetIssuePriorityPath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.SetIssuePriorityBody{Actor: actor, Priority: priority}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
	case model.SetParentAction, model.AddBlockerAction, model.AddRelatedAction:
		linkType := g.CreateLinkRequestBodyTypeParent
		switch cmd.Action {
		case model.AddBlockerAction:
			linkType = g.CreateLinkRequestBodyTypeBlocks
		case model.AddRelatedAction:
			linkType = g.CreateLinkRequestBodyTypeRelated
		}
		r, e := c.CreateLinkWithResponse(ctx, &g.CreateLinkRequestOptions{PathParams: &g.CreateLinkPath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.CreateLinkBody{Actor: actor, Type: linkType, ToRef: cmd.Value}})
		if e != nil {
			return issueWrite{}, e
		}
		return written(r.JSON200.Changed, r.JSON200.Issue, &r.JSON200.Link), nil
	}
	return issueWrite{}, nil
}

func (c *Client) WriteField(ctx context.Context, cmd model.WriteField) error {
	_, e := c.writeField(ctx, cmd)
	return daemonError(e)
}

// fieldUndoKinds are the field writes that have an inverse.
var fieldUndoKinds = map[model.Action]model.UndoKind{
	model.AddLabelAction:    model.UndoLabelAdd,
	model.RemoveLabelAction: model.UndoLabelRemove,
	model.AssignOwnerAction: model.UndoOwner,
	model.ClearOwnerAction:  model.UndoOwner,
	model.PriorityAction:    model.UndoPriority,
	model.SetParentAction:   model.UndoLinkAdd,
	model.AddBlockerAction:  model.UndoLinkAdd,
	model.AddRelatedAction:  model.UndoLinkAdd,
}

// WriteFieldRecorded is WriteField that also says what the write did to the Undo history.
func (c *Client) WriteFieldRecorded(ctx context.Context, cmd model.WriteField) (model.WriteRecord, error) {
	kind, undoable := fieldUndoKinds[cmd.Action]
	if !undoable {
		_, e := c.writeField(ctx, cmd)
		return model.WriteRecord{Unknown: ambiguousWrite(e)}, daemonError(e)
	}
	done := c.recorder(ctx, kind, cmd.ProjectID, cmd.UID, cmd.Actor)
	res, e := c.writeField(ctx, cmd)
	return done(res, ambiguousWrite(e), func(entry *model.UndoEntry) { entry.Label = cmd.Value }), daemonError(e)
}

// removeLink deletes the link with the id from the Issue, with kata's own error and answer.
func (c *Client) removeLink(ctx context.Context, projectID int64, uid, actor string, linkID int64) (issueWrite, error) {
	r, e := c.DeleteLinkWithResponse(ctx, &g.DeleteLinkRequestOptions{PathParams: &g.DeleteLinkPath{ProjectID: projectID, Ref: uid, LinkID: linkID}, Query: &g.DeleteLinkQuery{Actor: actor}})
	if e != nil {
		return issueWrite{}, e
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}

// move adds the label to add, then removes each label to remove, and stops at the first failure. It returns the labels it
// changed and the Issue after the last change.
func (c *Client) move(ctx context.Context, cmd model.MoveIssueCmd) (added, removed []string, last issueWrite, err error) {
	write := model.WriteField{Workspace: cmd.Workspace, UID: cmd.UID, ProjectID: cmd.ProjectID, Actor: cmd.Actor}
	step := func(action model.Action, label string) bool {
		write.Action, write.Value = action, label
		res, e := c.writeField(ctx, write)
		if e != nil {
			err = e
			return false
		}
		if res.Changed {
			last = res
			if action == model.AddLabelAction {
				added = append(added, label)
			} else {
				removed = append(removed, label)
			}
		}
		return true
	}
	if cmd.Add != "" && !step(model.AddLabelAction, cmd.Add) {
		return
	}
	for _, label := range cmd.Remove {
		if !step(model.RemoveLabelAction, label) {
			return
		}
	}
	return
}

// Move adds the label to add, then removes each label to remove, and stops at the first failure.
func (c *Client) Move(ctx context.Context, cmd model.MoveIssueCmd) error {
	_, _, _, err := c.move(ctx, cmd)
	return daemonError(err)
}

// MoveRecorded is Move that also says what the Move did to the Undo history.
func (c *Client) MoveRecorded(ctx context.Context, cmd model.MoveIssueCmd) (model.WriteRecord, error) {
	done := c.recorder(ctx, model.UndoLaneMove, cmd.ProjectID, cmd.UID, cmd.Actor)
	added, removed, last, err := c.move(ctx, cmd)
	return done(last, ambiguousWrite(err), func(entry *model.UndoEntry) {
		entry.Added, entry.Removed = added, removed
		// The move's steps applied in order to the pre-move labels.
		after := slices.Clone(entry.Labels)
		for _, l := range added {
			if !slices.Contains(after, l) {
				after = append(after, l)
			}
		}
		after = slices.DeleteFunc(after, func(l string) bool { return slices.Contains(removed, l) })
		// The write answers carry no labels: read the Issue after the move so a label another actor added since the pre-move read
		// is kept. A read that does not show the move's own result (a rival changed the Lane since the last write) is not the
		// move's outcome, so the derived set stands and undo refuses.
		now, e := c.showIssue(ctx, cmd.ProjectID, cmd.UID)
		if e != nil {
			// No read, so no verified set: the derived one could hold a label a rival changed since the pre-move read and make a
			// legitimate undo refuse. Without a set, undo checks the labels the move itself added and removed.
			entry.Labels = nil
			return
		}
		if now.Issue.UID == entry.UID && now.Issue.ProjectID == cmd.ProjectID &&
			!slices.ContainsFunc(added, func(l string) bool { return !slices.Contains(now.Issue.Labels, l) }) &&
			!slices.ContainsFunc(removed, func(l string) bool { return slices.Contains(now.Issue.Labels, l) }) {
			after = now.Issue.Labels
		}
		entry.Labels = append([]string{}, after...)
	}), daemonError(err)
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
