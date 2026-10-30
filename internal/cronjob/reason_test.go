package cronjob

import (
	"strings"
	"testing"
)

func TestReadReasonReadsTheConventionAndTheWorkersEarlierShape(t *testing.T) {
	for _, c := range []struct{ message, code, detail string }{
		{"reason: agent_quota\nthe provider answered 429", "agent_quota", "the provider answered 429"},
		{"reason: launch_unproven\nfirst\nsecond", "launch_unproven", "first\nsecond"},
		{"reason: herdr.pane-lost_2", "herdr.pane-lost_2", ""},                  // any adapter's code, no detail
		{"reason: agent_login \r\nlog in again", "agent_login", "log in again"}, // trailing space and CRLF on the reason line
		{"reason: " + strings.Repeat("a", 64) + "\nx", strings.Repeat("a", 64), "x"},
		// The worker's shape before the convention, for its own codes only.
		{"interrupted: the worker stopped while the agent ran", "interrupted", "the worker stopped while the agent ran"},
		{"agent_quota: the provider answered 429: limit", "agent_quota", "the provider answered 429: limit"},
		{"create_rejected: kata refused", "create_rejected", "kata refused"},
		// Not a reason: the whole message is detail.
		{"admitted", "", "admitted"},
		{"note: nothing to do", "", "note: nothing to do"},
		{"reason: Agent_Quota\nx", "", "reason: Agent_Quota\nx"},
		{"reason: \nx", "", "reason: \nx"},
		{"reason: two words\nx", "", "reason: two words\nx"},
		{"reason: " + strings.Repeat("a", 65), "", "reason: " + strings.Repeat("a", 65)},
		{"Reason: agent_quota", "", "Reason: agent_quota"},
		{"the run failed\nreason: agent_quota", "", "the run failed\nreason: agent_quota"},
		{"", "", ""},
	} {
		if code, detail := ReadReason(c.message); code != c.code || detail != c.detail {
			t.Errorf("ReadReason(%q) = %q, %q; want %q, %q", c.message, code, detail, c.code, c.detail)
		}
	}
}

func TestReasonMessageReadsBack(t *testing.T) {
	for _, c := range []struct{ code, detail, want string }{
		{"agent_quota", "the provider answered 429", "reason: agent_quota\nthe provider answered 429"},
		{"interrupted", "", "reason: interrupted"},
	} {
		got := ReasonMessage(c.code, c.detail)
		if got != c.want {
			t.Errorf("ReasonMessage(%q, %q) = %q, want %q", c.code, c.detail, got, c.want)
		}
		if code, detail := ReadReason(got); code != c.code || detail != c.detail {
			t.Errorf("ReadReason(%q) = %q, %q", got, code, detail)
		}
	}
}
