package model

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// VerbTone marks the history verbs that are worth a colour. The view maps it
// to a Tern tone; the empty value is neutral.
type VerbTone string

const (
	VerbAccent  VerbTone = "accent"
	VerbSuccess VerbTone = "success"
	VerbWarning VerbTone = "warning"
)

// DetailTime is the one time rule of the detail: Events, comments and the Created/Updated properties read
// like 2026-10-09 22:42, in the Pane calendar.
func (m Model) DetailTime(t time.Time) string {
	return m.Calendar.Stamp(t)
}

// historyRow phrases one event as "<actor> <verb> <object>". Only issue.closed carries a detail
// (Body: the close message and evidence); every other event stays one line. Types kata adds later
// read as their type name.
func (m Model) historyRow(event Event) DetailRow {
	payload, _ := decodePayload(event.Payload)
	verb, object, detail, tone := "", "", "", VerbTone("")
	switch event.Type {
	case "issue.created":
		verb, object, tone = "created", "this Issue", VerbAccent
	case "issue.commented":
		verb = "commented"
	case "issue.comment_edited":
		verb, object = "edited", "a comment"
	case "issue.assigned":
		verb, object = "assigned", "to "+payload.text("owner")
	case "issue.unassigned":
		verb = "unassigned"
	case "issue.closed":
		verb, object, detail, tone = "closed:", payload.text("reason"), closeDetail(payload), VerbSuccess
	case "issue.reopened":
		verb, tone = "reopened", VerbWarning
	case "issue.labeled":
		verb, object = "added label", "`"+payload.text("label")+"`"
	case "issue.unlabeled":
		verb, object = "removed label", "`"+payload.text("label")+"`"
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

// payloadMap is a decoded event payload; numbers keep their JSON spelling.
type payloadMap map[string]any

// decodePayload fails for a payload that is not a JSON object, and then returns no fields.
func decodePayload(raw string) (payloadMap, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var payload payloadMap
	err := decoder.Decode(&payload)
	return payload, err
}

// text is the value at key, empty when absent or null.
func (p payloadMap) text(key string) string {
	if p[key] == nil {
		return ""
	}
	return plain(p[key])
}

// pairs is the "key: value" list of an event this build has no phrasing for.
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

// plain is a value as people would write it: no quotes, braces or brackets.
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

// oneLine collapses s to a single line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func closeDetail(p payloadMap) string {
	var evidence []string
	items, _ := p["evidence"].([]any)
	for _, item := range items {
		fields, _ := item.(map[string]any)
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
	switch {
	case message == "":
		return list
	case list == "":
		return message
	}
	return message + " (" + list + ")"
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

func relationName(relation string) string { return strings.ReplaceAll(relation, "_", " ") }

func linkObject(event Event, p payloadMap) string {
	relation, peer := p.text("type"), p.text("to_short_id")
	if event.RelatedIssueShortID != nil {
		peer = *event.RelatedIssueShortID
	}
	if incoming, _ := p["incoming"].(bool); incoming && inverseRelation[relation] != "" {
		relation = inverseRelation[relation]
	}
	return relationName(relation) + " " + peer
}

// linkField is a links_changed payload field and the relation it names.
type linkField struct{ field, relation string }

var (
	linkAdds    = []linkField{{"parent_set", "parent"}, {"blocks_added", "blocks"}, {"blocked_by_added", "blocked by"}, {"related_added", "related"}}
	linkRemoves = []linkField{{"parent_removed", "parent"}, {"blocks_removed", "blocks"}, {"blocked_by_removed", "blocked by"}, {"related_removed", "related"}}
)

// linksChanged phrases the aggregated event an Issue edit records for any mix
// of added and removed links.
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
	return "changed links:", strings.Join(append(collect(linkAdds, "+"), collect(linkRemoves, "-")...), ", ")
}

// metadataChanged phrases schedule and deadline changes, and names the keys
// of any other metadata diff.
func metadataChanged(p payloadMap) (verb, object string) {
	diff, _ := p["diff"].(map[string]any)
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
	return "updated metadata:", strings.Join(keys, ", ")
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
