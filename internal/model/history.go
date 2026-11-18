package model

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type VerbTone string

const (
	VerbSuccess VerbTone = "success"
	VerbMuted   VerbTone = "muted"
)

// DetailTime is the one time rule of the detail: Events, comments and the meta line read like 18:21 today and 2026-10-09 22:42
// on any other day, in the Pane calendar.
func (m Model) DetailTime(t time.Time) string {
	return m.Calendar.Read(t, m.Now)
}

// EventHidden says the Events tab does not list the event: it only lists changes to the Issue, and the Issue's meta line already
// says who created it and when. Comments live on the Comments tab, and a notification slot's rows (told someone about a reply)
// are bookkeeping, not a change to the Issue.
func EventHidden(event Event) bool {
	switch event.Type {
	case "issue.created", "issue.commented", "issue.comment_edited":
		return true
	case "issue.metadata_updated":
		payload, _ := decodePayload(event.Payload)
		diff, _ := payload["diff"].(map[string]any)
		_, notification := slotDiff(diff)
		return notification
	}
	return false
}

// historyRow phrases one event as "<actor> <verb> <object>". Only issue.closed carries a detail
// (Body: the close message and evidence); every other event stays one line. Types kata adds later
// read as their type name.
func (m Model) historyRow(event Event) DetailRow {
	payload, _ := decodePayload(event.Payload)
	verb, object, detail, tone := "", "", "", VerbTone("")
	switch event.Type {
	case "issue.created":
		verb, object = "created", "this Issue"
	case "issue.commented":
		verb = "commented"
	case "issue.comment_edited":
		verb, object = "edited", "a comment"
	case "issue.assigned":
		verb, object = "assigned", "to "+payload.text("owner")
	case "issue.unassigned":
		verb = "unassigned"
	case "issue.closed":
		verb, object, tone = "closed", closeReason(payload.text("reason")), closeTone(payload.text("reason"))
		detail = closeDetail(payload, m.currentMetadata())
	case "issue.reopened":
		verb = "reopened"
	case "issue.labeled":
		verb, object = "added label", payload.text("label")
	case "issue.unlabeled":
		verb, object = "removed label", payload.text("label")
	case "issue.priority_set":
		verb, object = "set priority to", payload.text("priority")
	case "issue.priority_cleared":
		verb = "cleared priority"
	case "issue.updated":
		verb, object = "edited", editedFields(payload)
	case "issue.linked", "issue.unlinked":
		verb = strings.TrimPrefix(event.Type, "issue.")
		object = linkObject(event, payload)
	case "issue.links_changed":
		verb, object = linksChanged(payload)
	case "issue.metadata_updated":
		verb, object = metadataChanged(payload)
	case "issue.scheduled_reached":
		verb, object = "reached", "the scheduled date "+oneLine(payload.text("value"))
	case "issue.assignment_renewed":
		verb = assignmentRenewed(payload)
	case "issue.assignment_expired":
		verb = assignmentExpired(payload)
	case "issue.moved":
		verb, object = "moved", movedTo(event, payload)
	case "issue.soft_deleted":
		verb = "deleted"
	case "issue.restored":
		verb = "restored"
	default:
		verb = strings.NewReplacer(".", " ", "_", " ").Replace(strings.TrimPrefix(event.Type, "issue."))
	}
	row := DetailRow{
		Key: event.UID, Type: event.Type, Author: event.Actor, Time: m.DetailTime(event.CreatedAt), Body: detail,
		History: &HistorySentence{Verb: verb, Object: object, Tone: tone},
	}
	if event.RelatedIssueUID != nil {
		row.ReferenceUID = *event.RelatedIssueUID
	}
	if event.RelatedIssueShortID != nil && row.ReferenceUID != "" {
		if head, ok := strings.CutSuffix(object, *event.RelatedIssueShortID); ok && *event.RelatedIssueShortID != "" {
			row.Ref, row.History.Object = *event.RelatedIssueShortID, strings.TrimRight(head, " ")
		}
	}
	return row
}

// closeReason is the object of a closed event: "as done", "as won't fix"; the reason's words in lowercase, and an unknown code as it
// is. A close with no reason reads just "closed".
func closeReason(code string) string {
	if code == "" {
		return ""
	}
	words := CloseReasonWords(code)
	if words != code {
		words = strings.ToLower(words[:1]) + words[1:]
	}
	return "as " + words
}

// closeTone is success for a close as done, muted for a close that is not a completion, and plain for a code kata adds later.
func closeTone(code string) VerbTone {
	switch {
	case code == "done":
		return VerbSuccess
	case CloseReasonWords(code) != code:
		return VerbMuted
	}
	return ""
}

