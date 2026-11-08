package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func agentCardEntries(uid string) []model.AgentEntry {
	at := time.Date(2026, 10, 30, 9, 5, 0, 0, time.UTC)
	return []model.AgentEntry{
		{IssueUID: uid, IssueRef: "work#" + uid, Block: "31", StartedAt: at, Placement: model.AgentSplit},
		{IssueUID: uid, IssueRef: "work#" + uid, Block: "32", StartedAt: at.Add(time.Hour), Placement: model.AgentTab},
		{IssueUID: uid, IssueRef: "work#" + uid, Block: "33", StartedAt: at.Add(2 * time.Hour)},
	}
}

func withAgentRows(s driver.State) driver.State {
	s.Model.AgentMapping = agentCardEntries("t1")
	s.Model.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "31", State: "working"}, {Block: "32", State: "waiting_input"}, {Block: "33", State: "idle"}}}
	return s
}

func TestWOpensTheAgentCardInEveryIssueModeWithOrWithoutAgents(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		for with, state := range map[string]driver.State{"no agents": s, "agents": withAgentRows(s)} {
			got := step(t, state, key("w"))
			// Opening re-reads agents.json and, when the card lists agents, reads their sessions; it starts no agent.
			var seq int64
			var reads []model.Cmd
			for _, c := range got.Cmds {
				switch c := c.(type) {
				case model.RefreshAgents:
					seq = c.Seq
				case model.ReadAgentSessions:
					reads = append(reads, c)
				}
			}
			if seq == 0 || got.State.Model.TopCard() != model.CardAgent || len(got.Cmds) != 1+len(reads) || (with == "agents") != (len(reads) == 1) {
				t.Errorf("%s, %s: card %v, cmds %#v", name, with, got.State.Model.TopCard(), got.Cmds)
			}
			// Shift+w and shift+s start nothing on the Issue.
			for _, chord := range []string{"shift+w", "shift+s"} {
				if off := step(t, state, key(chord)); off.State.Model.TopCard() == model.CardAgent || len(off.Cmds) != 0 {
					t.Errorf("%s, %s, %s: card %v, cmds %#v", name, with, chord, off.State.Model.TopCard(), off.Cmds)
				}
			}
		}
	}
}

// Interrupt is the step before a correction or a Stop, so the card stays open for the next key: x after i asks to stop the
// agent and does not reach the Issue (where x closes it).
func TestInterruptKeepsTheAgentCardOpenSoXThenStopsTheAgent(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		open := step(t, withAgentRows(s), key("w")).State
		interrupted := step(t, open, key("i"))
		if m := interrupted.State.Model; m.TopCard() != model.CardAgent || !strings.Contains(wireTextOf(t, interrupted.State), "Interrupt sent.") {
			t.Errorf("%s: after i, card %v, notice %q; want the Agent card still open and the Notice on screen", name, m.TopCard(), m.Notice)
		}
		asked := step(t, interrupted.State, key("x")).State.Model
		if asked.CurrentMode() != model.ModeAgentCardStop || asked.AgentCard.Stopping != "31" {
			t.Errorf("%s: after i then x, mode %s, stopping %q; want the Stop question for block 31", name, asked.CurrentMode(), asked.AgentCard.Stopping)
		}
	}
}

func TestTheAgentCardWithNoAgentsStillStartsOneAndIgnoresEnterAndB(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		open := step(t, s, key("w")).State
		for _, chord := range []string{"enter", "b"} {
			if got := step(t, open, key(chord)); got.State.Model.TopCard() != model.CardAgent || len(got.Cmds) != 0 {
				t.Errorf("%s, %s: card %v, cmds %#v", name, chord, got.State.Model.TopCard(), got.Cmds)
			}
		}
		if start, ok := startedAgent(step(t, open, key("g")).Cmds); !ok || start.Placement != model.AgentBackground {
			t.Errorf("%s, g: %#v", name, start)
		}
	}
}

func TestTheAgentCardRowsShowStateStartAndWhere(t *testing.T) {
	s := step(t, withAgentRows(agentPanes(t, agentIssue("t1"))["list"]), key("w")).State
	for block, want := range map[string]string{
		"31": "agent working · started 2026-10-30 09:05 · split",
		"32": "agent waiting for you · started 2026-10-30 10:05 · tab",
		"33": "agent idle · started 2026-10-30 11:05 · split",
	} {
		if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID(block) {
			t.Errorf("rows reading %q: %v, want only %s", want, rows, view.AgentItemID(block))
		}
	}
}

