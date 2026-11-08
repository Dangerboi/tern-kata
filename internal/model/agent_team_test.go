package model_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// teamModel is cardModel with an open child "c" of the selected Issue.
func teamModel() model.Model {
	m := cardModel()
	m.Actor = "dana"
	m.Snapshot.Links = []model.Link{{ID: 1, Type: "parent", FromIssueUID: "c", FromStatus: "open", ToIssueUID: "a"}}
	return m
}

func TestCStartsTheTeamDialogAndEnterSendsItsChoice(t *testing.T) {
	m, _ := press(teamModel(), "w", "c")
	if m.CurrentMode() != model.ModeAgentCardTeam || m.AgentCard.Team != (model.TeamDialog{Open: true, Where: model.AgentSplit, Limit: model.TeamDefaultLimit}) {
		t.Fatalf("mode %s, dialog %+v", m.CurrentMode(), m.AgentCard.Team)
	}
	m, cmds := press(m, "t", "up", "up", "enter")
	start, ok := agentCmd[model.StartTeam](cmds)
	if !ok || start.Placement != model.AgentTab || start.Limit != 4 || start.IssueUID != "a" || start.IssueRef != "work#a" || start.ProjectID != 7 || start.Actor != "dana" || start.PaneBlock != "9" {
		t.Fatalf("cmds %#v", cmds)
	}
	if m.TopCard() == model.CardAgent || !strings.Contains(m.Notice, "work#a") {
		t.Errorf("card %v, notice %q; want the card closed and the start announced", m.TopCard(), m.Notice)
	}
}

func TestTheTeamDialogKeepsTheLimitBetweenOneAndEightAndEscGoesBackToTheCard(t *testing.T) {
	m, _ := press(teamModel(), "w", "c")
	more, _ := press(m, "up", "up", "up", "up", "up", "up", "up", "up", "up")
	less, _ := press(m, "down", "down", "down")
	if more.AgentCard.Team.Limit != model.TeamMaxLimit || less.AgentCard.Team.Limit != model.TeamMinLimit {
		t.Errorf("limits %d and %d, want %d and %d", more.AgentCard.Team.Limit, less.AgentCard.Team.Limit, model.TeamMaxLimit, model.TeamMinLimit)
	}
	back, cmds := press(m, "g", "escape")
	if back.CurrentMode() != model.ModeAgentCard || back.AgentCard.Team.Open || len(cmds) != 0 || back.TopCard() != model.CardAgent {
		t.Errorf("mode %s, dialog %+v, cmds %#v", back.CurrentMode(), back.AgentCard.Team, cmds)
	}
}

func TestCIsRefusedWithItsReasonAndStartsNothing(t *testing.T) {
	noChild := teamModel()
	noChild.Snapshot.Links = nil
	hasTeam := teamModel()
	hasTeam.Teams = []model.Team{{ParentUID: "a", State: model.TeamStopping}}
	closed := teamModel()
	closed.Issues[0].Status = "closed"
	for name, c := range map[string]struct {
		m      model.Model
		reason string
	}{
		"no open child":  {noChild, model.TeamNoChildReason},
		"a team already": {hasTeam, model.TeamExistsReason},
	} {
		opened, _ := press(c.m, "w")
		if reason := opened.AgentCardUnavailable(model.AgentCardTeam); reason != c.reason {
			t.Errorf("%s: the card's button says %q, want %q", name, reason, c.reason)
		}
		after, cmds := press(opened, "c")
		if _, started := agentCmd[model.StartTeam](cmds); started || after.AgentCard.Team.Open || after.TopCard() == model.CardAgent || after.Notice != c.reason {
			t.Errorf("%s: dialog %v, cmds %#v, notice %q; want the reason and nothing started", name, after.AgentCard.Team.Open, cmds, after.Notice)
		}
	}
	if after, _ := press(closed, "w"); after.TopCard() == model.CardAgent || after.Notice != model.ClosedIssueRefusal {
		t.Errorf("a closed Issue opened the card: %v %q", after.TopCard(), after.Notice)
	}
}

// drivingModel is a Pane that knows its agents: the pruned mapping and a first relay are in.
func drivingModel(t *testing.T) model.Model {
	t.Helper()
	m := teamModel()
	m.Teams = []model.Team{{ParentUID: "elsewhere", State: model.TeamRunning}}
	m, _ = model.Update(m, model.AgentsPruned{})
	m, cmds := model.Update(m, model.AgentStatesMsg{})
	if _, driven := agentCmd[model.DriveTeams](cmds); !driven {
		t.Fatalf("the first relay did not drive the teams: %#v", cmds)
	}
	return m
}

func TestTheQueueIsDrivenOnEachRelayAndEventOneAtATime(t *testing.T) {
	m := drivingModel(t)
	// While one drive is out, relays and events owe one more, not one each.
	m, cmds := model.Update(m, model.AgentStatesMsg{States: []model.AgentState{{Block: "5", State: model.AgentExited}}})
	if _, driven := agentCmd[model.DriveTeams](cmds); driven {
		t.Fatalf("a second drive started while one is out: %#v", cmds)
	}
	m, cmds = model.Update(m, model.TeamsDriven{Teams: []model.Team{{ParentUID: "a", State: model.TeamRunning}}})
	drive, driven := agentCmd[model.DriveTeams](cmds)
	if !driven || !slices.Equal(drive.Exited, []string{"5"}) || drive.Actor != "dana" {
		t.Fatalf("the owed drive: %#v", cmds)
	}
	if _, on := m.TeamOn("a"); !on {
		t.Errorf("the answered teams are not kept: %+v", m.Teams)
	}
	// With nothing owed an answer asks nothing.
	if _, cmds = model.Update(m, model.TeamsDriven{}); len(cmds) != 0 {
		t.Errorf("an answer with nothing owed asked %#v", cmds)
	}
}