// assignmentRenewed and assignmentExpired word the timed-assignment events as kata tui does, with the time as the payload has it.
func assignmentRenewed(p payloadMap) string {
	owner, until := p.text("owner"), p.text("assignment_expires_on")
	switch {
	case owner != "" && until != "":
		return "renewed assignment for " + owner + " until " + until
	case owner != "":
		return "renewed assignment for " + owner
	}
	return "renewed assignment"
}

func assignmentExpired(p payloadMap) string {
	owner, at := p.text("previous_owner"), p.text("assignment_expires_on")
	switch {
	case owner != "" && at != "":
		return "assignment for " + owner + " expired at " + at
	case owner != "":
		return "assignment for " + owner + " expired"
	}
	return "assignment expired"
}

type payloadMap map[string]any

// decodePayload fails for a payload that is not a JSON object, and then returns no fields.
func decodePayload(raw string) (payloadMap, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var payload payloadMap
	err := decoder.Decode(&payload)
	return payload, err
}

func (p payloadMap) text(key string) string {
	if p[key] == nil {
		return ""
	}
	return plain(p[key])
}

func (p payloadMap) pairs() string {
	keys := make([]string, 0, len(p))
	for key := range p {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + ": " + oneLine(plain(p[key]))
	}
	return strings.Join(parts, ", ")
}

