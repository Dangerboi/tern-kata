package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

func TestOnlyATernAtLeastThe0921ReleaseIsKnownToHaveTheSubcommands(t *testing.T) {
	for _, c := range []struct {
		out  string
		want bool
	}{
		{"tern 0.9.21 (bde0861)\n", true},
		{"tern 0.9.22", true},
		{"tern 0.10.0 (x)", true},
		{"tern 1.0.0", true},
		{"tern 0.9.20 (x)", false},
		{"tern 0.8.99", false},
		{"garbage", false},
		{"", false},
		{"tern 0.9", false},
		{"tern 0.9.x", false},
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
			if got := g.ternHasSubcommands(context.Background()); got != c.want {
				t.Errorf("%q: %v, want %v", c.out, got, c.want)
			}
		}
		if asked != 1 {
			t.Errorf("%q: asked %d times, want once (an answer is kept, old or not)", c.out, asked)
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
	if g.ternHasSubcommands(context.Background()) {
		t.Fatal("a failed ask counted as known")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if !g.ternHasSubcommands(cancelled) {
		t.Fatal("the second ask did not allow a current Tern")
	}
	if !g.ternHasSubcommands(context.Background()) || asked != 2 {
		t.Fatalf("asked %d times, want 2 with the answer then kept", asked)
	}
}

func TestSaveOnATernWithoutDownloadNeverAsksForOneAndSavesHere(t *testing.T) {
	for name, c := range map[string]struct {
		version string
		err     error
	}{
		"an older Tern":        {"tern 0.9.20 (x)", nil},
		"an unreadable one":    {"garbage", nil},
		"a version that fails": {"", errors.New("exit status 2")},
	} {
		t.Run(name, func(t *testing.T) {
			got, downloads, asked := saveFixtureWithVersion(t, true, c.version, c.err, "notes.txt", func([]string) ([]byte, error) {
				return []byte("/elsewhere/notes.txt\n"), nil
			})
			if len(*asked) != 0 {
				t.Fatalf("tern was asked %v", *asked)
			}
			want := filepath.Join(downloads, "notes.txt")
			if got.Err != nil || got.Path != want || got.Remote || got.Note != "" {
				t.Fatalf("saved %+v, want %s here", got, want)
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
		"ls --window ":  ternOut(string(lsOf("7"))),
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