func TestTheAgentCardKeysReachTheGateway(t *testing.T) {
	open := func() driver.State {
		return step(t, withAgentRows(agentPanes(t, agentIssue("t1"))["list"]), key("w")).State
	}
	for chord, placement := range map[string]model.AgentPlacement{"s": model.AgentSplit, "t": model.AgentTab, "g": model.AgentBackground} {
		got := step(t, open(), key(chord))
		if start, ok := startedAgent(got.Cmds); !ok || start.Placement != placement || got.State.Model.TopCard() == model.CardAgent {
			t.Errorf("w then %s: cmds %#v", chord, got.Cmds)
		}
	}
	got := feed(t, open(), key("down"), key("enter"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.FocusAgent{Block: "32"}) {
		t.Errorf("down, enter: cmds %#v, want focus of 32", got.Cmds)
	}
	got = feed(t, open(), key("down"), key("b"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.SendAgentBack{Block: "32", IssueRef: "work#t1"}) || got.State.Model.TopCard() == model.CardAgent {
		t.Errorf("down, b: cmds %#v", got.Cmds)
	}
	if esc := step(t, open(), key("escape")); esc.State.Model.TopCard() == model.CardAgent || len(esc.Cmds) != 0 {
		t.Errorf("esc: card %v, cmds %#v", esc.State.Model.TopCard(), esc.Cmds)
	}
}

func TestTheAgentCardWhereColumnNamesBackgroundAndOpenAgents(t *testing.T) {
	s := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
	s.Model.AgentMapping[0].Background = true
	s.Model.AgentMapping[1].Background, s.Model.AgentMapping[1].Open = true, true
	s = step(t, s, key("w")).State
	for block, want := range map[string]string{
		"31": "agent working · started 2026-10-30 09:05 · background",
		"32": "agent waiting for you · started 2026-10-30 10:05 · open",
	} {
		if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID(block) {
			t.Errorf("rows reading %q: %v", want, rows)
		}
	}
}

func TestTheAgentRowShowsTheFirstLineOfAnUnclearedMessageAndLosesItWhenTheSlotClears(t *testing.T) {
	entries := agentCardEntries("t1")
	entries[0].Address = "me/omp-31"
	m := model.Model{AgentMapping: entries, Issues: []model.Issue{{UID: "t1", Metadata: map[string]any{
		model.NotificationKey("me/omp-31"): map[string]any{"from": "me", "message": "rebase first\nthen test"},
	}}}}
	m.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "31", State: "working"}}}
	row := view.AgentRowText(m, m.IssueAgents("t1")[0])
	if !strings.HasSuffix(row, " · message unread: rebase first") || !strings.HasPrefix(row, "agent working · started ") {
		t.Errorf("row %q", row)
	}
	m.Issues[0].Metadata = map[string]any{}
	if row := view.AgentRowText(m, m.IssueAgents("t1")[0]); strings.Contains(row, "unread") {
		t.Errorf("cleared row %q", row)
	}
}

func TestMOnTheAgentCardMessagesTheSelectedRunningAgentWhereverItIs(t *testing.T) {
	at := time.Date(2026, 10, 30, 9, 5, 0, 0, time.UTC)
	entries := []model.AgentEntry{
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "31", Teammate: "omp", Address: "dana/omp-31", StartedAt: at, Placement: model.AgentSplit},
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "32", Teammate: "omp", Address: "dana/omp-32", StartedAt: at.Add(time.Hour), Placement: model.AgentTab},
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "33", Teammate: "omp", Address: "dana/omp-33", StartedAt: at.Add(2 * time.Hour), Placement: model.AgentBackground, Background: true},
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "34", Teammate: "omp", Address: "dana/omp-34", StartedAt: at.Add(3 * time.Hour)},
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "35", Teammate: "claude", NoAddress: model.AgentNotOmp, StartedAt: at.Add(4 * time.Hour)},
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "36", Teammate: "omp", Address: "sam/omp-36", StartedAt: at.Add(5 * time.Hour)},
	}
	pane := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
	pane.Model.AgentMapping, pane.Model.Actor = entries, "dana"
	pane.Model.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{
		{Block: "31", State: model.AgentWorking}, {Block: "32", State: model.AgentIdle}, {Block: "33", State: model.AgentWaitingInput},
		{Block: "34", State: model.AgentExited}, {Block: "35", State: model.AgentWorking}, {Block: "36", State: model.AgentWorking}}}
	open := step(t, pane, key("w")).State

	for i, address := range []string{"dana/omp-31", "dana/omp-32", "dana/omp-33"} {
		s := open
		for range i {
			s = step(t, s, key("down")).State
		}
		asked := step(t, s, key("m"))
		if m := asked.State.Model; m.TopCard() != model.CardAgentMessage || m.AgentMessage.Address != address || m.AgentMessage.IssueUID != "t1" ||
			m.AgentMessage.ProjectID != 7 || len(asked.Cmds) != 0 {
			t.Fatalf("row %d: card %v message %+v cmds %v, want the Message card for %s", i, m.TopCard(), m.AgentMessage, asked.Cmds, address)
		}
		typed := feed(t, asked.State, key("h"), key("i"))
		sent := sentMessage(t, step(t, typed.State, key("enter")).Cmds)
		if sent.Address != address || sent.UID != "t1" || sent.ProjectID != 7 || sent.Actor != "dana" || sent.Message != "hi" || sent.Replacing != "" {
			t.Errorf("row %d: sent %+v, want a message to %s on t1", i, sent, address)
		}
	}

	for row, reason := range map[int]string{3: model.AgentExitedRefusal, 4: model.AgentNotOmpRefusal, 5: model.AgentOtherActorRefusal} {
		s := open
		for range row {
			s = step(t, s, key("down")).State
		}
		got := step(t, s, key("m"))
		if m := got.State.Model; m.TopCard() != model.CardAgent || m.Notice != reason || len(got.Cmds) != 0 {
			t.Errorf("row %d: card %v notice %q cmds %v, want a Notice %q", row, m.TopCard(), m.Notice, got.Cmds, reason)
		}
		if why := got.State.Model.AgentCardUnavailable(model.AgentCardMessage); why != reason {
			t.Errorf("row %d: the button is off with %q, want %q", row, why, reason)
		}
	}
}

