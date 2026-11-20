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
	VerbMuted VerbTone = "muted"
)

// DetailTime is the one time rule of the detail: Events, comments and the meta line read like 18:21 today and 2026-10-09 22:42
// on any other day, in the Pane calendar.
func (m Model) DetailTime(t time.Time) string {
	return m.Calendar.Read(t, m.Now)
}

// EventHidden says the Events tab does not list the event: it only lists changes to the Issue, and the facts panel's Created row already
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
	// An event about another Issue (a child linked to the shown one) names that Issue as the peer, read from the shown
	// Issue's side.
	peerUID, peerShort := "", ""
	if event.RelatedIssueUID != nil {
		peerUID = *event.RelatedIssueUID
	}
	if event.RelatedIssueShortID != nil {
		peerShort = *event.RelatedIssueShortID
	}
	foreign := event.IssueUID != "" && event.IssueUID != m.DetailIssueUID() && event.IssueShortID != ""
	if foreign {
		peerUID, peerShort = event.IssueUID, event.IssueShortID
	}
	verb, object, message, tone := "", "", "", VerbTone("")
	var evidence []RowSpan
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
		message, evidence = closeDetail(payload, m.currentMetadata())
	case "issue.reopened":
		verb = "reopened"
	case "issue.labeled":
		verb, object = "added label", payload.text("label")
	case "issue.unlabeled":
		verb, object = "removed label", payload.text("label")
	case "issue.priority_set":
		verb, object = "set priority to", "P"+payload.text("priority")
	case "issue.priority_cleared":
		verb = "cleared priority"
	case "issue.updated":
		verb, object = "edited", editedFields(payload)
	case "issue.linked", "issue.unlinked":
		verb = strings.TrimPrefix(event.Type, "issue.")
		object = linkObject(event, payload, foreign)
	case "issue.links_changed":
		verb, object = linksChanged(payload, foreign, peerShort)
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
		Key: event.UID, Type: event.Type, Author: event.Actor, Time: m.DetailTime(event.CreatedAt), Message: message, Evidence: evidence,
		History: &HistorySentence{Verb: verb, Object: object, Tone: tone},
	}
	if strings.HasPrefix(event.Type, "issue.link") || strings.HasPrefix(event.Type, "issue.unlink") {
		names := m.linkedNames()
		row.History.Object = qualifyShorts(row.History.Object, names)
		object = qualifyShorts(object, names)
		peerShort = qualifyShorts(peerShort, names)
	}
	row.ReferenceUID = peerUID
	if peerShort != "" && peerUID != "" {
		if after, ok := strings.CutPrefix(object, peerShort+" as "); ok {
			row.Ref, row.History.Object, row.History.After = peerShort, "", "as "+after
		} else if head, ok := strings.CutSuffix(object, peerShort); ok {
			row.Ref, row.History.Object = peerShort, strings.TrimRight(head, " ")
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

// closeTone is plain for a close as done (a closed Issue is never success green), muted for a close that is not a completion, and plain for a code kata adds later.
func closeTone(code string) VerbTone {
	switch {
	case code == "done":
		return ""
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

// evidenceKindWords is the words of a kata evidence kind; a kind kata adds later reads as its code.
var evidenceKindWords = map[string]string{"pr": "pull request", "test": "test run", "reviewed-paths": "reviewed", "audit-no-change": "audit"}

// closeDetail is a close's message and its evidence as spans: "Evidence: " muted, then each item as its kind in words and its
// value in mono, so a command reads exactly as typed.
func closeDetail(p payloadMap, current map[string]any) (message string, evidence []RowSpan) {
	items, _ := p["evidence"].([]any)
	for _, item := range items {
		fields, _ := item.(map[string]any)
		kind, value := "", ""
		if plain(fields["type"]) == "attachment" {
			kind, value = attachmentEvidence(fields, current)
		} else {
			kind = plain(fields["type"])
			if words := evidenceKindWords[kind]; words != "" {
				kind = words
			}
			for _, key := range []string{"sha", "url", "command", "paths", "account", "rationale", "issue_ref"} {
				if v, ok := fields[key]; ok {
					if key == "sha" {
						v = truncate(plain(v), 7)
					}
					value = plain(v)
					break
				}
			}
		}
		if len(evidence) == 0 {
			evidence = append(evidence, RowSpan{"Evidence: ", SpanMuted})
		} else {
			evidence = append(evidence, RowSpan{", ", SpanMuted})
		}
		if value == "" {
			evidence = append(evidence, RowSpan{kind, ""})
			continue
		}
		evidence = append(evidence, RowSpan{kind + " ", ""}, RowSpan{value, SpanMono})
	}
	return strings.TrimSpace(p.text("message")), evidence
}

// attachmentEvidence is how an attachment evidence entry reads in a close: kind "attachment" and the name, with (changed) when the
// Issue has another version of the file now and (removed) when it no longer has the Attachment. The name comes from the copy kata
// froze into the evidence, so it survives the Attachment's removal. An entry with no copy reads from the Issue's current Attachment,
// like kata's web UI; one that has neither reads "Attachment removed", and one whose copy is no reference "Invalid attachment"
// (both as a kind with no value).
func attachmentEvidence(fields map[string]any, metadata map[string]any) (kind, value string) {
	id := plain(fields["attachment_id"])
	nowRef := metadata[attachmentKeyPrefix+id]
	now, nowOK := attachmentRef(id, nowRef)
	frozen, frozenOK := attachmentRef(id, fields["attachment"])
	if _, hasCopy := fields["attachment"]; !hasCopy {
		frozen, frozenOK = attachmentRef(id, nowRef)
		if !frozenOK {
			return "Attachment removed", ""
		}
	} else if !frozenOK {
		return "Invalid attachment", ""
	}
	switch {
	case !nowOK:
		return "attachment", frozen.Name + " (removed)"
	case !strings.EqualFold(now.Version, frozen.Version):
		return "attachment", frozen.Name + " (changed)"
	}
	return "attachment", frozen.Name
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
	return Capitalize(RelationWords(relation))
}

func linkObject(event Event, p payloadMap, foreign bool) string {
	relation, peer := p.text("type"), p.text("to_short_id")
	if event.RelatedIssueShortID != nil {
		peer = *event.RelatedIssueShortID
	}
	if foreign {
		peer = event.IssueShortID
	}
	incoming, _ := p["incoming"].(bool)
	if incoming != foreign && inverseRelation[relation] != "" {
		relation = inverseRelation[relation]
	}
	return linkNoun(RelationWords(relation)) + " " + peer
}

type linkField struct{ field, relation string }

var (
	linkAdds    = []linkField{{"parent_set", "parent"}, {"blocks_added", "blocks"}, {"blocked_by_added", "blocked by"}, {"related_added", "related"}}
	linkRemoves = []linkField{{"parent_removed", "parent"}, {"blocks_removed", "blocks"}, {"blocked_by_removed", "blocked by"}, {"related_removed", "related"}}
)

// inverseWords reads a relation from the other end of its link: the shown Issue was linked to by its child.
var inverseWords = map[string]string{"parent": "child", "blocks": "blocked by", "blocked by": "blocks", "related": "related"}

// linkNoun is what the linked Issue is to the shown one, as the Events tab says it: "blocked by" reads "blocker", and "blocks" reads
// "dependent"; the other relations read as their words.
func linkNoun(words string) string {
	switch words {
	case "blocked by":
		return "blocker"
	case "blocks":
		return "dependent"
	}
	return words
}

type linkPart struct{ noun, ref string }

// linkParts are the links the fields name, each as the noun of its relation and the Issue it links to. An event about another
// Issue links that Issue (peer) to the shown one, so its relations read from the shown Issue's side.
func linkParts(p payloadMap, specs []linkField, foreign bool, peer string) (parts []linkPart) {
	for _, spec := range specs {
		value, ok := p[spec.field]
		if !ok {
			continue
		}
		if foreign {
			parts = append(parts, linkPart{linkNoun(inverseWords[spec.relation]), peer})
			continue
		}
		refs, _ := value.([]any)
		if single, isString := value.(string); isString {
			refs = []any{single}
		}
		for _, ref := range refs {
			parts = append(parts, linkPart{linkNoun(spec.relation), plain(ref)})
		}
	}
	return parts
}

// phraseLinks phrases one direction of a link event: relations in order with their Issues ("blocker bbbb, related cccc and dddd"),
// and when every link is to one Issue, that Issue once with the relations after it ("bbbb as blocker and related"); historyRow
// draws the Issue's id where it stands.
func phraseLinks(parts []linkPart, prefix string) string {
	if len(parts) > 1 && prefix == "" && !slices.ContainsFunc(parts, func(l linkPart) bool { return l.ref != parts[0].ref }) {
		nouns := make([]string, len(parts))
		for i, l := range parts {
			nouns[i] = l.noun
		}
		last := len(nouns) - 1
		return parts[0].ref + " as " + strings.Join(nouns[:last], ", ") + " and " + nouns[last]
	}
	var order []string
	refs := map[string][]string{}
	for _, l := range parts {
		if _, seen := refs[l.noun]; !seen {
			order = append(order, l.noun)
		}
		refs[l.noun] = append(refs[l.noun], l.ref)
	}
	words := make([]string, len(order))
	for i, noun := range order {
		list := refs[noun]
		joined := strings.Join(list, ", ")
		if n := len(list); n > 1 && prefix == "" {
			joined = strings.Join(list[:n-1], ", ") + " and " + list[n-1]
		}
		words[i] = prefix + noun + " " + joined
	}
	return strings.Join(words, ", ")
}

func linksChanged(p payloadMap, foreign bool, peer string) (verb, object string) {
	added, removed := linkParts(p, linkAdds, foreign, peer), linkParts(p, linkRemoves, foreign, peer)
	switch {
	case len(removed) == 0:
		return "linked", phraseLinks(added, "")
	case len(added) == 0:
		return "unlinked", phraseLinks(removed, "")
	}
	return "changed links", phraseLinks(added, "+") + ", " + phraseLinks(removed, "-")
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
		if key == WorkAttentionMsgKey && diff[WorkAttentionKey] != nil {
			continue
		}
		if segment := metadataSegment(key, diff[key]); !slices.Contains(others, segment) {
			others = append(others, segment)
		}
	}
	if len(attached) == 0 && len(removed) == 0 && len(others) == 1 {
		// one change reads as a sentence: "set todo status to doing", "cleared Work attention"
		words, value, set := metadataChange(keys, diff)
		switch {
		case set && value != "":
			return "set", words + " to " + value
		case !set && words != "":
			return "cleared", words
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

// metadataChange is the one changed key of a diff as its words and its new value; set is false when the key was cleared. A
// value that is not a plain word or number reads as no value.
func metadataChange(keys []string, diff map[string]any) (words, value string, set bool) {
	for _, key := range keys {
		if key == WorkAttentionMsgKey && diff[WorkAttentionKey] != nil {
			continue
		}
		change, _ := diff[key].(map[string]any)
		return metadataKeyWords(key), metadataValue(key, change["to"]), change["to"] != nil
	}
	return "", "", false
}

// metadataSegment is one changed key as "<words> to <value>", "<words> cleared", or just the words when the value is no plain
// word or number.
func metadataSegment(key string, raw any) string {
	change, _ := raw.(map[string]any)
	words := metadataKeyWords(key)
	switch value := metadataValue(key, change["to"]); {
	case change["to"] == nil && change != nil:
		return words + " cleared"
	case value != "":
		return words + " to " + value
	}
	return words
}

// metadataValue is a plain value of a metadata key as the Events tab shows it: text and numbers, cut to a short line, and the
// Work attention signal in words. Anything else (an object, a list) shows nothing, as it has no short form.
func metadataValue(key string, v any) string {
	var text string
	switch v := v.(type) {
	case string:
		text = oneLine(v)
	case json.Number:
		text = v.String()
	case bool:
		text = fmt.Sprint(v)
	default:
		return ""
	}
	if key == WorkAttentionKey && text == WorkAttentionNeedsHuman {
		text = "needs human"
	}
	if r := []rune(text); len(r) > 40 {
		text = string(r[:39]) + "…"
	}
	return text
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

// linkedNames reads the short id of each Issue the shown Issue is linked to as the Pane names it: "#short" in the shown Issue's
// own Project and "project#short" in another, as the Links tab does.
func (m Model) linkedNames() map[string]string {
	if m.authority == nil || m.authority.Issue == nil {
		return nil
	}
	names := map[string]string{}
	for _, link := range m.authority.Links {
		for _, qualified := range []string{link.FromQualifiedID, link.ToQualifiedID} {
			project, short, ok := strings.Cut(qualified, "#")
			if !ok || short == "" {
				continue
			}
			if project == m.authority.Issue.ProjectName {
				names[short] = "#" + short
			} else {
				names[short] = qualified
			}
		}
	}
	return names
}

// qualifyShorts replaces each short id in text that names a linked Issue by its name; a word that is no linked id stays.
func qualifyShorts(text string, names map[string]string) string {
	if len(names) == 0 {
		return text
	}
	fields := strings.Split(text, " ")
	for i, field := range fields {
		word := strings.TrimRight(field, ",")
		if name, ok := names[word]; ok {
			fields[i] = name + field[len(word):]
		}
	}
	return strings.Join(fields, " ")
}
