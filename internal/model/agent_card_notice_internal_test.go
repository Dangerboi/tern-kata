package model

import "testing"

func TestAgentCardNoticeNamesAVerbForAnActionItDoesNotKnow(t *testing.T) {
	got := Model{}.agentCardNotice(Action("not-a-card-action"), "No agent on this Issue yet.")
	if want := "Cannot do that: no agent on this Issue yet."; got != want {
		t.Errorf("agentCardNotice = %q, want %q", got, want)
	}
}