func TestMOnTheAgentCardFollowsTheSenderRuleForAListedActor(t *testing.T) {
	at := time.Date(2026, 10, 30, 9, 5, 0, 0, time.UTC)
	entries := []model.AgentEntry{
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "31", Teammate: "omp", Address: "dana/omp-31", MessageFrom: "dana-laptop", StartedAt: at},
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "32", Teammate: "omp", Address: "dana/omp-32", StartedAt: at.Add(time.Hour)},
	}
	states := model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "31", State: model.AgentWorking}, {Block: "32", State: model.AgentWorking}}}
	for name, tc := range map[string]struct {
		verified bool
		row      int
		wantCard model.Card
		notice   string
	}{
		"listed, daemon not verified":     {false, 0, model.CardAgent, model.AgentUnverifiedRefusal},
		"listed, daemon verified":         {true, 0, model.CardAgentMessage, ""},
		"not listed, daemon verified":     {true, 1, model.CardAgent, model.AgentOtherActorRefusal},
		"not listed, daemon not verified": {false, 1, model.CardAgent, model.AgentOtherActorRefusal},
	} {
		pane := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
		pane.Model.AgentMapping, pane.Model.Actor, pane.Model.Agents, pane.Model.VerifiedSenders = entries, "dana-laptop", states, tc.verified
		s := step(t, pane, key("w")).State
		for range tc.row {
			s = step(t, s, key("down")).State
		}
		got := step(t, s, key("m"))
		if m := got.State.Model; m.TopCard() != tc.wantCard || m.Notice != tc.notice || len(got.Cmds) != 0 {
			t.Errorf("%s: card %v notice %q cmds %v, want card %v notice %q", name, m.TopCard(), m.Notice, got.Cmds, tc.wantCard, tc.notice)
		}
	}
}

func teamCardPane(t *testing.T, state model.TeamState) driver.State {
	t.Helper()
	s := agentPanes(t, agentIssue("t1"))["list"]
	s.Model.Snapshot.Links = []model.Link{
		{ID: 1, Type: "parent", FromIssueUID: "c1", FromQualifiedID: "work#c1", FromStatus: "open", ToIssueUID: "t1"},
		{ID: 2, Type: "parent", FromIssueUID: "c2", FromQualifiedID: "work#c2", FromStatus: "closed", ToIssueUID: "t1"},
		{ID: 3, Type: "parent", FromIssueUID: "c3", FromQualifiedID: "work#c3", FromStatus: "open", ToIssueUID: "t1"},
		{ID: 4, Type: "parent", FromIssueUID: "c4", FromQualifiedID: "other#c4", FromStatus: "open", ToIssueUID: "t1"},
		{ID: 5, Type: "parent", FromIssueUID: "c5", FromQualifiedID: "work#c5", FromStatus: "open", ToIssueUID: "t1"},
		{ID: 6, Type: "blocks", FromIssueUID: "c3", FromQualifiedID: "work#c3", FromStatus: "open", ToIssueUID: "c5", ToQualifiedID: "work#c5", ToStatus: "open"},
	}
	s.Model.AgentMapping = []model.AgentEntry{
		{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: "30", Teammate: "omp", Address: "dana/omp-30", Placement: model.AgentSplit},
		{IssueUID: "c1", IssueRef: "work#c1", ProjectID: 7, Block: "31", Teammate: "omp", Address: "dana/omp-31", Background: true},
	}
	s.Model.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "30", State: model.AgentIdle}, {Block: "31", State: model.AgentWorking}}}
	s.Model.Teams = []model.Team{{ParentUID: "t1", ParentRef: "work#t1", Project: "work", ProjectID: 7, CoordinatorBlock: "30", CoordinatorAddress: "dana/omp-30", Limit: 3, State: state,
		Children: []model.TeamChild{{UID: "c1", Ref: "work#c1", Block: "31"}, {UID: "c2", Ref: "work#c2", Block: "32"}}}}
	return step(t, s, key("w")).State
}

