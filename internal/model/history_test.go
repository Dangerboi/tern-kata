package model_test

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// historyRow is the detail row the model builds for one history event by dana.
func historyRow(t *testing.T, event model.Event) model.DetailRow {
	t.Helper()
	event.UID, event.Actor = "event", "dana"
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	}
	return model.HistoryRowOf(model.Model{}, event)
}

func TestHistoryReadsAsOneSentencePerEvent(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 40))
	for _, tt := range []struct {
		name   string
		event  model.Event
		title  string
		detail string
		tone   model.VerbTone
	}{
		{"created", model.Event{Type: "issue.created", Payload: `{"title":"Fix it","labels":["bug"]}`}, "dana created this Issue", "", ""},
		{"commented", model.Event{Type: "issue.commented", Payload: `{"body":"first line\nsecond line"}`}, "dana commented", "", ""},
		{"comment body not shown", model.Event{Type: "issue.commented", Payload: `{"body":"` + long + `\n\nsecond paragraph"}`}, "dana commented", "", ""},
		{"comment edited", model.Event{Type: "issue.comment_edited", Payload: `{"body":"better"}`}, "dana edited a comment", "", ""},
		{"assigned", model.Event{Type: "issue.assigned", Payload: `{"owner":"alice"}`}, "dana assigned to alice", "", ""},
		{"unassigned", model.Event{Type: "issue.unassigned", Payload: `{"owner":null}`}, "dana unassigned", "", ""},
		{"closed with reason", model.Event{Type: "issue.closed", Payload: `{"reason":"wontfix"}`}, "dana closed as won't fix", "", model.VerbMuted},
		{"closed with message and evidence", model.Event{Type: "issue.closed", Payload: `{"reason":"done","message":"shipped it","evidence":[{"type":"commit","sha":"abc123def4"},{"type":"pr","url":"https://x/y/1"},{"type":"test","command":"go test ./..."}]}`}, "dana closed as done", "shipped it\n\nEvidence: commit abc123d, pull request https://x/y/1, test run go test ./...", ""},
		{"reopened", model.Event{Type: "issue.reopened", Payload: `{"reopened_at":"2026-10-08T18:05:05Z"}`}, "dana reopened", "", ""},
		{"labelled", model.Event{Type: "issue.labeled", Payload: `{"label":"bug"}`}, "dana added label bug", "", ""},
		{"unlabelled", model.Event{Type: "issue.unlabeled", Payload: `{"label":"bug"}`}, "dana removed label bug", "", ""},
		{"priority set", model.Event{Type: "issue.priority_set", Payload: `{"priority":1,"old_priority":2}`}, "dana set priority to P1", "", ""},
		{"priority cleared", model.Event{Type: "issue.priority_cleared", Payload: `{"old_priority":3}`}, "dana cleared priority", "", ""},
		{"title and body edited", model.Event{Type: "issue.updated", Payload: `{"title":"New title","old_title":"Old","body":"x"}`}, "dana edited title and body", "", ""},
		{"body edited", model.Event{Type: "issue.updated", Payload: `{"body":"x"}`}, "dana edited body", "", ""},
		{"owner edited", model.Event{Type: "issue.updated", Payload: `{"owner":"bob","old_owner":"alice"}`}, "dana edited owner", "", ""},
		{"linked", model.Event{Type: "issue.linked", Payload: `{"type":"blocks","from_short_id":"aaaa","to_short_id":"bbbb","incoming":false}`}, "dana linked dependent bbbb", "", ""},
		{"linked incoming", model.Event{Type: "issue.linked", Payload: `{"type":"blocks","from_short_id":"aaaa","to_short_id":"bbbb","incoming":true}`}, "dana linked blocker bbbb", "", ""},
		{"unlinked", model.Event{Type: "issue.unlinked", Payload: `{"type":"related","from_short_id":"aaaa","to_short_id":"bbbb"}`}, "dana unlinked related bbbb", "", ""},
		{"links added", model.Event{Type: "issue.links_changed", Payload: `{"parent_set":"pppp","blocked_by_added":["bbbb","cccc"]}`}, "dana linked parent pppp, blocker bbbb and cccc", "", ""},
		{"one issue in two relations", model.Event{Type: "issue.links_changed", Payload: `{"blocked_by_added":["bbbb"],"related_added":["bbbb"]}`, RelatedIssueUID: new("b"), RelatedIssueShortID: new("bbbb")}, "dana linked bbbb as blocker and related", "", ""},
		{"links removed", model.Event{Type: "issue.links_changed", Payload: `{"related_removed":["rrrr"]}`}, "dana unlinked related rrrr", "", ""},
		{"links changed", model.Event{Type: "issue.links_changed", Payload: `{"blocks_added":["bbbb"],"parent_removed":"pppp"}`}, "dana changed links +dependent bbbb, -parent pppp", "", ""},
		{"attachment added", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"attachment.A":{"from":null,"to":{"name":"notes.txt","media_type":"text/plain","size":9,"docbank_version":"6f1c2e8a-2222-4222-8222-000000000001"}}}}`}, "dana attached notes.txt", "", ""},
		{"attachment removed", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"attachment.A":{"from":{"name":"shot.png","media_type":"image/png","size":9,"docbank_version":"6f1c2e8a-2222-4222-8222-000000000001"},"to":null}}}`}, "dana removed shot.png", "", ""},
		{"attachment unreadable", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"attachment.A":{"from":null,"to":"x"}}}`}, "dana attached an attachment", "", ""},
		{"work attention", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"work.attention":{"from":null,"to":"review"},"work.attention_msg":{"from":null,"to":"look"}}}`}, "dana set Work attention to review", "", ""},
		{"todo status", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"todo.status":{"from":null,"to":"doing"}}}`}, "dana set todo status to doing", "", ""},
		{"value cleared", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"todo.status":{"from":"doing","to":null}}}`}, "dana cleared todo status", "", ""},
		{"needs human", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"work.attention":{"from":null,"to":"needs-human"}}}`}, "dana set Work attention to needs human", "", ""},
		{"scheduled", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"scheduled_on":{"from":null,"to":"2026-12-01"}}}`}, "dana scheduled for 2026-12-01", "", ""},
		{"schedule cleared", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"scheduled_on":{"from":"2026-12-01","to":null}}}`}, "dana cleared schedule", "", ""},
		{"deadline", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"deadline_on":{"from":null,"to":"2026-12-31"}}}`}, "dana set deadline to 2026-12-31", "", ""},
		{"deadline cleared", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"deadline_on":{"from":"2026-12-31","to":null}}}`}, "dana cleared deadline", "", ""},
		{"other metadata", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"zeta":{"from":null,"to":1},"alpha":{"from":null,"to":2}}}`}, "dana updated alpha to 2, zeta to 1", "", ""},
		{"request", slotEvent(map[string]string{"bob": `{"from":"dana","message":"please look\nat this"}`}), "dana asked bob: please look at this", "", ""},
		{"reply notice", slotEvent(map[string]string{"bob": `{"from":"dana","message":"m","kind":"reply","re":"c:abc123"}`}), "dana told bob about a reply", "", ""},
		{"confirm notice", slotEvent(map[string]string{"bob": `{"from":"dana","message":"m","kind":"confirm"}`}), "dana told bob about a confirm", "", ""},
		{"refute notice", slotEvent(map[string]string{"bob": `{"from":"dana","message":"m","kind":"refute"}`}), "dana told bob about a refute", "", ""},
		{"supersede notice", slotEvent(map[string]string{"bob": `{"from":"dana","message":"m","kind":"supersede"}`}), "dana told bob about a supersede", "", ""},
		{"request cleared", slotEvent(map[string]string{"bob": "null"}), "dana cleared bob's request", "", ""},
		{"broadcast to five", slotEvent(map[string]string{"e": same, "a": same, "d": same, "b": same, "c": same}), "dana asked a, b, c +2: all hands", "", ""},
		{"broadcast to three", slotEvent(map[string]string{"c": same, "a": same, "b": same}), "dana asked a, b, c: all hands", "", ""},
		{"teammate address", slotEvent(map[string]string{"alice/rev": `{"from":"dana","message":"m"}`}), "dana asked alice/rev: m", "", ""},
		{"malformed optional fields dropped", slotEvent(map[string]string{"bob": `{"from":"dana","message":"m","kind":5,"re":[],"broadcast":"x","teammate":"bad handle"}`}), "dana asked bob: m", "", ""},
		{"undecodable key", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"notify.!!":{"from":null,"to":{"from":"dana","message":"m"}}}}`}, "dana updated notify !!", "", ""},
		{"slot with unreadable value", slotEvent(map[string]string{"bob": `{"from":"dana","message":" "}`}), "dana updated bob", "", ""},
		{"mixed diff", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"zeta":{"from":null,"to":1},"` + model.NotificationKey("bob") + `":{"from":null,"to":{"from":"dana","message":"m"}}}}`}, "dana updated bob, zeta to 1", "", ""},
		{"scheduled date reached", model.Event{Type: "issue.scheduled_reached", Payload: `{"field":"scheduled_on","value":"2026-12-01","timezone":"UTC","instant":"2026-12-01T00:00:00Z"}`}, "dana reached the scheduled date 2026-12-01", "", ""},
		{"moved", model.Event{Type: "issue.moved", ProjectName: "other", Payload: `{"from_short_id":"aaaa","to_short_id":"bbbb","to_project_uid":"p"}`}, "dana moved to other as bbbb", "", ""},
		{"deleted", model.Event{Type: "issue.soft_deleted", Payload: `{}`}, "dana deleted", "", ""},
		{"restored", model.Event{Type: "issue.restored", Payload: `{}`}, "dana restored", "", ""},
		{"unknown type lists sorted pairs", model.Event{Type: "issue.frobbed", Payload: `{"zeta":"last","alpha":["a","b"],"mid":{"k":1},"gone":null,"n":3}`}, "dana frobbed", "", ""},
		{"unknown values kept whole", model.Event{Type: "thing.happened", Payload: `{"text":"` + long + `"}`}, "dana thing happened", "", ""},
		{"long close message with evidence kept whole", model.Event{Type: "issue.closed", Payload: `{"reason":"done","message":"` + long + `","evidence":[{"type":"test","command":"` + long + `"}]}`}, "dana closed as done", long + "\n\nEvidence: test run " + long, ""},
		{"unknown payload that is not JSON", model.Event{Type: "issue.frobbed", Payload: "plain text"}, "dana frobbed", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			row := historyRow(t, tt.event)
			title := sentence(row)
			if title != tt.title || row.History.Tone != tt.tone {
				t.Errorf("title = %q tone = %q, want %q %q", title, row.History.Tone, tt.title, tt.tone)
			}
			if got := closeText(row); got != tt.detail {
				t.Errorf("detail = %q, want %q", got, tt.detail)
			}
			if strings.Contains(title+closeText(row), `{"`) {
				t.Errorf("row shows JSON: %q / %q", title, closeText(row))
			}
		})
	}
}

