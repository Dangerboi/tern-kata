package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

const thisHostID = "a0a0a0a0"

func remoteRecord(host, hostID, block, address string, at time.Time) model.AgentRecord {
	return model.AgentRecord{Address: address, Host: host, HostID: hostID, Block: block, Name: "omp", Actor: "dana", StartedAt: at, MessageFrom: []string{}}
}

// withRemoteRecords is s with Agent records in the metadata of Issue uid, a Pane whose own Host id is thisHostID.
func withRemoteRecords(s driver.State, uid string, records ...model.AgentRecord) driver.State {
	s.Model.HostID = thisHostID
	s.Model.Actor = "dana"
	issues := append([]model.Issue(nil), s.Model.Issues...)
	for i := range issues {
		if issues[i].UID != uid {
			continue
		}
		metadata := map[string]any{}
		for _, r := range records {
			metadata[model.AgentRecordKey(r.HostID, r.Block)] = r
		}
		issues[i].Metadata = metadata
	}
	s.Model.Issues = issues
	return s
}

func remotePane(t *testing.T) driver.State {
	t.Helper()
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	s := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
	return withRemoteRecords(s, "t1",
		remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", at),
		remoteRecord("mac", "c2c2c2c2", "4", "dana/omp-4", at.Add(time.Hour)),
		remoteRecord("laptop", thisHostID, "31", "dana/omp-31", at.Add(-time.Hour)), // this host's own record is no remote agent
	)
}

func TestTheAgentCardListsRemoteAgentsAfterTheLocalOnesWithTheirHost(t *testing.T) {
	s := step(t, remotePane(t), key("w")).State
	for key, want := range map[string]string{
		model.AgentRecordKey("b1b1b1b1", "8"): "on desk · started 2026-11-01 09:00",
		model.AgentRecordKey("c2c2c2c2", "4"): "on mac · started 2026-11-01 10:00",
	} {
		if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID(key) {
			t.Errorf("rows reading %q: %v, want only %s", want, rows, view.AgentItemID(key))
		}
	}
	if got := len(s.Model.AgentCardRows()) + len(s.Model.AgentCardRemoteRows()); got != 5 {
		t.Errorf("the card lists %d rows, want 3 local and 2 remote", got)
	}
	s = feed(t, s, key("end")).State
	if r, ok := s.Model.AgentCardSelectedRemote(); !ok || r.Host != "mac" {
		t.Errorf("end selects %+v, %v; want the agent on mac", r, ok)
	}
}

func TestARemoteRowOffersOnlyForgetAmongTheAgentKeys(t *testing.T) {
	s := step(t, remotePane(t), key("w")).State
	s = feed(t, s, key("down"), key("down"), key("down")).State // the first remote row
	want := model.RemoteAgentRefusal("desk")
	for _, action := range []model.Action{model.AgentCardGo, model.AgentCardBackground, model.AgentCardInterrupt, model.AgentCardStop} {
		if got := s.Model.AgentCardUnavailable(action); got != want {
			t.Errorf("%s is off with %q, want %q", action, got, want)
		}
	}
	if got := s.Model.AgentCardUnavailable(model.AgentCardForget); got != "" {
		t.Errorf("Forget is off with %q on a remote row", got)
	}
	for _, chord := range []string{"enter", "b", "i", "x"} {
		got := step(t, s, key(chord))
		if m := got.State.Model; len(got.Cmds) != 0 || m.TopCard() != model.CardAgent || m.CurrentMode() != model.ModeAgentCard || m.Notice != want {
			t.Errorf("%s: cmds %#v, card %v, mode %s, notice %q; want the card open with %q and nothing sent", chord, got.Cmds, m.TopCard(), m.CurrentMode(), m.Notice, want)
		}
	}
	local := step(t, remotePane(t), key("w")).State
	if got := step(t, local, key("f")); got.State.Model.Notice != model.AgentForgetLocalReason || got.State.Model.AgentCard.Forgetting != "" || len(got.Cmds) != 0 {
		t.Errorf("f on a local row: notice %q, cmds %#v", got.State.Model.Notice, got.Cmds)
	}
}

