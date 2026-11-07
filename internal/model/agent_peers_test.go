package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func peer(ref, block, address string) model.AgentEntry {
	_, short, _ := strings.Cut(ref, "#")
	return model.AgentEntry{IssueUID: "uid-" + short, IssueRef: ref, Block: block, Teammate: "omp", Address: address}
}

func addresses(agents []model.IssueAgent) []string {
	var found []string
	for _, a := range agents {
		found = append(found, a.Address)
	}
	return found
}

func TestRunningAgentsDropTheExitedAndCarryTheRelayedState(t *testing.T) {
	entries := []model.AgentEntry{peer("demo#aa11", "1", "dana/omp-1"), peer("demo#bb22", "2", "dana/omp-2"), peer("demo#cc33", "3", "dana/omp-3")}
	got := model.RunningAgents(entries, []model.AgentState{{Block: "1", State: model.AgentWorking}, {Block: "2", State: model.AgentExited}})
	if len(got) != 2 || got[0].Block != "1" || got[0].State != model.AgentWorking || got[1].Block != "3" || got[1].State != "" {
		t.Fatalf("running %+v, want block 1 working and block 3 with no state", got)
	}
}

func TestAMessageGoesToTheAddressOrToEveryAgentOnTheIssueButTheSender(t *testing.T) {
	running := model.RunningAgents([]model.AgentEntry{
		peer("demo#aa11", "1", "dana/omp-1"), peer("demo#aa11", "2", "dana/reviewer-2"), peer("demo#bb22", "3", "dana/omp-3"),
		{IssueUID: "uid-aa11", IssueRef: "demo#aa11", Block: "4", NoAddress: model.AgentNotOmp},
	}, nil)
	for _, tc := range []struct {
		to, own string
		want    []string
	}{
		{"dana/omp-3", "", []string{"dana/omp-3"}},
		{"dana/omp-1", "dana/omp-3", []string{"dana/omp-1"}},
		{"demo#aa11", "", []string{"dana/omp-1", "dana/reviewer-2"}},
		{"demo/aa11", "dana/omp-1", []string{"dana/reviewer-2"}},
		{"bb22", "", []string{"dana/omp-3"}},
		{"uid-bb22", "dana/omp-1", []string{"dana/omp-3"}},
	} {
		got, err := model.MessageTargets(running, tc.to, tc.own)
		if err != nil || strings.Join(addresses(got), ",") != strings.Join(tc.want, ",") {
			t.Errorf("to %q as %q: %v, %v, want %v", tc.to, tc.own, addresses(got), err, tc.want)
		}
	}
}

func TestAMessageToNobodyIsRefusedWithAReason(t *testing.T) {
	running := model.RunningAgents([]model.AgentEntry{
		peer("demo#aa11", "1", "dana/omp-1"), peer("demo#bb22", "2", "dana/omp-2"),
		{IssueUID: "uid-cc33", IssueRef: "demo#cc33", Block: "3", NoAddress: model.AgentNotOmp},
	}, []model.AgentState{{Block: "2", State: model.AgentExited}})
	for _, tc := range []struct{ to, own, want string }{
		{"dana/omp-9", "", "no running agent"},
		{"demo#bb22", "", "no running agent"}, // its agent exited
		{"demo#zz99", "", "no running agent"},
		{"demo#cc33", "", "can receive a message"},
		{"demo#aa11", "dana/omp-1", "can receive a message"}, // only the sender is there
		{"", "", "no running agent"},
	} {
		if got, err := model.MessageTargets(running, tc.to, tc.own); err == nil || got != nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("to %q: %v, %v, want an error with %q", tc.to, got, err, tc.want)
		}
	}
}

func TestAMessageIsTrimmedAndRefusedWhenEmptyOrTooLong(t *testing.T) {
	if text, err := model.CheckAgentMessage("  rebase first \n"); err != nil || text != "rebase first" {
		t.Fatalf("got %q, %v", text, err)
	}
	for _, text := range []string{"", " \n\t", strings.Repeat("a", 1025)} {
		if _, err := model.CheckAgentMessage(text); err == nil {
			t.Errorf("a message of %d bytes was accepted", len(text))
		}
	}
	if _, err := model.CheckAgentMessage(strings.Repeat("a", 1024)); err != nil {
		t.Errorf("1024 bytes refused: %v", err)
	}
}