const same = `{"from":"dana","message":"all hands","broadcast":true}`

// slotEvent is an issue.metadata_updated event that sets (or, for null, clears) the slots of the given addresses.
func slotEvent(slots map[string]string) model.Event {
	var parts []string
	for _, address := range slices.Sorted(maps.Keys(slots)) {
		value := slots[address]
		parts = append(parts, `"`+model.NotificationKey(address)+`":{"from":null,"to":`+value+`}`)
	}
	return model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{` + strings.Join(parts, ",") + `}}`}
}

// sentence is the event as one line, author first, the way a reader takes it in.
func sentence(row model.DetailRow) string {
	return strings.Join(strings.Fields(strings.Join([]string{row.Author, row.History.Verb, row.History.Object, row.Ref, row.History.After}, " ")), " ")
}

func TestHistoryRowsReadOldestFirst(t *testing.T) {
	// The snapshot lists History newest first.
	m := withDetail(model.Model{}, &model.IssueDetailData{History: []model.Event{{UID: "new", Type: "issue.reopened", Payload: "{}"}, {UID: "old", Type: "issue.closed", Payload: "{}"}, {UID: "first", Type: "issue.created", Payload: "{}"}}})
	var got []string
	for i := range m.DetailRowCount(model.TabEvents) {
		row, _ := activityRow(m, model.TabEvents, i)
		got = append(got, row.Key+" "+row.Type)
	}
	if want := []string{"old issue.closed", "new issue.reopened"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestHistoryTimestampsUseTheSnapshotZoneAndAreShort(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("x", 2*3600) // the process zone must be ignored
	t.Cleanup(func() { time.Local = saved })
	for _, tt := range []struct{ zone, want string }{{"", "2026-10-08 18:05"}, {"Asia/Tokyo", "2026-10-09 03:05"}} {
		m := withDetail(inZone(tt.zone), &model.IssueDetailData{History: []model.Event{{UID: "event", Type: "issue.reopened", Payload: "{}", CreatedAt: time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)}}})
		if row, _ := activityRow(m, model.TabEvents, 0); row.Time != tt.want {
			t.Errorf("zone %q: time = %q, want %q", tt.zone, row.Time, tt.want)
		}
	}
}

func TestHistoryEventsKeepTheirIssueReference(t *testing.T) {
	peer := "peer-uid"
	row := historyRow(t, model.Event{Type: "issue.linked", Payload: `{"type":"related","to_short_id":"bbbb"}`, RelatedIssueUID: &peer})
	if row.ReferenceUID != peer {
		t.Fatalf("reference = %q", row.ReferenceUID)
	}
}

// inZone delivers a snapshot in zone, the way the app sets the model zone.
func inZone(zone string) model.Model {
	m := model.WithSnapshotRead(model.Model{})
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest(), Query: m.SnapshotQuery(), LocalDate: "2026-10-08", TimeZone: zone})
	return m
}

func TestAssignmentEventsReadAsKataTuiDoes(t *testing.T) {
	for _, c := range []struct{ typ, payload, want string }{
		{"issue.assignment_expired", `{"previous_owner":"alice","assignment_expires_on":"2026-10-08T10:00:00Z"}`, "assignment for alice expired at 2026-10-08T10:00:00Z"},
		{"issue.assignment_expired", `{"previous_owner":"alice"}`, "assignment for alice expired"},
		{"issue.assignment_expired", `{}`, "assignment expired"},
		{"issue.assignment_renewed", `{"owner":"alice","assignment_expires_on":"2026-10-08T10:00:00Z"}`, "renewed assignment for alice until 2026-10-08T10:00:00Z"},
		{"issue.assignment_renewed", `{"owner":"alice"}`, "renewed assignment for alice"},
		{"issue.assignment_renewed", `{}`, "renewed assignment"},
	} {
		if got := historyRow(t, model.Event{Type: c.typ, Payload: c.payload}).History.Verb; got != c.want {
			t.Errorf("%s %s: %q, want %q", c.typ, c.payload, got, c.want)
		}
	}
}

func TestEventsTabListsOnlyChangesToTheIssue(t *testing.T) {
	for _, tt := range []struct {
		name   string
		event  model.Event
		hidden bool
	}{
		{"created", model.Event{Type: "issue.created", Payload: "{}"}, true},
		{"commented", model.Event{Type: "issue.commented", Payload: "{}"}, true},
		{"comment edited", model.Event{Type: "issue.comment_edited", Payload: "{}"}, true},
		{"notification slot", slotEvent(map[string]string{"bob": `{"from":"dana","message":"m","kind":"reply"}`}), true},
		{"other metadata", model.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"zeta":{"from":null,"to":1}}}`}, false},
		{"closed", model.Event{Type: "issue.closed", Payload: "{}"}, false},
		{"unknown type", model.Event{Type: "issue.frobbed", Payload: "{}"}, false},
	} {
		if got := model.EventHidden(tt.event); got != tt.hidden {
			t.Errorf("%s: EventHidden = %v, want %v", tt.name, got, tt.hidden)
		}
	}
	m := withDetail(model.Model{}, &model.IssueDetailData{History: []model.Event{{UID: "b", Type: "issue.commented", Payload: "{}"}, {UID: "a", Type: "issue.created", Payload: "{}"}}})
	if n := m.DetailRowCount(model.TabEvents); n != 0 {
		t.Errorf("events tab counts %d rows, want 0 for an Issue with only created and commented events", n)
	}
}

