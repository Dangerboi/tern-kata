package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// localFailure is a failure before anything was sent to the daemon.
type localFailure struct{ error }

func (e localFailure) Unwrap() error { return e.error }

// ambiguousWrite says a write that was sent failed in a way that leaves its outcome unknown: it may have been stored. A refusal
// kata answered with a 4xx, a failed dial and a failure before sending are not ambiguous.
func ambiguousWrite(err error) bool {
	if err == nil || unreachable(err) {
		return false
	}
	if _, local := errors.AsType[localFailure](err); local {
		return false
	}
	status := client.StatusCode(err)
	if envelope, ok := errors.AsType[g.ErrorEnvelope](err); ok && status == 0 {
		status = int(envelope.Status)
	}
	return status < 400 || status >= 500
}

// daemonAccount is who the daemon says it is and who it says the request acts as.
type daemonAccount struct {
	instanceUID, principal string
	closeRequiresEvidence  bool
}

func (c *Client) account(ctx context.Context) (daemonAccount, error) {
	r, err := c.InstanceWithResponse(ctx)
	if err != nil {
		return daemonAccount{}, err
	}
	principal, err := json.Marshal(r.JSON200.Auth)
	if err != nil {
		return daemonAccount{}, err
	}
	required := r.JSON200.Auth.CloseRequiresEvidence != nil && *r.JSON200.Auth.CloseRequiresEvidence
	return daemonAccount{instanceUID: r.JSON200.InstanceUID, principal: string(principal), closeRequiresEvidence: required}, nil
}

func (c *Client) showIssue(ctx context.Context, projectID int64, uid string) (*g.ShowIssueResponseBody, error) {
	r, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: uid}})
	if err != nil {
		return nil, err
	}
	if r.JSON200 == nil {
		return nil, errors.New("kata returned no Issue")
	}
	return r.JSON200, nil
}

func stateOfShown(i g.ShowIssueOut) model.IssueState {
	return model.IssueState{Status: i.Status, ClosedReason: i.ClosedReason, Owner: i.Owner, Priority: i.Priority, Body: i.Body, Revision: i.Revision}
}

