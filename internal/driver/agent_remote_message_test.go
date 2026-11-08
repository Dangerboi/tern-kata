package driver_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

var remoteAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

// onFirstRemoteRow is the Agent card of remotePane with the cursor on the agent on desk, the first remote row.
func onFirstRemoteRow(t *testing.T, verified bool, edit func(*model.Model)) driver.State {
	t.Helper()
	s := remotePane(t)
	s.Model.VerifiedSenders = verified
	if edit != nil {
		edit(&s.Model)
	}
	return feed(t, step(t, s, key("w")).State, key("down"), key("down"), key("down")).State
}

func TestMOnARemoteRowOpensTheMessageCardForItsAddressOnAVerifiedDaemon(t *testing.T) {
	open := step(t, onFirstRemoteRow(t, true, nil), key("m"))
	if m := open.State.Model; m.TopCard() != model.CardAgentMessage || len(open.Cmds) != 0 || m.Notice != "" {
		t.Fatalf("card %v notice %q cmds %v, want the Message card", m.TopCard(), m.Notice, open.Cmds)
	}
	sent := sentMessage(t, step(t, step(t, open.State, tern.Key{Text: "hi"}).State, key("enter")).Cmds)
	want := model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "t1", Actor: "dana", Address: "dana/omp-8", Message: "hi"}
	if sent != want {
		t.Errorf("sent %+v, want %+v", sent, want)
	}
}

func TestRQuickRepliesAnAgentOnAnotherHost(t *testing.T) {
	pick := step(t, onFirstRemoteRow(t, true, nil), key("r"))
	if m := pick.State.Model; m.CurrentMode() != model.ModeAgentQuickReply || len(pick.Cmds) != 0 {
		t.Fatalf("mode %s cmds %v, want the quick reply picker", m.CurrentMode(), pick.Cmds)
	}
	sent := sentMessage(t, step(t, pick.State, key("2")).Cmds)
	if sent.Address != "dana/omp-8" || sent.UID != "t1" || sent.Message != model.QuickReplyWrapUp {
		t.Errorf("sent %+v, want the wrap-up to dana/omp-8 on t1", sent)
	}
}

func TestMAndRRefuseARemoteRowOnAnUnverifiedDaemonOrForAnActorItDoesNotList(t *testing.T) {
	for name, tc := range map[string]struct {
		verified bool
		edit     func(*model.Model)
		reason   string
	}{
		"unverified daemon": {false, nil, model.AgentUnverifiedRefusal},
		"actor not listed":  {true, func(m *model.Model) { m.Actor = "erin" }, model.AgentOtherActorRefusal},
		"actor listed, unverified daemon": {false, func(m *model.Model) {
			m.Actor = "dana-laptop"
			issue := m.Issues[0]
			issue.Metadata = map[string]any{model.AgentRecordKey("b1b1b1b1", "8"): model.AgentRecord{Address: "dana/omp-8", Host: "desk", HostID: "b1b1b1b1",
				Block: "8", Name: "omp", Actor: "dana", StartedAt: remoteAt, MessageFrom: []string{"dana-laptop"}}}
			m.Issues = []model.Issue{issue}
		}, model.AgentUnverifiedRefusal},
	} {
		s := onFirstRemoteRow(t, tc.verified, tc.edit)
		for _, chord := range []string{"m", "r"} {
			got := step(t, s, key(chord))
			if m := got.State.Model; m.TopCard() != model.CardAgent || m.Notice != tc.reason || len(got.Cmds) != 0 {
				t.Errorf("%s, %s: card %v notice %q cmds %v, want a Notice %q", name, chord, m.TopCard(), m.Notice, got.Cmds, tc.reason)
			}
		}
		for _, action := range []model.Action{model.AgentCardMessage, model.AgentCardQuickReply} {
			if why := s.Model.AgentCardUnavailable(action); why != tc.reason {
				t.Errorf("%s: %s is off with %q, want %q", name, action, why, tc.reason)
			}
		}
	}
}

