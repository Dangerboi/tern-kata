package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestInboxAddressFollowsKatasRecipientRules(t *testing.T) {
	tests := []struct {
		name         string
		actor, inbox string
		want         string
		refused      string
	}{
		{name: "the actor when no inbox user is set", actor: "dana", want: "dana"},
		{name: "the inbox user over the actor", actor: "dana", inbox: "coordinator/pip", want: "coordinator/pip"},
		{name: "a blank inbox user falls back to the actor", actor: "dana", inbox: "   ", want: "dana"},
		{name: "the inbox user is trimmed", actor: "dana", inbox: "  pip  ", want: "pip"},
		{name: "a blank actor is refused", actor: " ", refused: "blank"},
		{name: "128 bytes is allowed", actor: strings.Repeat("a", 128), want: strings.Repeat("a", 128)},
		{name: "129 bytes is refused", actor: strings.Repeat("a", 129), refused: "128 bytes"},
		{name: "invalid UTF-8 is refused", actor: "dana\xff", refused: "UTF-8"},
		{name: "a control character is refused", actor: "da\tna", refused: "control"},
		{name: "a format character is refused", actor: "da\u200bna", refused: "control"},
		{name: "a refused inbox user is not replaced by the actor", actor: "dana", inbox: "p\x00ip", refused: "control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := model.Model{Actor: tt.actor, InboxUser: tt.inbox}.InboxAddress()
			if tt.refused != "" {
				if err == nil || !strings.Contains(err.Error(), tt.refused) {
					t.Fatalf("address %q, error %v; want a refusal naming %q", got, err, tt.refused)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("address %q, error %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestSlotsDecodeThroughTheSharedReaderIntoAttentionRows(t *testing.T) {
	const comment = "01JABCDEFGHJKMNPQRSTVWXYZ0"
	key := model.NotificationKey("dana")
	tests := []struct {
		name    string
		key     string
		value   string
		invalid bool
		sender  string
		tag     string
		handle  string
		first   string
	}{
		{name: "a request", key: key, value: `{"from":"bob","message":"Need a decision\nsecond line"}`, sender: "bob", tag: "request", first: "Need a decision"},
		{name: "a teammate sender", key: key, value: `{"from":"bob","teammate":"pip","message":"m"}`, sender: "bob/pip", tag: "request", first: "m"},
		{name: "a request naming a comment", key: key, value: `{"from":"bob","message":"m","re":"` + comment + `"}`, sender: "bob", tag: "request", handle: "c:vwxyz0", first: "m"},
		{name: "a reply kind", key: key, value: `{"from":"bob","message":"latest: confirm","kind":"confirm","re":"` + comment + `"}`, sender: "bob", tag: "confirm", handle: "c:" + strings.ToLower(comment[len(comment)-6:]), first: "latest: confirm"},
		{name: "a broadcast", key: key, value: `{"from":"bob","message":"m","broadcast":true}`, sender: "bob", tag: "broadcast", first: "m"},
		{name: "an invalid teammate is dropped", key: key, value: `{"from":"bob","teammate":"-bad","message":"m"}`, sender: "bob", tag: "request", first: "m"},
		{name: "malformed optional fields are dropped", key: key, value: `{"from":"bob","message":"m","re":7,"kind":["x"],"broadcast":"yes","teammate":3}`, sender: "bob", tag: "request", first: "m"},
		{name: "a missing sender", key: key, value: `{"message":"m"}`, invalid: true},
		{name: "a blank message", key: key, value: `{"from":"bob","message":"  "}`, invalid: true},
		{name: "not an object", key: key, value: `"text"`, invalid: true},
		{name: "not JSON", key: key, value: `{`, invalid: true},
		{name: "an undecodable key", key: "notify.!!", value: `{"from":"bob","message":"m"}`, invalid: true},
		{name: "a key that is not a slot", key: "scheduled_on", value: `{"from":"bob","message":"m"}`, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot, err := model.ReadNotificationSlot(tt.key, tt.value)
			if tt.invalid {
				if err == nil {
					t.Fatalf("read %+v, want the slot refused", slot)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if slot.Address != "dana" || slot.Raw != tt.value {
				t.Errorf("address %q raw %q", slot.Address, slot.Raw)
			}
			request := model.AttentionRequest{ProjectName: "work", ShortID: "ab12", Slot: slot}
			if request.Sender() != tt.sender || request.Tag() != tt.tag || request.CommentHandle() != tt.handle || request.FirstLine() != tt.first {
				t.Errorf("sender %q tag %q handle %q first %q; want %q %q %q %q",
					request.Sender(), request.Tag(), request.CommentHandle(), request.FirstLine(), tt.sender, tt.tag, tt.handle, tt.first)
			}
			if request.QualifiedID() != "work#ab12" {
				t.Errorf("qualified id %q", request.QualifiedID())
			}
		})
	}
}

func TestAttentionRequestsOrderNewestIssueUpdateFirstThenByQualifiedID(t *testing.T) {
	at := func(minutes int) time.Time { return time.Date(2026, 10, 9, 12, minutes, 0, 0, time.UTC) }
	requests := []model.AttentionRequest{
		{ProjectName: "work", ShortID: "bbbb", Updated: at(1)},
		{ProjectName: "work", ShortID: "aaaa", Updated: at(1)},
		{ProjectName: "alpha", ShortID: "zzzz", Updated: at(1)},
		{ProjectName: "work", ShortID: "cccc", Updated: at(9)},
	}
	model.OrderAttention(requests)
	var got []string
	for _, r := range requests {
		got = append(got, r.QualifiedID())
	}
	if want := "work#cccc alpha#zzzz work#aaaa work#bbbb"; strings.Join(got, " ") != want {
		t.Fatalf("order %v, want %s", got, want)
	}
}