func TestTheParentsCardShowsTheTeamUnderItsAgents(t *testing.T) {
	s := teamCardPane(t, model.TeamRunning)
	text := wireTextOf(t, s)
	wantText(t, "the team section", text, "Team: running · 3 at once")
	for uid, want := range map[string]string{
		"c1": "work#c1 · agent working",
		"c2": "work#c2 · ran: closed",
		"c3": "work#c3 · queued",
		"c4": "other#c4 · skipped: other Project",
		"c5": "work#c5 · not ready",
	} {
		if rows := rowsNamed(t, s.Model, view.TeamListID, want); len(rows) != 1 || rows[0] != view.TeamListID+"."+uid {
			t.Errorf("rows reading %q: %v, want only %s", want, rows, view.TeamListID+"."+uid)
		}
	}
}

func TestTheTeamSectionSaysPausedCoordinatorExitedAndEnding(t *testing.T) {
	for state, want := range map[model.TeamState]string{model.TeamPaused: "Team: paused · 3 at once", model.TeamStopping: "Team: coordinator exited · 3 at once", model.TeamEnding: "Team: ending · 3 at once"} {
		wantText(t, string(state), wireTextOf(t, teamCardPane(t, state)), want)
	}
	if text := wireTextOf(t, step(t, agentPanes(t, agentIssue("t1"))["list"], key("w")).State); strings.Contains(text, "Team:") {
		t.Errorf("an Issue without a team shows a team section: %q", text)
	}
}

func TestAChildsCardNamesItsTeamsParentAndCoordinator(t *testing.T) {
	s := teamCardPane(t, model.TeamRunning)
	s.Model.AgentCard.IssueUID, s.Model.AgentCard.IssueRef = "c1", "work#c1"
	text := wireTextOf(t, s)
	wantText(t, "the child's card", text, "a child of the team on work#t1", "coordinator dana/omp-30")
	if strings.Contains(text, "at once") {
		t.Errorf("a child's card shows the parent's team section: %q", text)
	}
}

func TestPOnTheParentsCardPausesAndResumesAndIsOffWithoutATeam(t *testing.T) {
	got := step(t, teamCardPane(t, model.TeamRunning), key("p"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.SetTeamPaused{ParentUID: "t1", ParentRef: "work#t1", Paused: true}) || got.State.Model.TopCard() != model.CardAgent {
		t.Errorf("p on a running team: cmds %#v", got.Cmds)
	}
	got = step(t, teamCardPane(t, model.TeamPaused), key("p"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.SetTeamPaused{ParentUID: "t1", ParentRef: "work#t1"}) {
		t.Errorf("p on a paused team: cmds %#v", got.Cmds)
	}
	wantText(t, "the paused team's card", wireTextOf(t, teamCardPane(t, model.TeamPaused)), "Resume the team")
	for _, state := range []model.TeamState{model.TeamStopping, model.TeamEnding} {
		if off := step(t, teamCardPane(t, state), key("p")); len(off.Cmds) != 0 || off.State.Model.TopCard() == model.CardAgent || off.State.Model.Notice == "" {
			t.Errorf("%s: cmds %#v, notice %q", state, off.Cmds, off.State.Model.Notice)
		}
	}
	open := step(t, agentPanes(t, agentIssue("t1"))["list"], key("w")).State
	if off := step(t, open, key("p")); len(off.Cmds) != 0 || off.State.Model.Notice != model.TeamNoneReason {
		t.Errorf("without a team: cmds %#v, notice %q", off.Cmds, off.State.Model.Notice)
	}
}