func TestMOnARemoteRowWithoutAnAddressSaysWhy(t *testing.T) {
	s := onFirstRemoteRow(t, true, func(m *model.Model) {
		issue := m.Issues[0]
		issue.Metadata = map[string]any{model.AgentRecordKey("b1b1b1b1", "8"): model.AgentRecord{Host: "desk", HostID: "b1b1b1b1", Block: "8", Name: "vim",
			Actor: "dana", StartedAt: remoteAt, NoAddress: model.AgentNotOmp, MessageFrom: []string{}}}
		m.Issues = []model.Issue{issue}
	})
	if got := step(t, s, key("m")); got.State.Model.Notice != model.AgentNotOmpRefusal {
		t.Errorf("notice %q, want %q", got.State.Model.Notice, model.AgentNotOmpRefusal)
	}
}

// remoteRequestPane is the Attention Page listing a request omp raised on work#aa11, where only other hosts run agents named omp.
func remoteRequestPane(t *testing.T, verified bool, records ...model.AgentRecord) driver.State {
	t.Helper()
	s := attentionPane(t)
	s.Model.VerifiedSenders = verified
	s = withRemoteRecords(s, "work-aa11", records...)
	return answerAttention(t, step(t, s, key("6")), []model.AttentionRequest{agentRaised(t)}, 0)
}

func TestMOnARequestNoLocalAgentRaisedReachesTheNewestRemoteAgentWithTheRequestsName(t *testing.T) {
	older := remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", remoteAt)
	newer := remoteRecord("mac", "c2c2c2c2", "4", "dana/omp-4", remoteAt.Add(time.Hour))
	other := remoteRecord("mac", "c2c2c2c2", "5", "dana/vim-5", remoteAt.Add(2*time.Hour))
	other.Name = "vim"

	open := step(t, remoteRequestPane(t, true, older, newer, other), key("m"))
	if m := open.State.Model; m.TopCard() != model.CardAgentMessage || m.AgentMessage.Address != "dana/omp-4" {
		t.Fatalf("card %v address %q notice %q, want the Message card for dana/omp-4", m.TopCard(), m.AgentMessage.Address, m.Notice)
	}
	sent := sentMessage(t, step(t, step(t, open.State, tern.Key{Text: "use the second table"}).State, key("enter")).Cmds)
	want := model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "work-aa11", Actor: "dana", Address: "dana/omp-4", Message: "use the second table"}
	if sent != want {
		t.Errorf("sent %+v, want %+v", sent, want)
	}

	pick := step(t, remoteRequestPane(t, true, older, newer), key("r"))
	if m := pick.State.Model; m.CurrentMode() != model.ModeAgentQuickReply || m.AgentMessage.Address != "dana/omp-4" {
		t.Errorf("r: mode %s address %q, want the picker for dana/omp-4", m.CurrentMode(), m.AgentMessage.Address)
	}
}

