package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const modernTernVersion = "tern 0.9.21 (bde0861)\n"

// ternProbe answers the calls a stub that cares about neither makes of a current Tern: its version and its window clients.
func ternProbe(args []string) ([]byte, bool) {
	switch args[0] {
	case "--version":
		return []byte(modernTernVersion), true
	case "inspect":
		return []byte(`{"clients":[]}`), true
	}
	return nil, false
}

func TestAtLeastTheSubcommandsFloorReadsSemverVersions(t *testing.T) {
	for _, c := range []struct {
		out  string
		want bool
	}{
		{"tern 0.9.21 (bde0861)\n", true},
		{"tern v0.9.21", true},
		{"tern 0.9.22", true},
		{"tern 0.9.22-dev", true},
		{"tern v0.9.22-dev (abc)", true},
		{"tern 0.10.0 (x)", true},
		{"tern 1.0.0", true},
		{"tern 0.9.21-dev", false},
		{"tern 0.9.20 (x)", false},
		{"tern 0.8.99", false},
		{"tern 0.9", false},
	} {
		g := NewGateway(nil)
		asked := 0
		g.runTern = func(_ context.Context, args []string) ([]byte, error) {
			asked++
			if !slices.Equal(args, []string{"--version"}) {
				t.Errorf("ran %q", args)
			}
			return []byte(c.out), nil
		}
		for range 3 {
			if got, err := g.ternAtLeast(context.Background(), ternSubcommandsFloor); err != nil || got != c.want {
				t.Errorf("%q: %v (%v), want %v", c.out, got, err, c.want)
			}
		}
		if asked != 1 {
			t.Errorf("%q: asked %d times, want once (an answer is kept, old or not)", c.out, asked)
		}
	}
}

func TestAnUnreadableVersionIsAnErrorAndIsAskedAgain(t *testing.T) {
	for _, out := range []string{"garbage", "", "tern", "tern 0.9.x", "tern latest"} {
		g := NewGateway(nil)
		asked := 0
		g.runTern = func(context.Context, []string) ([]byte, error) { asked++; return []byte(out), nil }
		for range 2 {
			if has, err := g.ternAtLeast(context.Background(), ternSubcommandsFloor); has || err == nil {
				t.Errorf("%q: %v, %v, want an error", out, has, err)
			}
		}
		if asked != 2 {
			t.Errorf("%q: asked %d times, want 2 (a failure is not kept)", out, asked)
		}
	}
}

func TestAFailedVersionAskIsAskedAgainAndTheProbeOutlivesTheCallersContext(t *testing.T) {
	g := NewGateway(nil)
	asked := 0
	g.runTern = func(ctx context.Context, _ []string) ([]byte, error) {
		asked++
		if asked == 1 {
			return nil, errors.New("exit status 1")
		}
		if ctx.Err() != nil {
			t.Errorf("the version ask ran under the caller's cancelled context")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Errorf("the version ask has no deadline of its own")
		}
		return []byte(modernTernVersion), nil
	}
	if has, err := g.ternAtLeast(context.Background(), ternSubcommandsFloor); has || err == nil {
		t.Fatalf("a failed ask gave %v, %v, want an error", has, err)
	}
	if has, err := g.ternAtLeast(context.Background(), ternSubcommandsFloor); !has || err != nil || asked != 2 {
		t.Fatalf("asked %d times, want 2 with a current Tern allowed (%v, %v)", asked, has, err)
	}
}

func TestACallerWhoseContextEndsStopsWaitingForTheVersionAndOthersShareTheAsk(t *testing.T) {
	g := NewGateway(nil)
	release := make(chan struct{})
	var asked atomic.Int32
	g.runTern = func(context.Context, []string) ([]byte, error) {
		asked.Add(1)
		<-release
		return []byte(modernTernVersion), nil
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if has, err := g.ternAtLeast(short, ternSubcommandsFloor); has || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the caller whose context ended got %v, %v", has, err)
	}
	go close(release)
	if has, err := g.ternAtLeast(context.Background(), ternSubcommandsFloor); !has || err != nil || asked.Load() != 1 {
		t.Fatalf("the second caller got %v, %v after %d asks, want the one ask's answer", has, err, asked.Load())
	}
}

