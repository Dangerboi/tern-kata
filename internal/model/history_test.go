package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// historyRow is the detail row the model builds for one history event by dana.
func historyRow(t *testing.T, event g.Event) model.DetailRow {
	t.Helper()
	event.UID, event.Actor = "event", "dana"
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	}
	m := withDetail(model.Model{}, &g.UISelectedAuthority{History: []g.Event{event}})
	row, ok := m.DetailRow(model.TabEvents, 0)
	if !ok {
		t.Fatal("no history row")
	}
	return row
}

func TestHistoryReadsAsOneSentencePerEvent(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 40))
	for _, tt := range []struct {
		name   string
		event  g.Event
		title  string
		detail string
		tone   model.VerbTone
	}{
		{"created", g.Event{Type: "issue.created", Payload: `{"title":"Fix it","labels":["bug"]}`}, "dana created this Issue", "", model.VerbAccent},
		{"commented", g.Event{Type: "issue.commented", Payload: `{"body":"first line\nsecond line"}`}, "dana commented", "first line\nsecond line", ""},
		{"comment kept whole", g.Event{Type: "issue.commented", Payload: `{"body":"` + long + `\n\nsecond paragraph"}`}, "dana commented", long + "\n\nsecond paragraph", ""},
		{"comment edited", g.Event{Type: "issue.comment_edited", Payload: `{"body":"better"}`}, "dana edited a comment", "better", ""},
		{"assigned", g.Event{Type: "issue.assigned", Payload: `{"owner":"alice"}`}, "dana assigned to alice", "", ""},
		{"unassigned", g.Event{Type: "issue.unassigned", Payload: `{"owner":null}`}, "dana unassigned", "", ""},
		{"closed with reason", g.Event{Type: "issue.closed", Payload: `{"reason":"wontfix"}`}, "dana closed: wontfix", "", model.VerbSuccess},
		{"closed with message and evidence", g.Event{Type: "issue.closed", Payload: `{"reason":"done","message":"shipped it","evidence":[{"type":"commit","sha":"abc123def4"},{"type":"pr","url":"https://x/y/1"},{"type":"test","command":"go test ./..."}]}`}, "dana closed: done", "shipped it (commit abc123d, pr https://x/y/1, test go test ./...)", model.VerbSuccess},
		{"reopened", g.Event{Type: "issue.reopened", Payload: `{"reopened_at":"2026-10-08T18:05:05Z"}`}, "dana reopened", "", model.VerbWarning},
		{"labelled", g.Event{Type: "issue.labeled", Payload: `{"label":"bug"}`}, "dana added label `bug`", "", ""},
		{"unlabelled", g.Event{Type: "issue.unlabeled", Payload: `{"label":"bug"}`}, "dana removed label `bug`", "", ""},
		{"priority set", g.Event{Type: "issue.priority_set", Payload: `{"priority":1,"old_priority":2}`}, "dana set priority to 1", "", ""},
		{"priority cleared", g.Event{Type: "issue.priority_cleared", Payload: `{"old_priority":3}`}, "dana cleared priority", "", ""},
		{"title and body edited", g.Event{Type: "issue.updated", Payload: `{"title":"New title","old_title":"Old","body":"x"}`}, "dana edited title and body", "New title", ""},
		{"body edited", g.Event{Type: "issue.updated", Payload: `{"body":"x"}`}, "dana edited body", "", ""},
		{"owner edited", g.Event{Type: "issue.updated", Payload: `{"owner":"bob","old_owner":"alice"}`}, "dana edited owner", "", ""},
		{"linked", g.Event{Type: "issue.linked", Payload: `{"type":"blocks","from_short_id":"aaaa","to_short_id":"bbbb","incoming":false}`}, "dana linked blocks bbbb", "", ""},
		{"linked incoming", g.Event{Type: "issue.linked", Payload: `{"type":"blocks","from_short_id":"aaaa","to_short_id":"bbbb","incoming":true}`}, "dana linked blocked by bbbb", "", ""},
		{"unlinked", g.Event{Type: "issue.unlinked", Payload: `{"type":"related","from_short_id":"aaaa","to_short_id":"bbbb"}`}, "dana unlinked related bbbb", "", ""},
		{"links added", g.Event{Type: "issue.links_changed", Payload: `{"parent_set":"pppp","blocked_by_added":["bbbb","cccc"]}`}, "dana linked parent pppp, blocked by bbbb, blocked by cccc", "", ""},
		{"links removed", g.Event{Type: "issue.links_changed", Payload: `{"related_removed":["rrrr"]}`}, "dana unlinked related rrrr", "", ""},
		{"links changed", g.Event{Type: "issue.links_changed", Payload: `{"blocks_added":["bbbb"],"parent_removed":"pppp"}`}, "dana changed links: +blocks bbbb, -parent pppp", "", ""},
		{"scheduled", g.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"scheduled_on":{"from":null,"to":"2026-12-01"}}}`}, "dana scheduled for 2026-12-01", "", ""},
		{"schedule cleared", g.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"scheduled_on":{"from":"2026-12-01","to":null}}}`}, "dana cleared schedule", "", ""},
		{"deadline", g.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"deadline_on":{"from":null,"to":"2026-12-31"}}}`}, "dana set deadline to 2026-12-31", "", ""},
		{"deadline cleared", g.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"deadline_on":{"from":"2026-12-31","to":null}}}`}, "dana cleared deadline", "", ""},
		{"other metadata", g.Event{Type: "issue.metadata_updated", Payload: `{"diff":{"zeta":{"from":null,"to":1},"alpha":{"from":null,"to":2}}}`}, "dana updated metadata: alpha, zeta", "", ""},
		{"moved", g.Event{Type: "issue.moved", ProjectName: "other", Payload: `{"from_short_id":"aaaa","to_short_id":"bbbb","to_project_uid":"p"}`}, "dana moved to other as bbbb", "", ""},
		{"deleted", g.Event{Type: "issue.soft_deleted", Payload: `{}`}, "dana deleted", "", ""},
		{"restored", g.Event{Type: "issue.restored", Payload: `{}`}, "dana restored", "", ""},
		{"unknown type lists sorted pairs", g.Event{Type: "issue.frobbed", Payload: `{"zeta":"last","alpha":["a","b"],"mid":{"k":1},"gone":null,"n":3}`}, "dana frobbed", "alpha: a, b, gone: none, mid: k: 1, n: 3, zeta: last", ""},
		{"unknown values kept whole", g.Event{Type: "thing.happened", Payload: `{"text":"` + long + `"}`}, "dana thing happened", "text: " + long, ""},
		{"long close message with evidence kept whole", g.Event{Type: "issue.closed", Payload: `{"reason":"done","message":"` + long + `","evidence":[{"type":"test","command":"` + long + `"}]}`}, "dana closed: done", long + " (test " + long + ")", model.VerbSuccess},
		{"unknown payload that is not JSON", g.Event{Type: "issue.frobbed", Payload: "plain text"}, "dana frobbed", "plain text", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			row := historyRow(t, tt.event)
			title := sentence(row)
			if title != tt.title || row.History.Tone != tt.tone {
				t.Errorf("title = %q tone = %q, want %q %q", title, row.History.Tone, tt.title, tt.tone)
			}
			if row.Body != tt.detail {
				t.Errorf("detail = %q, want %q", row.Body, tt.detail)
			}
			if strings.Contains(title+row.Body, `{"`) {
				t.Errorf("row shows JSON: %q / %q", title, row.Body)
			}
		})
	}
}