func TestMOnARemoteAgentsRequestIsRefusedOnAnUnverifiedDaemon(t *testing.T) {
	s := remoteRequestPane(t, false, remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", remoteAt))
	for _, chord := range []string{"m", "r"} {
		got := step(t, s, key(chord))
		if m := got.State.Model; m.TopCard() == model.CardAgentMessage || m.Notice != model.AgentUnverifiedRefusal {
			t.Errorf("%s: card %v notice %q, want the unverified refusal", chord, m.TopCard(), m.Notice)
		}
	}
}

func TestMOnARequestNamingNoAgentOfAnyHostStillSaysNoAgentRaisedIt(t *testing.T) {
	other := remoteRecord("desk", "b1b1b1b1", "8", "dana/vim-8", remoteAt)
	other.Name = "vim"
	got := step(t, remoteRequestPane(t, true, other), key("m"))
	if m := got.State.Model; m.TopCard() == model.CardAgentMessage || m.Notice != model.NoAgentRaisedRefusal {
		t.Errorf("card %v notice %q, want %q", m.TopCard(), m.Notice, model.NoAgentRaisedRefusal)
	}
}

func TestALocalAgentThatRaisedTheRequestWinsOverRemoteAgentsOfTheSameName(t *testing.T) {
	s := attentionPane(t)
	s = withRemoteRecords(s, "work-aa11", remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", remoteAt))
	s.Model.AgentMapping = []model.AgentEntry{ompAgent}
	s = answerAttention(t, step(t, s, key("6")), []model.AttentionRequest{agentRaised(t)}, 0)
	if open := step(t, s, key("m")); open.State.Model.AgentMessage.Address != "dana/omp-31" {
		t.Errorf("address %q, want the local agent's dana/omp-31", open.State.Model.AgentMessage.Address)
	}
}

// broadcastRemotePane is broadcastPane with Agent records on work#p and its open child work#c1 (and one on the closed child).
func broadcastRemotePane(t *testing.T, verified bool, local ...model.AgentEntry) driver.State {
	t.Helper()
	s := broadcastPane(t, nil, local...)
	s.Model.VerifiedSenders = verified
	// boardCard leaves ProjectID 0; a real snapshot's Issues carry their Project, which the remote targets' writes take from them.
	issues := append([]model.Issue(nil), s.Model.Issues...)
	for i := range issues {
		issues[i].ProjectID = 7
	}
	s.Model.Issues = issues
	s = withRemoteRecords(s, "p", remoteRecord("desk", "b1b1b1b1", "8", "dana/omp-8", remoteAt))
	other := remoteRecord("mac", "c2c2c2c2", "6", "erin/omp-6", remoteAt)
	other.Actor = "erin"
	s = withRemoteRecords(s, "c1", remoteRecord("mac", "c2c2c2c2", "4", "dana/omp-4", remoteAt), other)
	return withRemoteRecords(s, "c2", remoteRecord("mac", "c2c2c2c2", "7", "dana/omp-7", remoteAt))
}

func TestABroadcastIncludesAgentsOnOtherHostsOfTheIssueAndItsOpenChildren(t *testing.T) {
	s := broadcastRemotePane(t, true, broadcastAgent("p", "31"))
	sending := step(t, step(t, step(t, s, tern.Key{Name: "s", Shift: true}).State, tern.Key{Text: "schema changed"}).State, key("enter"))
	got := sentMessages(sending.Cmds)
	want := map[string]model.SendAgentMessage{
		"dana/omp-31": {Workspace: "/repo", ProjectID: 7, UID: "p", Actor: "dana", Address: "dana/omp-31", Message: "schema changed"},
		"dana/omp-8":  {Workspace: "/repo", ProjectID: 7, UID: "p", Actor: "dana", Address: "dana/omp-8", Message: "schema changed"},
		"dana/omp-4":  {Workspace: "/repo", ProjectID: 7, UID: "c1", Actor: "dana", Address: "dana/omp-4", Message: "schema changed"},
	}
	if len(sending.Cmds) != len(want) || len(got) != len(want) {
		t.Fatalf("cmds %#v, want one write each to %v", sending.Cmds, want)
	}
	for address, w := range want {
		if got[address] != w {
			t.Errorf("write to %s: %+v, want %+v", address, got[address], w)
		}
	}
	s = step(t, sending.State, model.AgentMessageSent{UID: "p", Address: "dana/omp-31"}).State
	s = step(t, s, model.AgentMessageSent{UID: "p", Address: "dana/omp-8"}).State
	s = step(t, s, model.AgentMessageSent{UID: "c1", Address: "dana/omp-4"}).State
	wantShown(t, "the outcome", s, "3 agents got the message.", "Skipped omp on work#c1: "+model.AgentOtherActorRefusal)
	wantNotShown(t, "the outcome", s, "work#c2")
}

func TestABroadcastOnAnUnverifiedDaemonListsRemoteAgentsSkippedButWritesToLocalOnes(t *testing.T) {
	s := broadcastRemotePane(t, false, broadcastAgent("p", "31"))
	sending := step(t, step(t, step(t, s, tern.Key{Name: "s", Shift: true}).State, tern.Key{Text: "hello"}).State, key("enter"))
	if got := sentMessages(sending.Cmds); len(sending.Cmds) != 1 || got["dana/omp-31"].UID != "p" {
		t.Fatalf("cmds %#v, want one write to the local agent", sending.Cmds)
	}
	s = step(t, sending.State, model.AgentMessageSent{UID: "p", Address: "dana/omp-31"}).State
	wantShown(t, "the outcome", s, "1 agent got the message.", "Skipped omp on work#p: "+model.AgentUnverifiedRefusal, "Skipped omp on work#c1: "+model.AgentUnverifiedRefusal)
}

func TestABroadcastOnAnIssueWithOnlyRemoteAgentsOpensTheCard(t *testing.T) {
	s := broadcastRemotePane(t, true)
	if open := step(t, s, tern.Key{Name: "s", Shift: true}); open.State.Model.TopCard() != model.CardAgentMessage {
		t.Fatalf("card %v notice %q, want the Message card", open.State.Model.TopCard(), open.State.Model.Notice)
	}
}

func TestTheSweepIncludesAgentsOnOtherHostsOnAnyIssueAndFollowsThemToFinished(t *testing.T) {
	s := broadcastRemotePane(t, true, broadcastAgent("p", "31"))
	sending := step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), ""))
	got := sentMessages(sending.Cmds)
	for _, address := range []string{"dana/omp-31", "dana/omp-8", "dana/omp-4", "dana/omp-7"} {
		if got[address].Message != model.QuickReplyWrapUp {
			t.Errorf("no wrap-up written to %s: %#v", address, sending.Cmds)
		}
	}
	if len(got) != 4 || len(sending.Cmds) != 4 {
		t.Fatalf("cmds %#v, want four writes (erin's agent is refused)", sending.Cmds)
	}

	s = sending.State
	var reads []model.ReadSweepComments
	for _, a := range []struct{ uid, address string }{{"p", "dana/omp-31"}, {"p", "dana/omp-8"}, {"c1", "dana/omp-4"}, {"c2", "dana/omp-7"}} {
		out := step(t, s, model.AgentMessageSent{UID: a.uid, Address: a.address})
		s, reads = out.State, append(reads, sweepReads(out.Cmds)...)
	}
	wantRead := func(uid, address, teammate string) model.ReadSweepComments {
		return model.ReadSweepComments{Workspace: "/repo", ProjectID: 7, UID: uid, Address: address, Teammate: teammate, Since: s.Model.AgentMessage.SweepSince}
	}
	if len(reads) != 3 {
		t.Fatalf("reads %+v, want one for each of the three remote agents", reads)
	}
	for _, w := range []model.ReadSweepComments{wantRead("p", "dana/omp-8", "omp-8"), wantRead("c1", "dana/omp-4", "omp-4"), wantRead("c2", "dana/omp-7", "omp-7")} {
		found := false
		for _, r := range reads {
			found = found || r == w
		}
		if !found {
			t.Errorf("no read %+v in %+v", w, reads)
		}
	}
	wantShown(t, "just after sending", s, "0 of 4 agents have finished the wrap-up.", "omp on work#p, on desk: not yet", "omp on work#c1, on mac: not yet",
		"Skipped omp on work#c1: "+model.AgentOtherActorRefusal)

	// Its teammate's comment since the sweep finishes it; no stream event or relay moves a remote agent otherwise.
	s = step(t, s, model.SweepCommentsRead{UID: "p", Address: "dana/omp-8", Commented: true}).State
	wantShown(t, "after desk's agent commented", s, "1 of 4 agents have finished the wrap-up.", "omp on work#p, on desk: finished")

	// The record going away (the agent exited and its host removed it) finishes it too.
	s = withRemoteRecords(s, "c1")
	wantShown(t, "after mac's record went", s, "2 of 4 agents have finished the wrap-up.", "omp on work#c1, on mac: finished")
	wantShown(t, "the record still there", s, "omp on work#c2, on mac: not yet")
}

func TestTheSweepOnAnUnverifiedDaemonListsRemoteAgentsSkipped(t *testing.T) {
	s := broadcastRemotePane(t, false, broadcastAgent("p", "31"))
	sending := step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), ""))
	if got := sentMessages(sending.Cmds); len(got) != 1 || got["dana/omp-31"].UID != "p" {
		t.Fatalf("cmds %#v, want one write to the local agent", sending.Cmds)
	}
	s = step(t, sending.State, model.AgentMessageSent{UID: "p", Address: "dana/omp-31"}).State
	wantShown(t, "the sweep", s, "0 of 1 agents have finished the wrap-up.", "Skipped omp on work#p: "+model.AgentUnverifiedRefusal)
	wantNotShown(t, "the sweep", s, "on desk: not yet")
}

func TestTheSweepWithOnlyRemoteAgentsOpensTheCard(t *testing.T) {
	s := broadcastRemotePane(t, true)
	out := step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), ""))
	if m := out.State.Model; m.TopCard() != model.CardAgentMessage || len(sentMessages(out.Cmds)) != 3 {
		t.Fatalf("card %v notice %q cmds %#v, want the sweep card and three writes", m.TopCard(), m.Notice, out.Cmds)
	}
}
