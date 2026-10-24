package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTheJoinCommandIsKatasWithTheSpokesSettingsOnlyWhenItIsRemote(t *testing.T) {
	base := model.FederationJoinCommand{HubURL: "https://hub.example:7474", HubProjectID: 12, ProjectName: "acme", Token: "tok", Actor: "erin", Capabilities: "claim,pull,push"}
	for name, c := range map[string]struct {
		command model.FederationJoinCommand
		want    string
	}{
		"a local spoke and the bare flags": {base,
			"kata federation join --hub-url https://hub.example:7474 --hub-project-id 12 --project acme --token tok --actor erin --capabilities claim,pull,push"},
		"every optional flag": {func() model.FederationJoinCommand {
			c := base
			c.HubProjectUID, c.ReplayHorizonEventID, c.BaselineThroughEventID = "UID", 5, 4
			c.PushEnabled, c.AllowInsecure, c.AdoptExisting = true, true, true
			return c
		}(), "kata federation join --hub-url https://hub.example:7474 --hub-project-id 12 --project acme --token tok --actor erin --capabilities claim,pull,push " +
			"--hub-project-uid UID --replay-horizon 5 --baseline-through 4 --push --allow-insecure --adopt-existing"},
		"a remote spoke with its insecure flag and token": {func() model.FederationJoinCommand {
			c := base
			c.SpokeEndpoint, c.SpokeAllowInsecure, c.SpokeToken = "http://spoke.example:7474", true, "spoke-tok"
			return c
		}(), "KATA_SERVER=http://spoke.example:7474 KATA_ALLOW_INSECURE=1 KATA_AUTH_TOKEN=spoke-tok kata federation join --hub-url https://hub.example:7474 " +
			"--hub-project-id 12 --project acme --token tok --actor erin --capabilities claim,pull,push"},
		"a spoke called local has no server": {func() model.FederationJoinCommand { c := base; c.SpokeEndpoint = "local"; return c }(),
			"kata federation join --hub-url https://hub.example:7474 --hub-project-id 12 --project acme --token tok --actor erin --capabilities claim,pull,push"},
		"a local spoke reached through a named entry": {func() model.FederationJoinCommand { c := base; c.SpokeEndpoint, c.SpokeDaemon = "local", "my laptop"; return c }(),
			"kata --daemon 'my laptop' federation join --hub-url https://hub.example:7474 --hub-project-id 12 --project acme --token tok --actor erin --capabilities claim,pull,push"},
		"words that need quoting": {func() model.FederationJoinCommand {
			c := base
			c.ProjectName, c.Token, c.Actor = "my project", "it's", ""
			return c
		}(), "kata federation join --hub-url https://hub.example:7474 --hub-project-id 12 --project 'my project' --token 'it'\"'\"'s' --actor '' --capabilities claim,pull,push"},
	} {
		if got := c.command.String(); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}