func TestAnUnreadableVersionPrunesNothing(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993"})
	putAgents(t, file, model.AgentEntry{Block: "7"})
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		switch args[0] {
		case "--version":
			return []byte("tern 0.9.22-dev (abc)"), nil
		case "inspect":
			return []byte(twoWindows), nil
		case "ls":
			if slices.Contains(args, "--window") {
				return []byte(`{"sessions":[{"id":9,"tabs":[{"id":5,"blocks":[{"id":9007199254740993}]}]}]}`), nil
			}
			return lsOf("7"), nil
		}
		return nil, errors.New("unscripted " + strings.Join(args, " "))
	}
	if got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{})); got.Err != nil || len(got.Agents) != 2 {
		t.Fatalf("%+v: a dev build must be read as a current Tern and keep both windows' agents", got)
	}
	g, dirs = agentGateway(t)
	file = seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993"})
	putAgents(t, file, model.AgentEntry{Block: "7"})
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		switch args[0] {
		case "--version":
			return []byte("tern next"), nil
		case "ls":
			return lsOf("7"), nil
		}
		return nil, errors.New("unscripted " + strings.Join(args, " "))
	}
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err == nil || len(got.Agents) != 2 {
		t.Fatalf("%+v, want both entries kept and the unreadable version reported", got)
	}
}

func TestAFailingVersionAskPrunesNothing(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993"})
	putAgents(t, file, model.AgentEntry{Block: "7"})
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		switch args[0] {
		case "--version":
			return []byte("tern: no such thing\n"), errors.New("exit status 2")
		case "ls":
			return lsOf("7"), nil
		}
		return nil, errors.New("unscripted " + strings.Join(args, " "))
	}
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err == nil || len(got.Agents) != 2 {
		t.Fatalf("%+v, want both entries kept and the failure reported", got)
	}
}

func TestSaveOnATernWithoutDownloadNeverAsksForOneAndSavesHere(t *testing.T) {
	for name, c := range map[string]struct {
		version string
		err     error
		note    string
	}{
		"an older Tern":        {"tern 0.9.20 (x)", nil, ""},
		"an unreadable one":    {"garbage", nil, `tern --version printed "garbage"`},
		"a version that fails": {"", errors.New("exit status 2"), "tern --version: exit status 2"},
	} {
		t.Run(name, func(t *testing.T) {
			got, downloads, asked := saveFixtureWithVersion(t, true, c.version, c.err, "notes.txt", func([]string) ([]byte, error) {
				return []byte("/elsewhere/notes.txt\n"), nil
			})
			if len(*asked) != 0 {
				t.Fatalf("tern was asked %v", *asked)
			}
			want := filepath.Join(downloads, "notes.txt")
			if got.Err != nil || got.Path != want || got.Remote || !strings.Contains(got.Note, c.note) || (c.note == "" && got.Note != "") {
				t.Fatalf("saved %+v, want %s here with note %q", got, want, c.note)
			}
			if body, err := os.ReadFile(want); err != nil || string(body) != "content" {
				t.Fatalf("the copy holds %q (%v)", body, err)
			}
		})
	}
}

func TestLivenessOnAnOlderTernNeverAsksForInspect(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "7"})
	putAgents(t, file, model.AgentEntry{Block: "31"})
	var calls [][]string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		calls = append(calls, args)
		switch args[0] {
		case "--version":
			return []byte("tern 0.9.20 (x)"), nil
		case "ls":
			return lsOf("7"), nil
		}
		return nil, errors.New("unscripted " + strings.Join(args, " "))
	}
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "7" {
		t.Fatalf("%+v, want only block 7 left", got)
	}
	for _, c := range calls {
		if c[0] == "inspect" {
			t.Errorf("ran %q on a Tern without inspect", c)
		}
	}
}

func TestGoingBackToAnEmptySessionNamesAnotherWindowWhenThePlacementFails(t *testing.T) {
	g, dirs := agentGateway(t)
	seedAgent(t, dirs, model.AgentEntry{Block: "9007199254740993", Open: true})
	calls := ternScript(g, map[string]func() ([]byte, error){
		"inspect":       ternOut(twoWindows),
		"ls":            func() ([]byte, error) { return lsOf("7"), nil },
		"ls --window 2": ternOut(`{"sessions":[{"id":9,"tabs":[{"id":5,"blocks":[{"id":9007199254740993}]}]}]}`),
		"new session":   ternOut(`{"session":3,"tab":9,"block":40}`),
		"close":         ternOut(`{"block":40}`),
		"move": func() ([]byte, error) {
			return []byte("tern move: no block is called `9007199254740993`\n"), errors.New("exit status 1")
		},
	})
	got := as[model.AgentPlaced](t, run(t, g, model.SendAgentBack{Block: "9007199254740993", IssueRef: "demo#ab12"}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "another Tern window") || !strings.Contains(got.Err.Error(), "no block is called") || got.Moved {
		t.Fatalf("%+v, want the reason and Tern's words, nothing moved; ran %q", got, *calls)
	}
	closed := false
	for _, c := range *calls {
		closed = closed || slices.Equal(c, []string{"close", "40", "--json"})
	}
	if !closed {
		t.Errorf("the placeholder shell was not closed; ran %q", *calls)
	}
}
