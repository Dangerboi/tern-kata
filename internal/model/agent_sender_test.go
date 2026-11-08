package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTheSenderRuleCoversEveryCombinationOfSenderHostAndDaemon(t *testing.T) {
	const owner = "dana"
	listed := []string{"dana-laptop"}
	for _, tc := range []struct {
		name     string
		sender   string
		remote   bool
		verified bool
		want     string
	}{
		{"own actor, local, unverified", owner, false, false, ""},
		{"own actor, local, verified", owner, false, true, ""},
		{"own actor, remote, unverified", owner, true, false, model.AgentUnverifiedRefusal},
		{"own actor, remote, verified", owner, true, true, ""},
		{"listed actor, local, unverified", "dana-laptop", false, false, model.AgentUnverifiedRefusal},
		{"listed actor, local, verified", "dana-laptop", false, true, ""},
		{"listed actor, remote, unverified", "dana-laptop", true, false, model.AgentUnverifiedRefusal},
		{"listed actor, remote, verified", "dana-laptop", true, true, ""},
		{"other actor, local, unverified", "mallory", false, false, model.AgentOtherActorRefusal},
		{"other actor, local, verified", "mallory", false, true, model.AgentOtherActorRefusal},
		{"other actor, remote, unverified", "mallory", true, false, model.AgentOtherActorRefusal},
		{"other actor, remote, verified", "mallory", true, true, model.AgentOtherActorRefusal},
	} {
		if got := model.MessageSenderRefusal(tc.sender, owner, listed, tc.remote, tc.verified); got != tc.want {
			t.Errorf("%s: refusal %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTheUnverifiedRefusalNamesTheDaemonSetting(t *testing.T) {
	if got := model.AgentUnverifiedRefusal; !strings.Contains(got, "[auth] require_token_identity") {
		t.Errorf("refusal %q does not name [auth] require_token_identity", got)
	}
}

func TestAnActorNotListedMayNotMessageAndNoListIsOnlyTheOwner(t *testing.T) {
	if !model.MayMessage("dana", "dana", nil) || model.MayMessage("eve", "dana", nil) || model.MayMessage("eve", "dana", []string{}) {
		t.Error("with no message_from only the agent's own actor may message")
	}
	if !model.MayMessage("eve", "dana", []string{"dana-laptop", "eve"}) {
		t.Error("a listed actor may message")
	}
}
