package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

var teamNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func TestTeamTakesOneRef(t *testing.T) {
	for _, ref := range []string{"demo#aa11", "demo/aa11", "aa11"} {
		plan, err := PlanLaunch([]string{"team", ref}, peerEnv(nil), "test")
		if err != nil || plan.Subcommand != "team" || plan.TargetRef != ref {
			t.Errorf("team %s: plan %+v, %v", ref, plan, err)
		}
	}
	for _, args := range [][]string{{"team"}, {"team", "a", "b"}, {"team", "a", "--as", "bob"}, {"team", "--issue", "a"}} {
		if _, err := PlanLaunch(args, peerEnv(nil), "test"); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	if plan, err := PlanLaunch([]string{"team", "--help"}, peerEnv(nil), "test"); err != nil || !plan.Help {
		t.Errorf("--help: %+v, %v", plan, err)
	}
}

// teamFixture is a team on demo#aa11 with a child in each state.
func teamFixture() (model.Team, []model.AgentEntry, []model.AgentState, []model.TeamKid) {
	team := model.Team{ParentUID: "uid-aa11", ParentRef: "demo#aa11", ProjectID: 3, Project: "demo", CoordinatorBlock: "1",
		CoordinatorAddress: "dana/omp-1", Limit: 2, State: model.TeamRunning,
		Children: []model.TeamChild{
			{UID: "uid-c1", Ref: "demo#c1c1", Block: "11", StartedAt: teamNow.Add(-time.Hour)},
			{UID: "uid-c2", Ref: "demo#c2c2", Block: "12", StartedAt: teamNow.Add(-time.Hour)},
			{UID: "uid-c3", Ref: "demo#c3c3", Block: "13", StartedAt: teamNow.Add(-time.Hour)},
			{UID: "uid-c4", Ref: "demo#c4c4", Block: "", StartedAt: teamNow.Add(-time.Second)},
			{UID: "uid-gone", Ref: "demo#gone", Block: "", StartedAt: teamNow.Add(-time.Hour)},
		}}
	entries := []model.AgentEntry{
		{IssueUID: "uid-c1", IssueRef: "demo#c1c1", Block: "11", Address: "dana/omp-11"},
		{IssueUID: "uid-c2", IssueRef: "demo#c2c2", Block: "12", Address: "dana/omp-12"},
		{IssueUID: "uid-c3", IssueRef: "demo#c3c3", Block: "13", Address: "dana/omp-13"},
		{IssueUID: "uid-c7", IssueRef: "demo#c7c7", Block: "17", Address: "dana/omp-17"},
	}
	states := []model.AgentState{{Block: "11", State: model.AgentWorking}, {Block: "12", State: model.AgentExited}, {Block: "13", State: model.AgentExited}}
	kids := []model.TeamKid{
		{UID: "uid-c1", Ref: "demo#c1c1", ProjectID: 3},
		{UID: "uid-c2", Ref: "demo#c2c2", ProjectID: 3, Closed: true},
		{UID: "uid-c3", Ref: "demo#c3c3", ProjectID: 3},
		{UID: "uid-c4", Ref: "demo#c4c4", ProjectID: 3},
		{UID: "uid-c5", Ref: "demo#c5c5", ProjectID: 3, Ready: true},
		{UID: "uid-c6", Ref: "demo#c6c6", ProjectID: 3},
		{UID: "uid-c7", Ref: "demo#c7c7", ProjectID: 3, Ready: true},
		{UID: "uid-c8", Ref: "other#c8c8", ProjectID: 4, Ready: true},
		{UID: "uid-c9", Ref: "demo#c9c9", ProjectID: 3, Closed: true},
	}
	return team, entries, states, kids
}

func runTeamFixture(t *testing.T, ref string, teams []model.Team, kidsErr error) (string, error) {
	t.Helper()
	_, entries, states, kids := teamFixture()
	files := func() ([]model.Team, []model.AgentEntry, []model.AgentState, error) {
		return teams, entries, states, nil
	}
	send := func(cmd model.Cmd) model.Msg {
		load := cmd.(model.LoadTeamKids)
		if load.ProjectID != 3 || load.ParentUID != "uid-aa11" {
			t.Errorf("read the children of %+v", load)
		}
		return model.TeamKidsLoaded{Kids: kids, Err: kidsErr}
	}
	var out bytes.Buffer
	err := runTeam(LaunchPlan{Subcommand: "team", TargetRef: ref}, "/w", files, teamNow, send, &out)
	return out.String(), err
}

func TestTeamPrintsTheTeamAndARowForEachChildState(t *testing.T) {
	team, _, _, _ := teamFixture()
	for _, ref := range []string{"demo#aa11", "demo/aa11", "aa11", "uid-aa11"} {
		out, err := runTeamFixture(t, ref, []model.Team{{ParentUID: "uid-other"}, team}, nil)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		if !strings.HasPrefix(out, "Team on demo#aa11: running, limit 2, coordinator dana/omp-1\n") {
			t.Errorf("%s: header of\n%s", ref, out)
		}
	}
	out, _ := runTeamFixture(t, "aa11", []model.Team{team}, nil)
	for _, want := range []string{
		"demo#c1c1  agent working                         dana/omp-11",
		"demo#c2c2  ran: closed                     -",
		"demo#c3c3  ran: exited with the Issue open  -",
		"demo#c4c4  starting                        -",
		"demo#c5c5  queued                          -",
		"demo#c6c6  not ready                       -",
		"demo#c7c7  agent                         dana/omp-17",
		"other#c8c8  skipped: other Project          -",
	} {
		if !hasRow(out, want) {
			t.Errorf("no row %q in\n%s", want, out)
		}
	}
	for _, absent := range []string{"gone", "c9c9"} {
		if strings.Contains(out, absent) {
			t.Errorf("%s is listed:\n%s", absent, out)
		}
	}
}

// hasRow compares a line of out with want ignoring the width of the gaps.
func hasRow(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.Join(strings.Fields(line), " ") == strings.Join(strings.Fields(want), " ") {
			return true
		}
	}
	return false
}

func TestTeamWithNoTeamOnTheIssueSaysSoAndFails(t *testing.T) {
	out, err := runTeamFixture(t, "demo#zz99", nil, nil)
	var failed subcommandError
	if !errors.As(err, &failed) || !strings.Contains(err.Error(), "no team on demo#zz99") || out != "" {
		t.Errorf("err %v, output %q", err, out)
	}
}

func TestTeamNeedsTheChildrenFromKata(t *testing.T) {
	team, _, _, _ := teamFixture()
	if _, err := runTeamFixture(t, "aa11", []model.Team{team}, errors.New("daemon down")); err == nil || !strings.Contains(err.Error(), "daemon down") {
		t.Errorf("err %v", err)
	}
}