// sentence is the event as one line, author first, the way a reader takes it in.
func sentence(row model.DetailRow) string {
	return strings.TrimSpace(strings.Join([]string{row.Author, row.History.Verb, row.History.Object, row.Ref}, " "))
}

func TestHistoryTimestampsUseTheSnapshotZoneAndAreShort(t *testing.T) {
	saved := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = saved })
	for _, tt := range []struct{ zone, want string }{{"", "2026-10-08 18:05"}, {"Asia/Tokyo", "2026-10-09 03:05"}} {
		m := withDetail(model.Model{TimeZone: tt.zone}, &g.UISelectedAuthority{History: []g.Event{{UID: "event", Type: "issue.reopened", Payload: "{}", CreatedAt: time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)}}})
		if row, _ := m.DetailRow(model.TabEvents, 0); row.Time != tt.want {
			t.Errorf("zone %q: time = %q, want %q", tt.zone, row.Time, tt.want)
		}
	}
}

func TestHistoryEventsKeepTheirIssueReference(t *testing.T) {
	peer := "peer-uid"
	row := historyRow(t, g.Event{Type: "issue.linked", Payload: `{"type":"related","to_short_id":"bbbb"}`, RelatedIssueUID: &peer})
	if row.ReferenceUID != peer {
		t.Fatalf("reference = %q", row.ReferenceUID)
	}
}