func plain(v any) string {
	switch v := v.(type) {
	case nil:
		return "none"
	case string:
		return v
	case json.Number:
		return v.String()
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = plain(item)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		return payloadMap(v).pairs()
	}
	return fmt.Sprint(v)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// firstLine is the trimmed first line of s.
func firstLine(s string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(first)
}

func (m Model) currentMetadata() map[string]any {
	if m.authority == nil || m.authority.Issue == nil {
		return nil
	}
	return m.authority.Issue.Metadata
}

func closeDetail(p payloadMap, current map[string]any) string {
	var evidence []string
	items, _ := p["evidence"].([]any)
	for _, item := range items {
		fields, _ := item.(map[string]any)
		if plain(fields["type"]) == "attachment" {
			evidence = append(evidence, attachmentEvidence(fields, current))
			continue
		}
		parts := []string{plain(fields["type"])}
		for _, key := range []string{"sha", "url", "command", "paths", "account", "rationale", "issue_ref"} {
			if value, ok := fields[key]; ok {
				if key == "sha" {
					value = truncate(plain(value), 7)
				}
				parts = append(parts, plain(value))
				break
			}
		}
		evidence = append(evidence, strings.Join(parts, " "))
	}
	message, list := strings.TrimSpace(p.text("message")), strings.Join(evidence, ", ")
	if list != "" {
		list = "Evidence: " + list
	}
	switch {
	case message == "":
		return list
	case list == "":
		return message
	}
	return message + "\n\n" + list
}

// attachmentEvidence is how an attachment evidence entry reads in a close: "attachment <name>", with (changed) when the Issue
// has another version of the file now and (removed) when it no longer has the Attachment. The name comes from the copy kata froze
// into the evidence, so it survives the Attachment's removal. An entry with no copy reads from the Issue's current Attachment,
// like kata's web UI; one that has neither reads "Attachment removed", and one whose copy is no reference "Invalid attachment".
func attachmentEvidence(fields map[string]any, metadata map[string]any) string {
	id := plain(fields["attachment_id"])
	nowRef := metadata[attachmentKeyPrefix+id]
	now, nowOK := attachmentRef(id, nowRef)
	frozen, frozenOK := attachmentRef(id, fields["attachment"])
	if _, hasCopy := fields["attachment"]; !hasCopy {
		frozen, frozenOK = attachmentRef(id, nowRef)
		if !frozenOK {
			return "Attachment removed"
		}
	} else if !frozenOK {
		return "Invalid attachment"
	}
	switch {
	case !nowOK:
		return "attachment " + frozen.Name + " (removed)"
	case !strings.EqualFold(now.Version, frozen.Version):
		return "attachment " + frozen.Name + " (changed)"
	}
	return "attachment " + frozen.Name
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func editedFields(p payloadMap) string {
	var fields []string
	for _, field := range []string{"title", "body", "owner"} {
		if _, ok := p[field]; ok {
			fields = append(fields, field)
		}
	}
	return strings.Join(fields, " and ")
}

var inverseRelation = map[string]string{"parent": "child", "child": "parent", "blocks": "blocked_by", "blocked_by": "blocks"}

// relationLabel is a Links row's label: the relation in words, capitalised.
func relationLabel(relation string) string {
	words := RelationWords(relation)
	if words == "" {
		return words
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

func linkObject(event Event, p payloadMap) string {
	relation, peer := p.text("type"), p.text("to_short_id")
	if event.RelatedIssueShortID != nil {
		peer = *event.RelatedIssueShortID
	}
	if incoming, _ := p["incoming"].(bool); incoming && inverseRelation[relation] != "" {
		relation = inverseRelation[relation]
	}
	return RelationWords(relation) + " " + peer
}

type linkField struct{ field, relation string }

var (
	linkAdds    = []linkField{{"parent_set", "parent"}, {"blocks_added", "blocks"}, {"blocked_by_added", "blocked by"}, {"related_added", "related"}}
	linkRemoves = []linkField{{"parent_removed", "parent"}, {"blocks_removed", "blocks"}, {"blocked_by_removed", "blocked by"}, {"related_removed", "related"}}
)

func linksChanged(p payloadMap) (verb, object string) {
	collect := func(specs []linkField, prefix string) (parts []string) {
		for _, spec := range specs {
			value, ok := p[spec.field]
			if !ok {
				continue
			}
			refs, _ := value.([]any)
			if single, isString := value.(string); isString {
				refs = []any{single}
			}
			for _, ref := range refs {
				parts = append(parts, prefix+spec.relation+" "+plain(ref))
			}
		}
		return parts
	}
	added, removed := collect(linkAdds, ""), collect(linkRemoves, "")
	switch {
	case len(removed) == 0:
		return "linked", strings.Join(added, ", ")
	case len(added) == 0:
		return "unlinked", strings.Join(removed, ", ")
	}
	return "changed links", strings.Join(append(collect(linkAdds, "+"), collect(linkRemoves, "-")...), ", ")
}

// metadataChanged phrases schedule and deadline changes, and names the keys
// of any other metadata diff.
func metadataChanged(p payloadMap) (verb, object string) {
	diff, _ := p["diff"].(map[string]any)
	if changes, ok := slotDiff(diff); ok {
		return notificationSentence(changes)
	}
	if len(diff) == 1 {
		for key, change := range diff {
			to := ""
			if fields, ok := change.(map[string]any); ok && fields["to"] != nil {
				to = plain(fields["to"])
			}
			switch {
			case key == ScheduledKey && to != "":
				return "scheduled for", oneLine(to)
			case key == ScheduledKey:
				return "cleared schedule", ""
			case key == DeadlineKey && to != "":
				return "set deadline to", oneLine(to)
			case key == DeadlineKey:
				return "cleared deadline", ""
			}
		}
	}
	keys := make([]string, 0, len(diff))
	for key := range diff {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var attached, removed, others []string
	for _, key := range keys {
		if id, ok := strings.CutPrefix(key, attachmentKeyPrefix); ok {
			change, _ := diff[key].(map[string]any)
			if name, set := changedAttachment(id, change); set {
				attached = append(attached, name)
			} else {
				removed = append(removed, name)
			}
			continue
		}
		if word := metadataKeyWords(key); !slices.Contains(others, word) {
			others = append(others, word)
		}
	}
	switch {
	case len(others) == 0 && len(removed) == 0:
		return "attached", strings.Join(attached, ", ")
	case len(others) == 0 && len(attached) == 0:
		return "removed", strings.Join(removed, ", ")
	}
	return "updated", strings.Join(append(append(others, attached...), removed...), ", ")
}

// changedAttachment is the file name in one attachment key's diff and whether the diff set it (false: it removed it). A name that
// cannot be read is "an attachment".
func changedAttachment(id string, change map[string]any) (name string, set bool) {
	set = change["to"] != nil
	value := change["from"]
	if set {
		value = change["to"]
	}
	if ref, ok := attachmentRef(id, value); ok {
		return ref.Name, set
	}
	return "an attachment", set
}

// metadataKeyWords is a metadata key as words: a notification slot as its address, the Work attention keys as "Work attention",
// and any other key with its dots and underscores read as spaces.
func metadataKeyWords(key string) string {
	if address, ok := NotificationAddress(key); ok {
		return address
	}
	if key == WorkAttentionKey || key == WorkAttentionMsgKey {
		return "Work attention"
	}
	words := strings.NewReplacer(".", " ", "_", " ").Replace(key)
	if words == "" {
		return words
	}
	return strings.ToLower(words[:1]) + words[1:]
}

func movedTo(event Event, p payloadMap) string {
	var parts []string
	if event.ProjectName != "" {
		parts = append(parts, "to "+event.ProjectName)
	}
	if id := p.text("to_short_id"); id != "" {
		parts = append(parts, "as "+id)
	}
	return strings.Join(parts, " ")
}