func TestAnEventAboutAnotherIssueNamesItFromTheShownIssuesSide(t *testing.T) {
	m := withDetail(model.Model{}, &model.IssueDetailData{Issue: &model.Issue{UID: "parent"}})
	child, parent := "child", "parent"
	childShort := "kb01"
	for _, tt := range []struct {
		name, typ, payload, want string
	}{
		{"parent set", "issue.links_changed", `{"parent_set":"mm2a"}`, "dana linked child kb01"},
		{"blocks added", "issue.links_changed", `{"blocks_added":["mm2a"]}`, "dana linked blocker kb01"},
		{"linked", "issue.linked", `{"type":"parent","to_short_id":"mm2a"}`, "dana linked child kb01"},
	} {
		row := model.HistoryRowOf(m, model.Event{Type: tt.typ, Actor: "dana", Payload: tt.payload, IssueUID: child, IssueShortID: childShort,
			RelatedIssueUID: &parent, RelatedIssueShortID: new("mm2a")})
		if got := sentence(row); got != tt.want || row.ReferenceUID != child {
			t.Errorf("%s: %q opening %q, want %q opening child", tt.name, got, row.ReferenceUID, tt.want)
		}
	}
}

// closeText is a close event's message and evidence line as one text, the way they read under the event.
func closeText(row model.DetailRow) string {
	if row.Evidence == nil {
		return row.Message
	}
	if row.Message == "" {
		return row.EvidenceText()
	}
	return row.Message + "\n\n" + row.EvidenceText()
}