func TestAChildTheDriveStartedIsMappedAndAFailureIsTheNotice(t *testing.T) {
	m := drivingModel(t)
	child := model.AgentEntry{IssueUID: "c", IssueRef: "work#c", Block: "31", Background: true}
	m, _ = model.Update(m, model.TeamsDriven{Started: []model.AgentEntry{child}})
	if got := m.IssueAgents("c"); len(got) != 1 || got[0].Block != "31" {
		t.Errorf("agents on the child: %+v", got)
	}
	m, _ = model.Update(m, model.TeamsDriven{Failures: []string{"Cannot start an agent on work#d for the team on work#a: tern failed"}})
	if !strings.Contains(m.Notice, "work#d") {
		t.Errorf("notice %q", m.Notice)
	}
}

func TestATeamThatStartedIsMappedAndDrivenAtOnce(t *testing.T) {
	m := drivingModel(t)
	m, _ = model.Update(m, model.TeamsDriven{})
	m, cmds := model.Update(m, model.TeamStarted{Placement: model.AgentSplit, IssueRef: "work#a", Entry: model.AgentEntry{IssueUID: "a", IssueRef: "work#a", Block: "30", Address: "dana/omp-30"}})
	if _, driven := agentCmd[model.DriveTeams](cmds); !driven || !strings.Contains(m.Notice, "work#a") || len(m.IssueAgents("a")) != 1 {
		t.Errorf("cmds %#v, notice %q, agents %+v", cmds, m.Notice, m.IssueAgents("a"))
	}
	m, _ = model.Update(m, model.TeamStarted{IssueRef: "work#a", Err: errors.New("tern failed")})
	if !strings.Contains(m.Notice, "Cannot start a team on work#a") {
		t.Errorf("notice %q", m.Notice)
	}
}

func pausableModel(state model.TeamState) model.Model {
	m := teamModel()
	m.Actor = "dana"
	m.Teams = []model.Team{{ParentUID: "a", ParentRef: "work#a", State: state}}
	return m
}

func TestPPausesARunningTeamAndResumesAPausedOneAndKeepsTheCardOpen(t *testing.T) {
	for state, paused := range map[model.TeamState]bool{model.TeamRunning: true, model.TeamPaused: false} {
		m, cmds := press(pausableModel(state), "w", "p")
		if len(cmds) == 0 || m.TopCard() != model.CardAgent {
			t.Fatalf("%s: card %v, cmds %#v", state, m.TopCard(), cmds)
		}
		want := model.SetTeamPaused{ParentUID: "a", ParentRef: "work#a", Paused: paused}
		if got, ok := agentCmd[model.SetTeamPaused](cmds); !ok || got != want {
			t.Errorf("%s: cmds %#v, want %+v", state, cmds, want)
		}
	}
}

func TestPIsRefusedWithItsReasonOnAnIssueWithoutATeamAndOnAStoppingOrEndingTeam(t *testing.T) {
	for name, c := range map[string]struct {
		m      model.Model
		reason string
	}{
		"no team":  {teamModel(), model.TeamNoneReason},
		"stopping": {pausableModel(model.TeamStopping), model.TeamStoppingReason},
		"ending":   {pausableModel(model.TeamEnding), model.TeamEndingReason},
	} {
		opened, _ := press(c.m, "w")
		if reason := opened.AgentCardUnavailable(model.AgentCardPause); reason != c.reason {
			t.Errorf("%s: the button says %q, want %q", name, reason, c.reason)
		}
		after, cmds := press(opened, "p")
		if _, sent := agentCmd[model.SetTeamPaused](cmds); sent || after.TopCard() == model.CardAgent || after.Notice != c.reason {
			t.Errorf("%s: cmds %#v, card %v, notice %q; want the reason and nothing written", name, cmds, after.TopCard(), after.Notice)
		}
	}
}

func TestThePauseAnswerKeepsTheTeamsAndResumingDrivesTheQueue(t *testing.T) {
	m := drivingModel(t)
	m, _ = model.Update(m, model.TeamsDriven{})
	m, cmds := model.Update(m, model.TeamPausedSet{ParentRef: "work#a", Paused: true, Teams: []model.Team{{ParentUID: "a", State: model.TeamPaused}}})
	if t2, _ := m.TeamOn("a"); t2.State != model.TeamPaused || !strings.Contains(m.Notice, "Paused") || len(cmds) != 0 {
		t.Errorf("paused: %+v, notice %q, cmds %#v", m.Teams, m.Notice, cmds)
	}
	m, cmds = model.Update(m, model.TeamPausedSet{ParentRef: "work#a", Teams: []model.Team{{ParentUID: "a", State: model.TeamRunning}}})
	if _, driven := agentCmd[model.DriveTeams](cmds); !driven || !strings.Contains(m.Notice, "Resumed") {
		t.Errorf("resumed: notice %q, cmds %#v", m.Notice, cmds)
	}
	m, _ = model.Update(m, model.TeamPausedSet{ParentRef: "work#a", Paused: true, Err: errors.New(model.TeamEndingReason)})
	if !strings.Contains(m.Notice, "Cannot pause the team on work#a") {
		t.Errorf("notice %q", m.Notice)
	}
}
