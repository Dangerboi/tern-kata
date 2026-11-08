package data

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
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
	changed, err := c.clearSlot(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, cmd.Key, cmd.Seen)
	return model.AttentionCleared{UID: cmd.UID, Changed: changed}, err
}

// RaiseAgentAttention writes the request for cmd.Address on an Issue as `kata notify` does, as the Pane's actor, naming the waiting agent as teammate when it has a name, and without a
// guard: it replaces an earlier request, which is how a newly waiting agent's message lands.
func (c *Client) RaiseAgentAttention(ctx context.Context, cmd model.RaiseAgentAttention) error {
	value := notifyValue(cmd.Actor, cmd.Message, "", cmd.Teammate)
	return daemonError(c.writeMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, model.NotificationKey(cmd.Address), value))
}

// HandOffAgent writes what an agent that stops without closing its Issue writes: work.attention = needs-human and work.attention_msg
// = the reason, then, when asked, the label ready-for-agent removed and ready-for-human added, as the Pane's actor. It stops at the
// first write that fails.
func (c *Client) HandOffAgent(ctx context.Context, cmd model.HandOffAgent) error {
	for _, kv := range [][2]string{{model.WorkAttentionKey, model.WorkAttentionNeedsHuman}, {model.WorkAttentionMsgKey, cmd.Message}} {
		if err := c.writeMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, kv[0], kv[1]); err != nil {
			return daemonError(err)
		}
	}
	if !cmd.SwapLabels {
		return nil
	}
	for _, step := range []struct {
		action model.Action
		label  string
	}{{model.RemoveLabelAction, "ready-for-agent"}, {model.AddLabelAction, "ready-for-human"}} {
		if _, err := c.writeField(ctx, model.WriteField{ProjectID: cmd.ProjectID, UID: cmd.UID, Actor: cmd.Actor, Action: step.action, Value: step.label}); err != nil {
			return daemonError(err)
		}
	}
	return nil
}

// ClearAgentAttention removes the request for cmd.Address on an Issue as `kata notify --clear` does, but only while it still
// reads as cmd.Actor wrote it (whatever its teammate) with one of cmd.Messages, and guarded on the value read, so a request someone else wrote since survives. A
// slot that is gone, someone else's, or rewritten between the read and the write counts as cleared.
func (c *Client) ClearAgentAttention(ctx context.Context, cmd model.ClearAgentAttention) error {
	key := model.NotificationKey(cmd.Address)
	// A slot that does not read as a request, or reads as someone else's, is not this Pane's to clear.
	return c.clearSlotIf(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, key, func(raw string) bool {
		slot, err := model.ReadNotificationSlot(key, raw)
		return err == nil && slot.From == cmd.Actor && slices.Contains(cmd.Messages, slot.Message)
	})
}

// SendAgentMessage writes a message to an agent's Agent address on an Issue as `kata notify` does, as cmd.Actor (the Pane's actor,
// or an agent's) with cmd.Teammate on the slot when set, guarded on the address holding nothing or, when the person agreed to
// replace it, the value they saw. A guard that fails reads the slot again: a value there is held and nothing is written; a slot gone
// since is written as an empty one.
func (c *Client) SendAgentMessage(ctx context.Context, cmd model.SendAgentMessage) (model.AgentMessageSent, error) {
	key := model.NotificationKey(cmd.Address)
	value := notifyValue(cmd.Actor, cmd.Message, cmd.Re, cmd.Teammate)
	sent := model.AgentMessageSent{UID: cmd.UID, Address: cmd.Address}
	var expect *string
	if cmd.Replacing != "" {
		expect = &cmd.Replacing
	}
	for range guardedAttempts {
		_, err := c.patchMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, key, value, expect)
		if daemonCode(err) != "metadata_guard_failed" {
			return sent, daemonError(err)
		}
		raw, held, err := c.readSlot(ctx, cmd.ProjectID, cmd.UID, key)
		if err != nil {
			return sent, err
		}
		if held && (expect == nil || raw != *expect) {
			sent.Held = raw
			if slot, err := model.ReadNotificationSlot(key, raw); err == nil {
				// The first line, with control characters made spaces: both are shown to people, and text another actor wrote must not steer a terminal.
				first, _, _ := strings.Cut(strings.TrimSpace(slot.Message), "\n")
				sent.Unread, sent.UnreadFrom = model.PlainText(first), model.PlainText(slot.From)
			}
			return sent, nil
		}
		expect = nil
		if held {
			expect = &raw
		}
	}
	return sent, errors.New("what kata holds for the agent kept changing; send the message again")
}

// ClearAgentAddress removes whatever is left at an exited agent's Agent address on an Issue, whoever wrote it: nobody reads that
// address again. The clear is guarded on the value read, so a write that lands in between stays.
func (c *Client) ClearAgentAddress(ctx context.Context, cmd model.ClearAgentAddress) error {
	return c.clearSlotIf(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, model.NotificationKey(cmd.Address), func(string) bool { return true })
}

// A slot that is gone, or rewritten between the read and the write, counts as cleared.
func (c *Client) clearSlotIf(ctx context.Context, projectID int64, uid, actor, key string, mine func(raw string) bool) error {
	raw, held, err := c.readSlot(ctx, projectID, uid, key)
	if err != nil || !held || !mine(raw) {
		return err
	}
	_, err = c.clearSlot(ctx, projectID, uid, actor, key, raw)
	return err
}

func (c *Client) readSlot(ctx context.Context, projectID int64, uid, key string) (raw string, held bool, err error) {
	metadata, _, err := c.readMetadata(ctx, projectID, uid)
	value, held := metadata[key]
	return string(value), held, err
}

// clearSlot removes the slot under key while it still holds seen. A slot rewritten since is left alone and reported as changed.
func (c *Client) clearSlot(ctx context.Context, projectID int64, uid, actor, key, seen string) (changed bool, err error) {
	if _, err := c.patchMetadataKey(ctx, projectID, uid, actor, key, nil, &seen); err != nil {
		if daemonCode(err) == "metadata_guard_failed" {
			return true, nil
		}
		return false, daemonError(err)
	}
	return false, nil
}

func notifyValue(from, message, re, teammate string) map[string]any {
	value := map[string]any{"from": from, "message": message}
	if re != "" {
		value["re"] = re
	}
	if teammate != "" {
		value["teammate"] = teammate
	}
	return value
}

func (c *Client) ReadSweepComments(ctx context.Context, cmd model.ReadSweepComments) (model.SweepCommentsRead, error) {
	shown, err := c.showIssue(ctx, cmd.ProjectID, cmd.UID)
	if err != nil {
		return model.SweepCommentsRead{}, daemonError(err)
	}
	read := model.SweepCommentsRead{UID: cmd.UID, Address: cmd.Address}
	for _, comment := range shown.Comments {
		if comment.Teammate != nil && *comment.Teammate == cmd.Teammate && !comment.CreatedAt.Before(cmd.Since) {
			read.Commented = true
		}
	}
	return read, nil
}

func (c *Client) ReadAgentIssue(ctx context.Context, cmd model.ReadAgentIssue) (model.AgentIssueRead, error) {
	shown, err := c.showIssue(ctx, cmd.ProjectID, cmd.UID)
	if err != nil {
		return model.AgentIssueRead{}, daemonError(err)
	}
	return model.AgentIssueRead{UID: cmd.UID, Status: shown.Issue.Status}, nil
}