func TestLinkEventsNameIssuesAsThePaneDoes(t *testing.T) {
	m := withDetail(model.Model{}, &model.IssueDetailData{
		Issue: &model.Issue{UID: "a", ProjectName: "tern-kata"},
		Links: []model.Link{
			{ID: 1, Type: "related", FromIssueUID: "a", FromQualifiedID: "tern-kata#aaaa", ToIssueUID: "far", ToQualifiedID: "agent-skills#cpab"},
			{ID: 2, Type: "related", FromIssueUID: "a", FromQualifiedID: "tern-kata#aaaa", ToIssueUID: "near", ToQualifiedID: "tern-kata#1vza"},
		},
	})
	row := model.HistoryRowOf(m, model.Event{Type: "issue.links_changed", Actor: "dana", Payload: `{"related_added":["cpab","1vza"]}`})
	if got, want := sentence(row), "dana linked related agent-skills#cpab and #1vza"; got != want {
		t.Errorf("event reads %q, want %q", got, want)
	}
}

func TestAReplyStatesItsRelationOnceWithItsTarget(t *testing.T) {
	m := withDetail(model.Model{}, &model.IssueDetailData{
		Issue: &model.Issue{UID: "a"},
		Comments: []model.Comment{
			{UID: "c1", Author: "dangerboi", Backlinks: []model.CommentLink{{UID: new("c2"), Kind: "reply", Author: new("dana")}}},
			{UID: "c2", Author: "dana", ReplyKind: new("reply"), Reply: &model.CommentLink{UID: new("c1"), Kind: "reply", Author: new("dangerboi")}},
		},
	})
	for uid, want := range map[string]string{"c1": "dana's reply", "c2": "dangerboi's comment"} {
		if got := m.CommentLinkLabels(uid); len(got) != 1 || got[0] != want {
			t.Errorf("%s links read %q, want one %q", uid, got, want)
		}
	}
}