func TestForgetAsksNamingHostAndStartThenWritesTheRecordOffAndTheSlotAway(t *testing.T) {
	open := feed(t, step(t, remotePane(t), key("w")).State, key("down"), key("down"), key("down")).State
	asked := step(t, open, key("f"))
	if m := asked.State.Model; len(asked.Cmds) != 0 || m.CurrentMode() != model.ModeAgentCardForget {
		t.Fatalf("f: mode %s, cmds %#v; want the question and no write", m.CurrentMode(), asked.Cmds)
	}
	text := wireTextOf(t, asked.State)
	for _, part := range []string{"Forget the agent on desk", "2026-11-01 09:00", "work#t1"} {
		if !strings.Contains(text, part) {
			t.Errorf("the question lacks %q: %s", part, text)
		}
	}
	for _, chord := range []string{"n", "escape"} {
		kept := step(t, asked.State, key(chord))
		if m := kept.State.Model; len(kept.Cmds) != 0 || m.CurrentMode() != model.ModeAgentCard || m.AgentCard.Forgetting != "" {
			t.Errorf("%s: mode %s, cmds %#v; want the list back and no write", chord, m.CurrentMode(), kept.Cmds)
		}
	}
	yes := step(t, asked.State, key("y"))
	want := model.ForgetAgentRecord{Workspace: open.Model.Workspace, ProjectID: 7, UID: "t1", Actor: "dana", HostID: "b1b1b1b1", Block: "8", Address: "dana/omp-8", Host: "desk"}
	if len(yes.Cmds) != 1 || yes.Cmds[0] != want || yes.State.Model.TopCard() == model.CardAgent {
		t.Errorf("y: cmds %#v, card %v; want %#v and the card closed", yes.Cmds, yes.State.Model.TopCard(), want)
	}
	done := step(t, yes.State, model.AgentRecordForgotten{Host: "desk"})
	if done.State.Model.Notice != "Forgot the agent on desk." {
		t.Errorf("notice %q", done.State.Model.Notice)
	}
	failed := step(t, yes.State, model.AgentRecordForgotten{Host: "desk", Err: errTest})
	if !strings.HasPrefix(failed.State.Model.Notice, "Cannot forget the agent on desk: ") {
		t.Errorf("notice %q", failed.State.Model.Notice)
	}
}

func TestALocalRowSaysAnotherHostUsesItsAddress(t *testing.T) {
	s := remotePane(t)
	s.Model.AgentMapping[0].AddressUsedBy = "desk"
	s = step(t, s, key("w")).State
	want := "agent working · started 2026-10-30 09:05 · split · " + model.AddressUsedWords("desk")
	if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID("31") {
		t.Errorf("rows reading %q: %v", want, rows)
	}
	if !strings.Contains(model.AddressUsedWords("desk"), "not published") {
		t.Errorf("words %q", model.AddressUsedWords("desk"))
	}
}

func TestTheBadgeCountsRemoteAgentsAndNamesTheirHostWhenNoLocalAgentRuns(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for name, c := range map[string]struct {
		local       bool
		records     []model.AgentRecord
		text, style string
	}{
		"only one remote agent":          {false, []model.AgentRecord{remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", at)}, "agent on desk", "muted"},
		"two agents on one host":         {false, []model.AgentRecord{remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", at), remoteRecord("desk", "b1b1b1b1", "9", "dana/omp-9", at)}, "agent on desk · 2", "muted"},
		"agents on two hosts":            {false, []model.AgentRecord{remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", at), remoteRecord("mac", "c2c2c2c2", "4", "dana/omp-4", at)}, "agents on 2 hosts", "muted"},
		"a local agent and a remote one": {true, []model.AgentRecord{remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", at)}, "agent working · 2", "accent"},
		"this host's own record":         {true, []model.AgentRecord{remoteRecord("here", thisHostID, "5", "dana/omp-5", at)}, "agent working", "accent"},
	} {
		panes := agentPanes(t, agentIssue("t1"))
		for _, pane := range []string{"list", "lane"} {
			s := panes[pane]
			if c.local {
				s = withAgents(t, s, []model.AgentEntry{badgeAgent("5", 1)}, model.AgentState{Block: "5", State: model.AgentWorking})
			}
			s = withRemoteRecords(s, "t1", c.records...)
			if text, style := badgeSpan(t, s, "t1"); text != c.text || style != c.style {
				t.Errorf("%s, %s: badge %q in %q, want %q in %q", name, pane, text, style, c.text, c.style)
			}
		}
	}
}

func TestTheBadgeSaysMessageUnreadForAMessageAtARemoteAddress(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	s := withRemoteRecords(agentPanes(t, agentIssue("t1"))["list"], "t1", remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", at))
	s.Model.Issues[0].Metadata[model.NotificationKey("dana/omp-8")] = map[string]any{"from": "dana", "message": "rebase first\nthen test"}
	if text, style := badgeSpan(t, s, "t1"); text != "agent on desk · message unread" || style != "warning" {
		t.Errorf("badge %q in %q", text, style)
	}
	s = step(t, s, key("w")).State
	want := "on desk · started 2026-11-01 09:00 · message unread: rebase first"
	if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 {
		t.Errorf("rows reading %q: %v", want, rows)
	}
}
