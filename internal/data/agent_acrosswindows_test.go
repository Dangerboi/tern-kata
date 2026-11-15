package data

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func moveCalls(calls [][]string) int {
	return len(slices.DeleteFunc(slices.Clone(calls), func(c []string) bool { return c[0] != "move" }))
}

const twoWindows = `{"clients":[{"window":"","kind":"window"},{"window":"2","kind":"window"},{"window":"","kind":"cli"},{"window":"","kind":"remote"}]}`

// windowsTern answers a Tern with two windows: the Pane's own (key "") shows block 7, window 2 shows block 2^53+1 and has a
// parked pane 44. Commands that name no window are the Pane's.
func windowsTern(g *Gateway) *[][]string {
	return ternScript(g, map[string]func() ([]byte, error){
		"inspect":       ternOut(twoWindows),
		"ls":            func() ([]byte, error) { return lsOf("7"), nil },
		"ls --window 2": ternOut(`{"sessions":[{"id":9,"tabs":[{"id":5,"blocks":[{"id":9007199254740993}]}]}],"detached":[{"id":44}]}`),
		"ls --window ":  ternOut(string(lsOf("7"))),
	})
}

func TestAnAgentInAnotherWindowKeepsItsEntryAndAnAgentGoneFromEveryWindowIsPruned(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993"})
	putAgents(t, file, model.AgentEntry{Block: "44"}, model.AgentEntry{Block: "7"}, model.AgentEntry{Block: "31"})
	windowsTern(g)
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	var kept []string
	for _, e := range got.Agents {
		kept = append(kept, e.Block)
	}
	if got.Err != nil || !slices.Equal(kept, []string{"9007199254740993", "44", "7"}) {
		t.Fatalf("kept %v (err %v), want the blocks of both windows and the parked one, not 31", kept, got.Err)
	}
}

func TestAWindowThatCannotBeListedPrunesNothing(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993"})
	putAgents(t, file, model.AgentEntry{Block: "31"})
	ternScript(g, map[string]func() ([]byte, error){
		"inspect": ternOut(twoWindows),
		"ls":      func() ([]byte, error) { return lsOf("7"), nil },
		"ls --window 2": func() ([]byte, error) {
			return []byte("tern ls: no window has key 2\n"), errors.New("exit status 1")
		},
	})
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err == nil || len(got.Agents) != 2 {
		t.Fatalf("%+v, want both entries kept and the failure reported", got)
	}
}

func TestATernWithoutWindowKeysFallsBackToTheOwnWindowsListing(t *testing.T) {
	for name, inspect := range map[string]func() ([]byte, error){
		"no inspect command": func() ([]byte, error) {
			return []byte("tern: unknown command inspect\n"), errors.New("exit status 2")
		},
		"no clients field": ternOut(`{"sessions":[],"detached":[]}`),
	} {
		g, dirs := agentGateway(t)
		file := seedAgent(t, dirs, model.AgentEntry{Block: "7"})
		putAgents(t, file, model.AgentEntry{Block: "31"})
		calls := ternScript(g, map[string]func() ([]byte, error){"inspect": inspect, "ls": func() ([]byte, error) { return lsOf("7"), nil }})
		got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
		if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "7" {
			t.Errorf("%s: %+v, want only block 7 left", name, got)
		}
		for _, c := range *calls {
			if slices.Contains(c, "--window") {
				t.Errorf("%s: ran %q, want no window named", name, c)
			}
		}
	}
}

func TestFocusingAnAgentInAnotherWindowNamesThatWindow(t *testing.T) {
	g, _ := agentGateway(t)
	calls := windowsTern(g)
	inner := g.runTern
	g.runTern = func(ctx context.Context, args []string) ([]byte, error) {
		if args[0] == "focus" && !slices.Contains(args, "--window") {
			return []byte("tern focus: no block is called `9007199254740993`\n"), errors.New("exit status 1")
		}
		if args[0] == "focus" {
			*calls = append(*calls, args)
			return []byte(`{"block":9007199254740993}`), nil
		}
		return inner(ctx, args)
	}
	if got := g.focusAgent(context.Background(), model.FocusAgent{Block: "9007199254740993"}); got.Err != nil {
		t.Fatalf("focus: %v, ran %q", got.Err, *calls)
	}
	last := (*calls)[len(*calls)-1]
	if !slices.Equal(last, []string{"focus", "9007199254740993", "--window", "2"}) {
		t.Errorf("last call %q, want focus naming window 2", last)
	}
}

func TestAFocusThatFailsForAnotherReasonIsNotRetriedInAnotherWindow(t *testing.T) {
	g, _ := agentGateway(t)
	calls := windowsTern(g)
	inner := g.runTern
	g.runTern = func(ctx context.Context, args []string) ([]byte, error) {
		if args[0] == "focus" {
			return []byte("tern focus: the window is closing\n"), errors.New("exit status 1")
		}
		return inner(ctx, args)
	}
	// Block 7 is in the Pane's own window, 31 is in none.
	for _, block := range []string{"7", "31"} {
		if got := g.focusAgent(context.Background(), model.FocusAgent{Block: block}); got.Err == nil || !strings.Contains(got.Err.Error(), "closing") {
			t.Errorf("block %s: %v, want Tern's own words", block, got.Err)
		}
	}
	for _, c := range *calls {
		if c[0] == "focus" && slices.Contains(c, "--window") {
			t.Errorf("ran %q, want no retry", c)
		}
	}
}

func TestOpeningAnAgentOfAnotherWindowSaysSo(t *testing.T) {
	g, dirs := agentGateway(t)
	seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993", Background: true})
	calls := windowsTern(g)
	inner := g.runTern
	g.runTern = func(ctx context.Context, args []string) ([]byte, error) {
		if args[0] == "move" {
			return []byte("tern move: no block is called `9007199254740993`\n"), errors.New("exit status 1")
		}
		return inner(ctx, args)
	}
	got := as[model.AgentPlaced](t, run(t, g, model.OpenAgent{Block: "9007199254740993", PaneBlock: "7"}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "another Tern window") || !strings.Contains(got.Err.Error(), "no block is called") || got.Moved {
		t.Errorf("%+v, want the reason and Tern's words, nothing moved; ran %q", got, *calls)
	}
}

func TestABlockListedOnlyUnderARemoteWindowsKeyIsKept(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "55"})
	putAgents(t, file, model.AgentEntry{Block: "31"})
	ternScript(g, map[string]func() ([]byte, error){
		"inspect":       ternOut(`{"clients":[{"window":"","kind":"window"},{"window":"3","kind":"remote"},{"window":"3","kind":"cli"},{"window":"9","kind":"cli"}]}`),
		"ls":            func() ([]byte, error) { return lsOf("7"), nil },
		"ls --window ":  ternOut(string(lsOf("7"))),
		"ls --window 3": ternOut(string(lsOf("55"))),
	})
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "55" {
		t.Fatalf("%+v, want block 55 kept and 31 pruned, and no listing of the cli client's key 9", got)
	}
}