func stateOfIssue(i g.Issue) model.IssueState {
	return model.IssueState{Status: i.Status, ClosedReason: i.ClosedReason, Owner: i.Owner, Priority: i.Priority, Body: i.Body, Revision: i.Revision}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// recorder reads the account and the Issue before a write and returns the function that, given the write's answer and whether its
// failure left the outcome unknown, says what the write did to the Undo history: an entry that takes it back, the reason it
// cannot be taken back, or that its outcome is unknown. fill adds what is particular to the kind of change.
func (c *Client) recorder(ctx context.Context, kind model.UndoKind, projectID int64, uid, actor string) func(res issueWrite, unknown bool, fill ...func(*model.UndoEntry)) model.WriteRecord {
	account, accountErr := c.account(ctx)
	before, beforeErr := c.showIssue(ctx, projectID, uid)
	return func(res issueWrite, unknown bool, fill ...func(*model.UndoEntry)) model.WriteRecord {
		switch {
		case unknown:
			return model.WriteRecord{Unknown: true}
		case !res.Changed:
			return model.WriteRecord{}
		case accountErr != nil || beforeErr != nil || account.instanceUID == "":
			return model.WriteRecord{Boundary: "the Issue could not be read before the change"}
		case res.Issue == nil:
			return model.WriteRecord{Boundary: "mutation response did not identify the changed issue"}
		case res.Issue.UID != before.Issue.UID || res.Issue.ProjectID != before.Issue.ProjectID:
			return model.WriteRecord{Boundary: "mutation response identified a different issue"}
		}
		if reason := undoBoundary(kind, before, res); reason != "" {
			return model.WriteRecord{Boundary: reason}
		}
		entry := model.UndoEntry{
			Kind: kind, UID: before.Issue.UID, ProjectID: projectID, ProjectUID: derefString(before.Issue.ProjectUID),
			ShortID: before.Issue.ShortID, Actor: actor, InstanceUID: account.instanceUID, Principal: account.principal,
			Before: stateOfShown(before.Issue), After: stateOfIssue(*res.Issue), Revision: res.Issue.Revision,
		}
		if kind == model.UndoLaneMove {
			entry.Labels = before.Issue.Labels
		}
		if kind == model.UndoLinkAdd {
			entry.Link = &model.UndoLink{ID: res.Link.ID, Type: res.Link.Type, FromUID: res.Link.From.UID, ToUID: res.Link.To.UID}
		}
		for _, f := range fill {
			f(&entry)
		}
		return model.WriteRecord{Entry: &entry}
	}
}

// undoBoundary is why a change that completed cannot be taken back, "" when it can.
func undoBoundary(kind model.UndoKind, before *g.ShowIssueResponseBody, res issueWrite) string {
	issue := before.Issue
	switch kind {
	case model.UndoOwner:
		if issue.AssignmentExpiresOn != nil || res.Issue.AssignmentExpiresOn != nil {
			return "owner edit involving an expiry cannot be undone"
		}
	case model.UndoClose:
		if issue.RecurrenceID != nil {
			return "closing a recurring issue cannot be undone"
		}
		if before.Lease != nil {
			return "closing a leased issue cannot be undone"
		}
	case model.UndoReopen:
		if issue.RecurrenceID != nil {
			return "reopening a recurring issue cannot be undone"
		}
		if issue.ClosedReason == nil || *issue.ClosedReason != "done" {
			return "reopening an issue not closed as done cannot be undone"
		}
	case model.UndoLinkAdd:
		link := res.Link
		if link == nil || link.ID == 0 || link.From.UID == "" || link.To.UID == "" {
			return "link response did not identify the created link"
		}
		if link.From.UID != issue.UID && !(link.Type == "related" && link.To.UID == issue.UID) {
			return "link response did not match the created link"
		}
	}
	return ""
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameStatus(i g.ShowIssueOut, s model.IssueState) bool {
	return i.Status == s.Status && equalPtr(i.ClosedReason, s.ClosedReason)
}

func sameLinkEndpoints(l g.LinkOut, e *model.UndoLink) bool {
	return e != nil && l.Type == e.Type && l.From.UID == e.FromUID && l.To.UID == e.ToUID
}

// dateOf is the day an Issue holds under a metadata key, "" when it holds none.
func dateOf(i g.ShowIssueOut, key string) string {
	s, _ := i.Metadata[key].(string)
	return s
}

// restored says the Issue already holds what the entry's change took away.
func restored(e model.UndoEntry, shown *g.ShowIssueResponseBody) bool {
	i := shown.Issue
	switch e.Kind {
	case model.UndoClose, model.UndoReopen:
		return sameStatus(i, e.Before)
	case model.UndoDelete:
		return i.DeletedAt == nil
	case model.UndoOwner:
		return equalPtr(i.Owner, e.Before.Owner)
	case model.UndoPriority:
		return equalPtr(i.Priority, e.Before.Priority)
	case model.UndoBody:
		return i.Body == e.Before.Body
	case model.UndoLabelAdd:
		return !slices.Contains(i.Labels, e.Label)
	case model.UndoLabelRemove:
		return slices.Contains(i.Labels, e.Label)
	case model.UndoLinkAdd:
		for _, l := range shown.Links {
			if sameLinkEndpoints(l, e.Link) {
				return false
			}
		}
		return e.Link != nil && (e.Link.Type != "parent" || shown.Parent == nil)
	case model.UndoLaneMove:
		if e.Labels != nil {
			return sameLabelSet(i.Labels, undoneLabels(e))
		}
		for _, label := range e.Added {
			if slices.Contains(i.Labels, label) {
				return false
			}
		}
		for _, label := range e.Removed {
			if !slices.Contains(i.Labels, label) {
				return false
			}
		}
		return true
	case model.UndoDates:
		for _, d := range e.Dates {
			if dateOf(i, d.Key) != d.Was {
				return false
			}
		}
		return true
	}
	return false
}

// undoneLabels is the label set a Lane Move's undo leaves: the set the move left, without the labels it added and with the
// ones it removed.
func undoneLabels(e model.UndoEntry) []string {
	out := slices.DeleteFunc(slices.Clone(e.Labels), func(l string) bool { return slices.Contains(e.Added, l) })
	for _, l := range e.Removed {
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

func sameLabelSet(a, b []string) bool {
	// Sorted copies, so a repeated label cannot stand in for a missing one.
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// laneMoveReachable says the labels are the set the move left or a state its inverse passes through: the old labels added
// back first, then the new ones removed. Anything else was written by someone else.
func laneMoveReachable(e model.UndoEntry, labels []string) bool {
	for _, l := range labels {
		if !slices.Contains(e.Labels, l) && !slices.Contains(e.Removed, l) {
			return false
		}
	}
	backAll := true
	for _, l := range e.Removed {
		backAll = backAll && slices.Contains(labels, l)
	}
	for _, l := range e.Labels {
		if !slices.Contains(labels, l) && (!backAll || !slices.Contains(e.Added, l)) {
			return false
		}
	}
	return true
}

// moved is the refusal when the Issue is no longer as the entry's change left it, "" when it is.
func moved(e model.UndoEntry, shown *g.ShowIssueResponseBody) string {
	i := shown.Issue
	atRevision := i.Revision == e.Revision
	switch e.Kind {
	case model.UndoClose, model.UndoReopen:
		if !sameStatus(i, e.After) || !atRevision {
			return "issue status changed since the action"
		}
		if i.RecurrenceID != nil || shown.Lease != nil {
			return "issue now has recurrence or a live lease"
		}
	case model.UndoOwner:
		if !equalPtr(i.Owner, e.After.Owner) || !atRevision {
			return "issue owner changed since the action"
		}
	case model.UndoPriority:
		if !equalPtr(i.Priority, e.After.Priority) || !atRevision {
			return "issue priority changed since the action"
		}
	case model.UndoBody:
		if i.Body != e.After.Body || !atRevision {
			return "issue body changed since the action"
		}
	case model.UndoLabelAdd, model.UndoLabelRemove:
		if !atRevision {
			return "issue changed since the label edit"
		}
	case model.UndoLaneMove:
		// Label edits do not move the revision, so any other write does. An undo whose second label write was refused leaves the
		// card with its old Lane labels back and the new ones still on (the inverse adds before it removes), so a retry is accepted
		// from there. A Lane label gone with the old ones not all back is not a state the inverse passes through: someone else
		// moved the card on.
		if e.Labels != nil {
			if !atRevision || !laneMoveReachable(e, i.Labels) {
				return "issue changed since the Lane move"
			}
			break
		}
		restoredBack, removedNow := 0, 0
		for _, label := range e.Removed {
			if slices.Contains(i.Labels, label) {
				restoredBack++
			}
		}
		for _, label := range e.Added {
			if !slices.Contains(i.Labels, label) {
				removedNow++
			}
		}
		if !atRevision || (removedNow > 0 && restoredBack < len(e.Removed)) {
			return "issue changed since the Lane move"
		}
	case model.UndoLinkAdd:
		found, replaced := false, false
		for _, l := range shown.Links {
			if !sameLinkEndpoints(l, e.Link) {
				continue
			}
			if l.ID == e.Link.ID {
				found = true
			} else {
				replaced = true
			}
		}
		if !found || replaced || !atRevision {
			return "created link changed since the action"
		}
	case model.UndoDates:
		for _, d := range e.Dates {
			if now := dateOf(i, d.Key); now != d.Value && now != d.Was {
				return "issue dates changed since the action"
			}
		}
	}
	return ""
}

// Undo takes back the change of cmd.Entry, as kata tui does: the same daemon, account and Issue, the Issue as the change left
// it, and the inverse sent through the calls the Pane's own writes use.
func (c *Client) Undo(ctx context.Context, cmd model.UndoChange) model.ChangeUndone {
	e := cmd.Entry
	out := model.ChangeUndone{EntryID: e.ID, Result: model.UndoFailed}
	account, err := c.account(ctx)
	if err != nil {
		out.Err = daemonError(err)
		return out
	}
	conflict := func(reason string) model.ChangeUndone {
		out.Result, out.Conflict, out.Err = model.UndoConflict, reason, nil
		return out
	}
	if account.instanceUID == "" || account.instanceUID != e.InstanceUID {
		return conflict("daemon instance changed")
	}
	if account.principal != e.Principal {
		return conflict("daemon principal changed")
	}
	shown, err := c.showForEntry(ctx, e)
	if err != nil {
		out.Err = daemonError(err)
		return out
	}
	i := shown.Issue
	out.Ref, out.FromRevision = i.ShortID, i.Revision
	if i.UID != e.UID || i.ProjectID != e.ProjectID || (e.ProjectUID != "" && derefString(i.ProjectUID) != e.ProjectUID) {
		return conflict("issue identity changed")
	}
	if restored(e, shown) {
		out.Result = model.UndoAlready
		return out
	}
	if reason := moved(e, shown); reason != "" {
		return conflict(reason)
	}
	if e.Kind == model.UndoReopen && account.closeRequiresEvidence && cmd.Close == nil {
		out.Result = model.UndoNeedsEvidence
		return out
	}
	res, err := c.invert(ctx, cmd, account, shown)
	if err != nil {
		if ambiguousWrite(err) {
			out.Result = model.UndoUnknown
		}
		out.Err = daemonError(err)
		return out
	}
	if !res.Changed {
		after, err := c.showForEntry(ctx, e)
		switch {
		case err != nil:
			out.Err = daemonError(err)
		case after.Issue.UID != e.UID:
			out.Err = errors.New("undo readback returned a different issue")
		case restored(e, after):
			out.Result = model.UndoAlready
		default:
			out.Err = errors.New("undo returned no change; issue still needs correction")
		}
		return out
	}
	out.Result = model.UndoChanged
	if res.Issue != nil {
		out.ToRevision = res.Issue.Revision
	}
	return out
}

// invert sends the inverse of the entry's change.
func (c *Client) invert(ctx context.Context, cmd model.UndoChange, account daemonAccount, shown *g.ShowIssueResponseBody) (issueWrite, error) {
	e := cmd.Entry
	field := model.WriteField{UID: e.UID, ProjectID: e.ProjectID, Actor: e.Actor}
	switch e.Kind {
	case model.UndoClose:
		return c.reopenRaw(ctx, model.ReopenIssueCmd{UID: e.UID, ProjectID: e.ProjectID, Actor: e.Actor})
	case model.UndoDelete:
		return c.restoreRaw(ctx, e.ProjectID, e.UID, e.Actor)
	case model.UndoReopen:
		closing := model.CloseIssueCmd{UID: e.UID, ProjectID: e.ProjectID, Actor: e.Actor, Quick: true, Reason: model.ReasonDone,
			Key: fmt.Sprintf("undo-%s-%d", e.UID, shown.Issue.Revision)}
		if account.closeRequiresEvidence {
			if cmd.Close == nil {
				return issueWrite{}, localFailure{errors.New("evidence-bearing done close is required")}
			}
			closing = *cmd.Close
			closing.UID, closing.ProjectID, closing.Actor, closing.Reason, closing.Quick = e.UID, e.ProjectID, e.Actor, model.ReasonDone, false
		}
		return c.closeRaw(ctx, closing)
	case model.UndoOwner:
		field.Action = model.ClearOwnerAction
		if e.Before.Owner != nil {
			field.Action, field.Value = model.AssignOwnerAction, *e.Before.Owner
		}
		return c.writeField(ctx, field)
	case model.UndoPriority:
		field.Action, field.Value = model.PriorityAction, "-"
		if e.Before.Priority != nil {
			field.Value = fmt.Sprint(*e.Before.Priority)
		}
		return c.writeField(ctx, field)
	case model.UndoBody:
		return c.editBody(ctx, e, e.Before.Body)
	case model.UndoLabelAdd:
		field.Action, field.Value = model.RemoveLabelAction, e.Label
		return c.writeField(ctx, field)
	case model.UndoLabelRemove:
		field.Action, field.Value = model.AddLabelAction, e.Label
		return c.writeField(ctx, field)
	case model.UndoLinkAdd:
		return c.removeLink(ctx, e.ProjectID, e.UID, e.Actor, e.Link.ID)
	case model.UndoLaneMove:
		var last issueWrite
		for _, step := range slices.Concat(laneSteps(model.AddLabelAction, e.Removed), laneSteps(model.RemoveLabelAction, e.Added)) {
			// A retry after a partial undo: the label is already where the step would put it.
			if slices.Contains(shown.Issue.Labels, step.label) != (step.action == model.RemoveLabelAction) {
				continue
			}
			field.Action, field.Value = step.action, step.label
			res, err := c.writeField(ctx, field)
			if err != nil {
				return last, err
			}
			if res.Changed {
				last = res
			}
		}
		return last, nil
	case model.UndoDates:
		var changes []model.DateChange
		for _, d := range e.Dates {
			if dateOf(shown.Issue, d.Key) != d.Was {
				changes = append(changes, model.DateChange{Key: d.Key, Label: d.Label, Value: d.Was, Was: d.Value})
			}
		}
		_, changed, last, ambiguous, err := c.setDates(ctx, model.SetDatesCmd{UID: e.UID, ProjectID: e.ProjectID, Actor: e.Actor, Changes: changes})
		last.Changed = len(changed) > 0
		if err != nil && !ambiguous {
			err = localFailure{err}
		}
		return last, err
	}
	return issueWrite{}, localFailure{fmt.Errorf("unsupported undo action %q", e.Kind)}
}

type laneStep struct {
	action model.Action
	label  string
}

func laneSteps(action model.Action, labels []string) []laneStep {
	steps := make([]laneStep, len(labels))
	for n, label := range labels {
		steps[n] = laneStep{action, label}
	}
	return steps
}

func (c *Client) editBody(ctx context.Context, e model.UndoEntry, body string) (issueWrite, error) {
	actor := e.Actor
	r, err := c.EditIssueWithResponse(ctx, &g.EditIssueRequestOptions{PathParams: &g.EditIssuePath{ProjectID: fmt.Sprint(e.ProjectID), Ref: e.UID}, Body: &g.EditIssueBody{Body: &body, Actor: &actor}})
	if err != nil {
		return issueWrite{}, err
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}
