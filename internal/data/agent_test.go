package data

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func writeAgentFile(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAgentTomlUnsetTopLevelAndPerProject(t *testing.T) {
	dir := t.TempDir()
	unset, err := ReadAgentConfig(filepath.Join(dir, "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := unset.For("demo"); got.Actor != "" || !slices.Equal(got.Command, []string{"omp"}) || got.Checkout != "" {
		t.Errorf("unset: %+v", got)
	}
	path := writeAgentFile(t, filepath.Join(dir, "agent.toml"), `
actor = "me-agent"
command = ["omp", "--fast"]
[projects.demo]
actor = "demo-agent"
command = ["omp", "--model", "opus"]
checkout = "/work/demo"
[projects.plain]
checkout = "/work/plain"
`)
	cfg, err := ReadAgentConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.For("other"); got.Actor != "me-agent" || !slices.Equal(got.Command, []string{"omp", "--fast"}) || got.Checkout != "" {
		t.Errorf("top level: %+v", got)
	}
	if got := cfg.For("demo"); got.Actor != "demo-agent" || !slices.Equal(got.Command, []string{"omp", "--model", "opus"}) || got.Checkout != "/work/demo" {
		t.Errorf("per project: %+v", got)
	}
	if got := cfg.For("plain"); got.Actor != "me-agent" || !slices.Equal(got.Command, []string{"omp", "--fast"}) || got.Checkout != "/work/plain" {
		t.Errorf("a table without values keeps the top level's: %+v", got)
	}
}

func TestAgentTomlRefusesWhatItCannotUse(t *testing.T) {
	for name, c := range map[string]struct{ text, want string }{
		"unknown key":         {"colour = \"red\"\n", "unknown key colour"},
		"unknown project key": {"[projects.demo]\nbranch = \"x\"\n", "unknown key projects.demo.branch"},
		"top level checkout":  {"checkout = \"/work\"\n", "unknown key checkout"},
		"relative checkout":   {"[projects.demo]\ncheckout = \"work/demo\"\n", "not an absolute directory"},
		"empty command":       {"command = []\n", "must name a program"},
		"blank program":       {"[projects.demo]\ncommand = [\"\"]\n", "must name a program"},
		"not toml":            {"actor = \n", "agent.toml"},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeAgentFile(t, filepath.Join(t.TempDir(), "agent.toml"), c.text)
			_, err := ReadAgentConfig(path)
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "agent.toml") {
				t.Fatalf("error %v, want one naming agent.toml and %q", err, c.want)
			}
		})
	}
}

func TestTheAgentDirectoryIsTheBindingThenAgentTomlThenWorkerToml(t *testing.T) {
	dir := t.TempDir()
	workerPath := writeAgentFile(t, filepath.Join(dir, "worker.toml"), `
executor = "laptop"
[profiles.default]
[checkouts]
a = "/work/a"
b = "/work/b"
[[projects]]
name = "demo"
checkouts = ["b", "a"]
[[projects]]
name = "open"
`)
	toml := AgentSettings{Checkout: "/agent/demo"}
	for _, c := range []struct {
		name, binding, project string
		settings               AgentSettings
		want                   string
	}{
		{"binding first", "/bound", "demo", toml, "/bound"},
		{"agent.toml before worker.toml", "", "demo", toml, "/agent/demo"},
		{"first worker.toml checkout", "", "demo", AgentSettings{}, "/work/b"},
	} {
		got, err := AgentDir(c.binding, c.project, c.settings, workerPath)
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	for _, c := range []struct{ project, path string }{{"open", workerPath}, {"unlisted", workerPath}, {"demo", filepath.Join(dir, "none.toml")}} {
		if got, err := AgentDir("", c.project, AgentSettings{}, c.path); err == nil || !strings.Contains(err.Error(), "agent.toml") {
			t.Errorf("%s: %q, %v; want a refusal naming agent.toml", c.project, got, err)
		}
	}
}

func TestAgentArgvBuilders(t *testing.T) {
	line := AgentCommandLine("", []string{"omp", "--fast"}, "/state/p.md")
	if want := []string{"omp", "--fast", "@/state/p.md"}; !slices.Equal(line, want) {
		t.Errorf("without an actor %q, want %q", line, want)
	}
	wrapped := AgentCommandLine("me-agent", []string{"omp"}, "/state/p.md")
	if want := []string{"tern-kata", "agent", "--as", "me-agent", "--", "omp", "@/state/p.md"}; !slices.Equal(wrapped, want) {
		t.Errorf("with an actor %q, want %q", wrapped, want)
	}
	split := AgentSplitArgs("24489903521795", "/work/my repo", line)
	if want := []string{"split", "24489903521795", "right", "--cwd", "/work/my repo", "--json", "--", "omp", "--fast", "@/state/p.md"}; !slices.Equal(split, want) {
		t.Errorf("split %q, want %q", split, want)
	}
	if got := AgentSplitArgs("", "/w", line); got[1] != "@focused" {
		t.Errorf("a Pane that cannot name its block splits %q", got[1])
	}
	tab := AgentTabArgs("/work", wrapped)
	if want := []string{"new", "tab", "--cwd", "/work", "--json", "--", "tern-kata", "agent", "--as", "me-agent", "--", "omp", "@/state/p.md"}; !slices.Equal(tab, want) {
		t.Errorf("tab %q, want %q", tab, want)
	}
}

func TestTheBlockIdOfTernJSONKeepsItsDigits(t *testing.T) {
	got, err := ParseTernBlock([]byte("note\n{\n  \"session\": 1,\n  \"tab\": 21500606283826,\n  \"block\": 18446744073709551615\n}\n"))
	if err != nil || got != "18446744073709551615" {
		t.Fatalf("%q, %v", got, err)
	}
	for _, bad := range []string{"", "not json", `{"session": 1}`} {
		if _, err := ParseTernBlock([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestAgentsJSONRoundTripsAndEveryChangeStartsFromTheFileAsItIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "agents.json")
	if list, err := ReadAgents(path); err != nil || list != nil {
		t.Fatalf("missing file: %v, %v", list, err)
	}
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	first := model.AgentEntry{IssueUID: "U1", IssueRef: "demo#ab12", Block: "11", StartedAt: at, PromptFile: "/s/p1.md", PaneBlock: "7"}
	if _, err := UpdateAgents(path, func(l []model.AgentEntry) []model.AgentEntry { return append(l, first) }); err != nil {
		t.Fatal(err)
	}
	// Another Pane writes between this Pane's reads: the change sees its entry.
	other := model.AgentEntry{IssueUID: "U2", IssueRef: "demo#cd34", Block: "22", StartedAt: at, PaneBlock: "8", Background: true, Open: true}
	if _, err := UpdateAgents(path, func(l []model.AgentEntry) []model.AgentEntry { return append(l, other) }); err != nil {
		t.Fatal(err)
	}
	var saw int
	got, err := UpdateAgents(path, func(l []model.AgentEntry) []model.AgentEntry { saw = len(l); return l })
	if err != nil || saw != 2 || !slices.Equal(got, []model.AgentEntry{first, other}) {
		t.Fatalf("saw %d, got %+v, %v", saw, got, err)
	}
	read, err := ReadAgents(path)
	if err != nil || !slices.EqualFunc(read, got, func(a, b model.AgentEntry) bool { return a == b }) {
		t.Fatalf("read %+v, %v", read, err)
	}
	writeAgentFile(t, path, "{broken")
	if _, err := ReadAgents(path); err == nil {
		t.Error("a broken agents.json was read")
	}
}

func agentGateway(t *testing.T) (*Gateway, worker.Dirs) {
	t.Helper()
	dirs := worker.Dirs{Config: t.TempDir(), State: t.TempDir()}
	g := NewGateway(nil)
	g.workerDirs = func() (worker.Dirs, error) { return dirs, nil }
	g.now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 7, 0, time.UTC) }
	return g, dirs
}

func TestStartingAnAgentWritesThePromptRunsTernAndRecordsTheBlock(t *testing.T) {
	g, dirs := agentGateway(t)
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "actor = \"me-agent\"\n[projects.demo]\ncheckout = \"/work/demo\"\n")
	var ran []string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		ran = args
		return []byte(`{"session": 1, "tab": 2, "block": 31}`), nil
	}
	got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentSplit, IssueUID: "U1", IssueRef: "demo#ab12", IssueTitle: "Fix it",
		ProjectName: "demo", PaneBlock: "7"}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	e := got.Entry
	if e.Block != "31" || e.IssueUID != "U1" || e.IssueRef != "demo#ab12" || e.PaneBlock != "7" || e.Background || e.Open ||
		!e.StartedAt.Equal(time.Date(2026, 10, 29, 12, 0, 7, 0, time.UTC)) || filepath.Dir(e.PromptFile) != filepath.Join(dirs.State, AgentPromptsFolder) {
		t.Fatalf("entry %+v", e)
	}
	want := []string{"split", "7", "right", "--cwd", "/work/demo", "--json", "--", "tern-kata", "agent", "--as", "me-agent", "--", "omp", "@" + e.PromptFile}
	if !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	text, err := os.ReadFile(e.PromptFile)
	if err != nil || string(text) != AgentPrompt("demo#ab12", "Fix it") {
		t.Fatalf("prompt %q, %v", text, err)
	}
	if stored, err := ReadAgents(filepath.Join(dirs.State, AgentsFile)); err != nil || len(stored) != 1 || stored[0] != e {
		t.Fatalf("agents.json %+v, %v", stored, err)
	}
	// W starts a tab, in the Binding's workspace, and the command runs directly without an actor.
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentTab, IssueRef: "demo#cd34", ProjectName: "demo", Workspace: "/bound"}))
	if len(ran) < 8 || !slices.Equal(ran[:7], []string{"new", "tab", "--cwd", "/bound", "--json", "--", "omp"}) {
		t.Fatalf("W ran %q", ran)
	}
}

func TestAgentPromptNameCarriesTheStartingPaneOfATab(t *testing.T) {
	for _, c := range []struct{ starter, want string }{
		{"24489903521795", "prompt-24489903521795-*.md"},
		{"", "prompt-*.md"},
		{"7/../x", "prompt-*.md"},
		{"a1", "prompt-*.md"},
	} {
		if got := AgentPromptName(c.starter); got != c.want {
			t.Errorf("AgentPromptName(%q) = %q, want %q", c.starter, got, c.want)
		}
	}
	g, dirs := agentGateway(t)
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(lsWithAgents), nil
		}
		return []byte(`{"session":1,"tab":2,"block":31}`), nil
	}
	for _, c := range []struct {
		placement model.AgentPlacement
		pattern   string
	}{
		{model.AgentTab, `^prompt-7-[0-9]+\.md$`},
		{model.AgentSplit, `^prompt-[0-9]+\.md$`},
		{model.AgentBackground, `^prompt-7-[0-9]+\.md$`},
	} {
		started := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: c.placement, IssueRef: "demo#ab12", ProjectName: "demo", Workspace: "/bound", PaneBlock: "7"}))
		if started.Err != nil {
			t.Fatal(started.Err)
		}
		if name := filepath.Base(started.Entry.PromptFile); !regexp.MustCompile(c.pattern).MatchString(name) {
			t.Errorf("placement %v: prompt file %q does not match %s", c.placement, name, c.pattern)
		}
	}
}

func TestAnAgentThatCannotStartLeavesNoPromptAndNoEntry(t *testing.T) {
	g, dirs := agentGateway(t)
	cmd := model.StartAgent{Placement: model.AgentSplit, IssueRef: "demo#ab12", ProjectName: "demo", Workspace: "/bound"}
	g.runTern = func(context.Context, []string) ([]byte, error) {
		return []byte("tern: no such block\n"), errors.New("exit status 1")
	}
	failed := as[model.AgentStarted](t, run(t, g, cmd))
	if failed.Err == nil || failed.Err.Error() != "exit status 1: tern: no such block" {
		t.Errorf("failure %v", failed.Err)
	}
	if left, _ := os.ReadDir(filepath.Join(dirs.State, AgentPromptsFolder)); len(left) != 0 {
		t.Errorf("prompt files left: %v", left)
	}
	if stored, _ := ReadAgents(filepath.Join(dirs.State, AgentsFile)); len(stored) != 0 {
		t.Errorf("entries left: %v", stored)
	}
	// A project with no directory, and an agent.toml that does not parse, start nothing and ask tern nothing.
	g.runTern = func(context.Context, []string) ([]byte, error) { t.Error("tern was asked"); return nil, nil }
	cmd.Workspace = ""
	if got := as[model.AgentStarted](t, run(t, g, cmd)); got.Err == nil || !strings.Contains(got.Err.Error(), "agent.toml") {
		t.Errorf("no directory: %v", got.Err)
	}
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "bogus = 1\n")
	cmd.Workspace = "/bound"
	if got := as[model.AgentStarted](t, run(t, g, cmd)); got.Err == nil || !strings.Contains(got.Err.Error(), "unknown key bogus") {
		t.Errorf("bad agent.toml: %v", got.Err)
	}
}

func TestPruningDropsTheEntriesTernNoLongerListsWithTheirPrompts(t *testing.T) {
	g, dirs := agentGateway(t)
	prompts := filepath.Join(dirs.State, AgentPromptsFolder)
	if err := os.MkdirAll(prompts, 0o700); err != nil {
		t.Fatal(err)
	}
	live := writeAgentFile(t, filepath.Join(prompts, "live.md"), "x")
	dead := writeAgentFile(t, filepath.Join(prompts, "dead.md"), "x")
	file := filepath.Join(dirs.State, AgentsFile)
	if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry {
		return append(l, model.AgentEntry{Block: "18494129176578", PromptFile: live}, model.AgentEntry{Block: "99", PromptFile: dead})
	}); err != nil {
		t.Fatal(err)
	}
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		if !slices.Equal(args, []string{"ls", "--json"}) {
			t.Errorf("ran %q", args)
		}
		return []byte(`{"sessions":[{"id":1,"tabs":[{"id":5,"blocks":[{"id":18494129176578},{"id":24489903521795}]}]}]}`), nil
	}
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "18494129176578" {
		t.Fatalf("pruned %+v", got)
	}
	if _, err := os.Stat(dead); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the pruned entry's prompt file stayed: %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("a live entry's prompt file went: %v", err)
	}
	// When tern cannot be asked nothing is dropped.
	g.runTern = func(context.Context, []string) ([]byte, error) { return nil, errors.New("exit status 1") }
	failed := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if failed.Err == nil {
		t.Error("a failed tern ls pruned without error")
	}
	stored, _ := ReadAgents(file)
	if len(stored) != 1 {
		t.Errorf("entries after a failed ls: %v", stored)
	}
	if len(failed.Agents) != 1 || failed.Agents[0].Block != "18494129176578" {
		t.Errorf("entries answered after a failed ls: %v", failed.Agents)
	}
}

func TestPruningKeepsAnEntryAddedAfterTheEntriesItChecked(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	prompt := writeAgentFile(t, filepath.Join(dirs.State, "new.md"), "x")
	if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry {
		return append(l, model.AgentEntry{Block: "99"})
	}); err != nil {
		t.Fatal(err)
	}
	g.runTern = func(context.Context, []string) ([]byte, error) {
		// Another Pane starts an agent while tern ls is answering.
		if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry {
			return append(l, model.AgentEntry{Block: "123", PromptFile: prompt})
		}); err != nil {
			t.Fatal(err)
		}
		return []byte(`{"sessions":[{"id":1,"tabs":[{"id":5,"blocks":[{"id":7}]}]}]}`), nil
	}
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "123" {
		t.Fatalf("pruned %+v", got)
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Errorf("the new entry's prompt file went: %v", err)
	}
}

func TestTheAgentPromptIsStable(t *testing.T) {
	got := AgentPrompt("demo#ab12", "  Fix the\n# crash\tnow ")
	path := "testdata/agent_prompt.golden"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("prompt differs from %s (run with -update to rewrite):\n%s", path, got)
	}
}

func lsOf(blocks ...string) []byte {
	var list []string
	for _, b := range blocks {
		list = append(list, `{"id":`+b+`}`)
	}
	return []byte(`{"sessions":[{"id":1,"tabs":[{"id":5,"blocks":[` + strings.Join(list, ",") + `]}]}]}`)
}

// lsCounter makes runTern answer `tern ls --json` with the blocks in *live and counts the asks.
func lsCounter(t *testing.T, g *Gateway, live *[]string) *int {
	t.Helper()
	asked := 0
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		if !slices.Equal(args, []string{"ls", "--json"}) {
			t.Errorf("ran %q", args)
		}
		asked++
		return lsOf(*live...), nil
	}
	return &asked
}

func putAgents(t *testing.T, file string, entries ...model.AgentEntry) {
	t.Helper()
	if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry { return append(l, entries...) }); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshingReadsTheFileAsItIsAndAsksTernNothingWithoutSuspects(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "1"}, model.AgentEntry{Block: "2"})
	g.runTern = func(context.Context, []string) ([]byte, error) { t.Error("refreshing ran tern"); return nil, nil }
	if got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{})); got.Err != nil || len(got.Agents) != 2 {
		t.Fatalf("a plain re-read: %+v", got)
	}
}

func TestRefreshingDropsASuspectOnlyWhenTernNoLongerListsItsBlock(t *testing.T) {
	g, dirs := agentGateway(t)
	prompts := filepath.Join(dirs.State, AgentPromptsFolder)
	if err := os.MkdirAll(prompts, 0o700); err != nil {
		t.Fatal(err)
	}
	listed := writeAgentFile(t, filepath.Join(prompts, "listed.md"), "x")
	dead := writeAgentFile(t, filepath.Join(prompts, "dead.md"), "x")
	other := writeAgentFile(t, filepath.Join(prompts, "other.md"), "x")
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file,
		model.AgentEntry{Block: "1", PaneBlock: "7", PromptFile: listed},
		model.AgentEntry{Block: "144115188075855871", PaneBlock: "7", PromptFile: dead},
		model.AgentEntry{Block: "3", PaneBlock: "7", PromptFile: other})
	live := []string{"1", "7"}
	asked := lsCounter(t, g, &live)
	got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{Suspects: []string{"1", "144115188075855871"}, PaneBlock: "7"}))
	if got.Err != nil || len(got.Agents) != 2 || got.Agents[0].Block != "1" || got.Agents[1].Block != "3" {
		t.Fatalf("refreshed %+v", got)
	}
	if *asked != 1 {
		t.Errorf("tern ls ran %d times", *asked)
	}
	if _, err := os.Stat(dead); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the gone entry's prompt stayed: %v", err)
	}
	for _, p := range []string{listed, other} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("a kept entry's prompt went: %v", err)
		}
	}
}

func TestRefreshingKeepsEverythingWhenTernCannotBeAsked(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "1", PaneBlock: "7"})
	g.runTern = func(context.Context, []string) ([]byte, error) { return nil, errors.New("exit status 1") }
	got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{Suspects: []string{"1"}, PaneBlock: "9"}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].PaneBlock != "7" {
		t.Fatalf("refreshed %+v", got)
	}
}

func TestTheLiveBlocksOfTernAreCachedForTenSeconds(t *testing.T) {
	g, dirs := agentGateway(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }
	putAgents(t, filepath.Join(dirs.State, AgentsFile), model.AgentEntry{Block: "1", PaneBlock: "7"})
	live := []string{"1", "7"}
	asked := lsCounter(t, g, &live)
	refresh := model.RefreshAgents{PaneBlock: "7"}
	run(t, g, refresh)
	now = now.Add(9 * time.Second)
	run(t, g, refresh)
	if *asked != 1 {
		t.Fatalf("tern ls ran %d times within 10 s, want 1", *asked)
	}
	now = now.Add(2 * time.Second)
	run(t, g, refresh)
	if *asked != 2 {
		t.Fatalf("tern ls ran %d times after 10 s, want 2", *asked)
	}
}

func TestACachedBlockSetThatLacksALaterBlockIsNotTrustedToDropOrAdopt(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "1", PaneBlock: "7"})
	live := []string{"7"}
	asked := lsCounter(t, g, &live)
	run(t, g, model.RefreshAgents{PaneBlock: "7", Suspects: []string{"2"}})
	// Block 1 started after that answer; its starting Pane 7 is there, and a newer Pane 9 asks.
	live = []string{"1", "7", "9"}
	got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{Suspects: []string{"1"}, PaneBlock: "9"}))
	if len(got.Agents) != 1 || got.Agents[0].PaneBlock != "7" || *asked != 2 {
		t.Fatalf("refreshed %+v after %d asks", got.Agents, *asked)
	}
	if again := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{PaneBlock: "9"})); *asked != 2 || len(again.Agents) != 1 {
		t.Fatalf("the fresh answer was not cached: %d asks", *asked)
	}
}

func TestASuspectBlockStillInACachedAnswerIsConfirmedWithAFreshOne(t *testing.T) {
	g, dirs := agentGateway(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "1", PaneBlock: "7"}, model.AgentEntry{Block: "2", PaneBlock: "7"})
	live := []string{"1", "2", "7"}
	asked := lsCounter(t, g, &live)
	// Agent 1 closes: the answer is cached with agent 2 alive.
	live = []string{"2", "7"}
	if got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{Suspects: []string{"1"}, PaneBlock: "7"})); len(got.Agents) != 1 || got.Agents[0].Block != "2" {
		t.Fatalf("refreshed %+v", got.Agents)
	}
	// Agent 2 closes within 10 s; the cached answer still lists it.
	live = []string{"7"}
	now = now.Add(3 * time.Second)
	got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{Suspects: []string{"2"}, PaneBlock: "7"}))
	if got.Err != nil || len(got.Agents) != 0 || *asked != 2 {
		t.Fatalf("refreshed %+v after %d asks", got.Agents, *asked)
	}
	if stored, _ := ReadAgents(file); len(stored) != 0 {
		t.Errorf("entries after the drop: %v", stored)
	}
}

func TestAPaneAdoptsTheAgentsOfAPaneThatIsGoneAtStartAndOnRefresh(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file,
		model.AgentEntry{Block: "1", PaneBlock: "6"},
		model.AgentEntry{Block: "2", PaneBlock: "7"},
		model.AgentEntry{Block: "3", PaneBlock: ""})
	live := []string{"1", "2", "3", "7", "9"}
	lsCounter(t, g, &live)
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{PaneBlock: "9"}))
	if got.Err != nil || len(got.Agents) != 3 || got.Agents[0].PaneBlock != "9" || got.Agents[1].PaneBlock != "7" || got.Agents[2].PaneBlock != "9" {
		t.Fatalf("pruned %+v", got)
	}
	if stored, _ := ReadAgents(file); !slices.Equal(stored, got.Agents) {
		t.Errorf("stored %+v", stored)
	}
	// On refresh: Pane 7 goes away, and Pane 9 takes its agent.
	live = []string{"1", "2", "3", "9"}
	g.lsLive = nil
	refreshed := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{PaneBlock: "9"}))
	if refreshed.Err != nil || refreshed.Agents[1].PaneBlock != "9" {
		t.Fatalf("refreshed %+v", refreshed)
	}
}

func TestAPaneNeverAdoptsWhileTheStartingPaneIsAliveOrItHasNoBlock(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "2", PaneBlock: "7"}, model.AgentEntry{Block: "3", PaneBlock: "6"})
	live := []string{"2", "3", "7", "9"}
	lsCounter(t, g, &live)
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{PaneBlock: "9"}))
	if got.Agents[0].PaneBlock != "7" || got.Agents[1].PaneBlock != "9" {
		t.Fatalf("pruned %+v", got.Agents)
	}
	putAgents(t, file, model.AgentEntry{Block: "4", PaneBlock: "5"})
	live = append(live, "4")
	if blind := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{})); blind.Agents[2].PaneBlock != "5" {
		t.Fatalf("a Pane with no block adopted: %+v", blind.Agents)
	}
}

func TestAdoptingLeavesAnEntryAnotherPaneTookInTheMeantime(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "2", PaneBlock: "6"})
	g.runTern = func(context.Context, []string) ([]byte, error) {
		// Pane 8 adopts the entry while this Pane's tern ls is answering; Pane 8 is not in this answer.
		if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry { l[0].PaneBlock = "8"; return l }); err != nil {
			t.Fatal(err)
		}
		return lsOf("2", "9"), nil
	}
	got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{PaneBlock: "9"}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].PaneBlock != "8" {
		t.Fatalf("pruned %+v", got)
	}
}

func TestFocusingAnAgentRunsTernFocusAndReportsItsOutput(t *testing.T) {
	g, _ := agentGateway(t)
	var ran []string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) { ran = args; return nil, nil }
	if got := as[model.AgentFocused](t, run(t, g, model.FocusAgent{Block: "144115188075855871"})); got.Err != nil || !slices.Equal(ran, []string{"focus", "144115188075855871"}) {
		t.Fatalf("ran %q: %+v", ran, got)
	}
	g.runTern = func(context.Context, []string) ([]byte, error) {
		return []byte("tern: no such block\n"), errors.New("exit status 1")
	}
	if got := as[model.AgentFocused](t, run(t, g, model.FocusAgent{Block: "1"})); got.Err == nil || got.Err.Error() != "exit status 1: tern: no such block" {
		t.Errorf("error %v", got.Err)
	}
}

func TestTheBackgroundArgvBuilders(t *testing.T) {
	line := []string{"omp", "@/p.md"}
	for name, c := range map[string]struct{ got, want []string }{
		"new session": {AgentBackgroundSessionArgs("/work", line), []string{"new", "session", "kata agents", "--cwd", "/work", "--json", "--", "omp", "@/p.md"}},
		"new tab":     {AgentBackgroundTabArgs("/work", line), []string{"new", "tab", "kata agents", "--cwd", "/work", "--json", "--", "omp", "@/p.md"}},
		"rename":      {RenameAgentArgs("31", "demo#ab12"), []string{"rename", "31", "demo#ab12"}},
		"open":        {MoveRightOfArgs("31", "7"), []string{"move", "31", "right-of", "7", "--json"}},
		"open focus":  {MoveRightOfArgs("31", ""), []string{"move", "31", "right-of", "@focused", "--json"}},
		"new-tab":     {MoveNewTabArgs("31"), []string{"move", "31", "new-tab", "--json"}},
		"shell tab":   {AgentShellTabArgs(true), []string{"new", "tab", "kata agents", "--json"}},
		"shell sess":  {AgentShellTabArgs(false), []string{"new", "session", "kata agents", "--json"}},
		"close":       {CloseBlockArgs("40"), []string{"close", "40", "--json"}},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s: %q, want %q", name, c.got, c.want)
		}
	}
}

const lsWithAgents = `{"sessions":[{"id":1,"name":"main","shown":true,"tabs":[{"id":2,"blocks":[{"id":7}]}]},
{"id":3,"name":"kata agents","shown":false,"tabs":[{"id":4,"name":"x#1","blocks":[{"id":18494129176578},{"id":55}]}]}]}`

const lsWithoutAgents = `{"sessions":[{"id":1,"name":"main","shown":true,"tabs":[{"id":2,"blocks":[{"id":7}]}]}]}`

// ternScript answers each tern call from replies keyed by the first words of the command, and records the calls.
func ternScript(g *Gateway, replies map[string]func() ([]byte, error)) *[][]string {
	var calls [][]string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		calls = append(calls, args)
		for n := min(len(args), 3); n > 0; n-- {
			if reply, ok := replies[strings.Join(args[:n], " ")]; ok {
				return reply()
			}
		}
		return nil, fmt.Errorf("unscripted %q", args)
	}
	return &calls
}

func ternOut(out string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(out), nil }
}

func TestStartingABackgroundAgentUsesTheHiddenSessionAsItIs(t *testing.T) {
	cmd := model.StartAgent{Placement: model.AgentBackground, IssueUID: "U1", IssueRef: "demo#ab12", ProjectName: "demo", Workspace: "/bound", PaneBlock: "7"}
	prefix := func(calls [][]string, i int) string { return strings.Join(calls[i][:min(len(calls[i]), 3)], " ") }
	for name, c := range map[string]struct {
		ls      string
		lsAfter string // what `tern ls --json` lists from its second call on; ls when empty
		replies map[string]func() ([]byte, error)
		want    []string
	}{
		"session exists": {lsWithAgents, "", map[string]func() ([]byte, error){"new tab kata agents": ternOut(`{"session":3,"tab":9,"block":31}`)}, []string{"ls --json", "new tab kata agents", "rename 31 demo#ab12"}},
		"no session":     {lsWithoutAgents, "", map[string]func() ([]byte, error){"new session kata agents": ternOut(`{"session":3,"tab":9,"block":31}`)}, []string{"ls --json", "new session kata agents", "rename 31 demo#ab12"}},
		"a race for the session": {lsWithoutAgents, lsWithAgents, map[string]func() ([]byte, error){
			"new session kata agents": func() ([]byte, error) {
				return []byte("tern new: a session is already called `kata agents`\n"), errors.New("exit status 1")
			},
			"new tab kata agents": ternOut(`{"session":3,"tab":9,"block":31}`)}, []string{"ls --json", "new session kata agents", "ls --json", "new tab kata agents", "rename 31 demo#ab12"}},
	} {
		g, dirs := agentGateway(t)
		lsCalls := 0
		c.replies["ls"] = func() ([]byte, error) {
			lsCalls++
			if lsCalls > 1 && c.lsAfter != "" {
				return []byte(c.lsAfter), nil
			}
			return []byte(c.ls), nil
		}
		c.replies["rename"] = ternOut("")
		calls := ternScript(g, c.replies)
		got := as[model.AgentStarted](t, run(t, g, cmd))
		if got.Err != nil || got.Note != "" || !got.Entry.Background || got.Entry.Open || got.Entry.Block != "31" || got.Entry.PaneBlock != "7" {
			t.Fatalf("%s: %+v", name, got)
		}
		var seen []string
		for i := range *calls {
			p := prefix(*calls, i)
			if p == "rename 31 demo#ab12" || p == "ls --json" {
				p = strings.Join((*calls)[i], " ")
			}
			seen = append(seen, p)
		}
		if !slices.Equal(seen, c.want) {
			t.Errorf("%s: ran %q, want %q", name, seen, c.want)
		}
		if stored, _ := ReadAgents(filepath.Join(dirs.State, AgentsFile)); len(stored) != 1 || !stored[0].Background {
			t.Errorf("%s: agents.json %+v", name, stored)
		}
	}
}

func TestABackgroundAgentThatCannotStartLeavesNothing(t *testing.T) {
	g, dirs := agentGateway(t)
	ternScript(g, map[string]func() ([]byte, error){
		"ls":          ternOut(lsWithAgents),
		"new tab":     func() ([]byte, error) { return []byte("tern new: boom\n"), errors.New("exit status 1") },
		"new session": ternOut(""),
	})
	got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentBackground, IssueRef: "demo#ab12", ProjectName: "demo", Workspace: "/bound"}))
	if got.Err == nil || got.Err.Error() != "exit status 1: tern new: boom" {
		t.Fatalf("error %v", got.Err)
	}
	if left, _ := os.ReadDir(filepath.Join(dirs.State, AgentPromptsFolder)); len(left) != 0 {
		t.Errorf("prompt files left: %v", left)
	}
	// A tab that cannot be named leaves the agent started and says so.
	ternScript(g, map[string]func() ([]byte, error){
		"ls":      ternOut(lsWithAgents),
		"new tab": ternOut(`{"block":31}`),
		"rename":  func() ([]byte, error) { return []byte("no block\n"), errors.New("exit status 1") },
	})
	named := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentBackground, IssueRef: "demo#ab12", ProjectName: "demo", Workspace: "/bound"}))
	if named.Err != nil || named.Note != "cannot name its tab: exit status 1: no block" {
		t.Errorf("rename failure: %+v", named)
	}
}

func seedAgent(t *testing.T, dirs worker.Dirs, e model.AgentEntry) string {
	t.Helper()
	file := filepath.Join(dirs.State, AgentsFile)
	if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry { return append(l, e) }); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestAFailedNewSessionFallsBackToATabWhenTheSessionExistsNow(t *testing.T) {
	g, _ := agentGateway(t)
	lsCalls := 0
	calls := ternScript(g, map[string]func() ([]byte, error){
		"ls": func() ([]byte, error) {
			lsCalls++
			if lsCalls == 1 {
				return []byte(lsWithoutAgents), nil
			}
			return []byte(lsWithAgents), nil
		},
		"new session kata agents": func() ([]byte, error) { return []byte("tern: cannot create it\n"), errors.New("exit status 1") },
		"new tab kata agents":     ternOut(`{"block":31}`),
		"rename":                  ternOut(""),
	})
	got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentBackground, IssueRef: "demo#ab12", ProjectName: "demo", Workspace: "/bound"}))
	if got.Err != nil || got.Entry.Block != "31" || lsCalls != 2 {
		t.Fatalf("%+v after %d ls calls, calls %q", got, lsCalls, *calls)
	}
}

func TestOpeningABackgroundAgentMovesItBesideThePaneAndMarksItOpen(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Background: true})
	calls := ternScript(g, map[string]func() ([]byte, error){"move": ternOut(`{"block":31}`)})
	got := as[model.AgentPlaced](t, run(t, g, model.OpenAgent{Block: "31", PaneBlock: "7"}))
	if got.Err != nil || !got.Moved || !got.Open || got.Back || !reflect.DeepEqual(*calls, [][]string{{"move", "31", "right-of", "7", "--json"}}) {
		t.Fatalf("%+v, ran %q", got, *calls)
	}
	if stored, _ := ReadAgents(file); !stored[0].Open {
		t.Errorf("agents.json %+v", stored)
	}
	// A refusal moves nothing and writes nothing.
	seedAgent(t, dirs, model.AgentEntry{Block: "32", Background: true})
	ternScript(g, map[string]func() ([]byte, error){"move": func() ([]byte, error) { return []byte("no block\n"), errors.New("exit status 1") }})
	refused := as[model.AgentPlaced](t, run(t, g, model.OpenAgent{Block: "32", PaneBlock: "7"}))
	if refused.Err == nil || refused.Err.Error() != "exit status 1: no block" || refused.Moved {
		t.Fatalf("refusal %+v", refused)
	}
	if stored, _ := ReadAgents(file); stored[1].Open {
		t.Errorf("agents.json after a refusal %+v", stored)
	}
}

func TestGoingBackMovesTheAgentIntoTheHiddenSessionAndNamesItsTab(t *testing.T) {
	back := model.SendAgentBack{Block: "31", IssueRef: "demo#ab12"}
	for name, c := range map[string]struct {
		ls      string
		replies map[string]func() ([]byte, error)
		want    [][]string
	}{
		"a block is left": {lsWithAgents, map[string]func() ([]byte, error){"move": ternOut(`{"block":31}`)}, [][]string{
			{"ls", "--json"}, {"move", "31", "right-of", "18494129176578", "--json"}, {"move", "31", "new-tab", "--json"}, {"rename", "31", "demo#ab12"}}},
		"the session has no block": {`{"sessions":[{"id":3,"name":"kata agents","tabs":[]}]}`, map[string]func() ([]byte, error){
			"new tab": ternOut(`{"session":3,"tab":9,"block":40}`), "move": ternOut(`{"block":31}`), "close": ternOut(`{"block":40}`)}, [][]string{
			{"ls", "--json"}, {"new", "tab", "kata agents", "--json"}, {"move", "31", "right-of", "40", "--json"}, {"close", "40", "--json"}, {"rename", "31", "demo#ab12"}}},
		"the session is gone": {lsWithoutAgents, map[string]func() ([]byte, error){
			"new session": ternOut(`{"session":3,"tab":9,"block":40}`), "move": ternOut(`{"block":31}`), "close": ternOut(`{"block":40}`)}, [][]string{
			{"ls", "--json"}, {"new", "session", "kata agents", "--json"}, {"move", "31", "right-of", "40", "--json"}, {"close", "40", "--json"}, {"rename", "31", "demo#ab12"}}},
	} {
		g, dirs := agentGateway(t)
		file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Open: true, Placement: model.AgentSplit})
		c.replies["ls"], c.replies["rename"] = ternOut(c.ls), ternOut("")
		calls := ternScript(g, c.replies)
		got := as[model.AgentPlaced](t, run(t, g, back))
		if got.Err != nil || !got.Moved || got.Open || !got.Back || !reflect.DeepEqual(*calls, c.want) {
			t.Errorf("%s: %+v, ran %q, want %q", name, got, *calls, c.want)
		}
		if stored, _ := ReadAgents(file); stored[0].Open || !stored[0].Background {
			t.Errorf("%s: agents.json %+v", name, stored)
		}
	}
}

func TestAFailedStepGoingBackShowsTernsWordsAndKeepsTheAgentOpen(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Background: true, Open: true})
	calls := ternScript(g, map[string]func() ([]byte, error){
		"ls":   ternOut(lsWithAgents),
		"move": func() ([]byte, error) { return []byte("tern move: no block\n"), errors.New("exit status 1") },
	})
	got := as[model.AgentPlaced](t, run(t, g, model.SendAgentBack{Block: "31", IssueRef: "demo#ab12"}))
	if got.Err == nil || got.Err.Error() != "exit status 1: tern move: no block" || got.Moved || len(*calls) != 2 {
		t.Fatalf("%+v, ran %q", got, *calls)
	}
	if stored, _ := ReadAgents(file); !stored[0].Open {
		t.Errorf("agents.json %+v", stored)
	}
	// A tab that cannot be named: the agent is back, the entry closed, the failure shown.
	ternScript(g, map[string]func() ([]byte, error){"ls": ternOut(lsWithAgents), "move": ternOut(`{"block":31}`),
		"rename": func() ([]byte, error) { return []byte("no block\n"), errors.New("exit status 1") }})
	named := as[model.AgentPlaced](t, run(t, g, model.SendAgentBack{Block: "31", IssueRef: "demo#ab12"}))
	if named.Err == nil || !named.Moved || named.Open {
		t.Fatalf("rename failure %+v", named)
	}
	if stored, _ := ReadAgents(file); stored[0].Open {
		t.Errorf("agents.json %+v", stored)
	}
}

func TestPruningKeepsABlockOfTheHiddenSession(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "18494129176578", Background: true})
	ternScript(g, map[string]func() ([]byte, error){"ls": ternOut(lsWithAgents)})
	if got := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{})); got.Err != nil || len(got.Agents) != 1 {
		t.Fatalf("pruned %+v", got)
	}
	if stored, _ := ReadAgents(file); len(stored) != 1 {
		t.Errorf("agents.json %+v", stored)
	}
}

func TestClosingAFinishedAgentClosesItsBlockAndDropsItsEntryAndPrompt(t *testing.T) {
	g, dirs := agentGateway(t)
	prompt := writeAgentFile(t, filepath.Join(t.TempDir(), "p.md"), "go")
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Background: true, PromptFile: prompt})
	seedAgent(t, dirs, model.AgentEntry{Block: "32", Background: true})
	calls := ternScript(g, map[string]func() ([]byte, error){"close": ternOut(`{"block":31}`)})
	got := as[model.AgentClosed](t, run(t, g, model.CloseAgent{Block: "31"}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "32" || !reflect.DeepEqual(*calls, [][]string{{"close", "31", "--json"}}) {
		t.Fatalf("%+v, ran %q", got, *calls)
	}
	if stored, _ := ReadAgents(file); len(stored) != 1 || stored[0].Block != "32" {
		t.Errorf("agents.json %+v", stored)
	}
	if _, err := os.Stat(prompt); !os.IsNotExist(err) {
		t.Errorf("prompt file stays: %v", err)
	}
}

func TestAFailedCloseKeepsTheEntryAndThePrompt(t *testing.T) {
	g, dirs := agentGateway(t)
	prompt := writeAgentFile(t, filepath.Join(t.TempDir(), "p.md"), "go")
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Background: true, PromptFile: prompt})
	ternScript(g, map[string]func() ([]byte, error){"close": func() ([]byte, error) { return []byte("no block\n"), errors.New("exit status 1") }})
	got := as[model.AgentClosed](t, run(t, g, model.CloseAgent{Block: "31"}))
	if got.Err == nil || got.Err.Error() != "exit status 1: no block" {
		t.Fatalf("%+v", got)
	}
	if stored, _ := ReadAgents(file); len(stored) != 1 {
		t.Errorf("agents.json %+v", stored)
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Errorf("prompt file: %v", err)
	}
}

func TestRecordingWhereTheRelaySawAnAgentChangesOnlyItsOpenFlag(t *testing.T) {
	g, dirs := agentGateway(t)
	seedAgent(t, dirs, model.AgentEntry{Block: "31", IssueRef: "demo#ab12", Background: true})
	file := seedAgent(t, dirs, model.AgentEntry{Block: "32", Background: true, Open: true})
	got := as[model.AgentOpenSet](t, run(t, g, model.SetAgentOpen{Block: "31", Open: true}))
	if got.Err != nil || got.Block != "31" || !got.Open {
		t.Fatalf("%+v", got)
	}
	got = as[model.AgentOpenSet](t, run(t, g, model.SetAgentOpen{Block: "32", Open: false}))
	if got.Err != nil || got.Open {
		t.Fatalf("%+v", got)
	}
	stored, _ := ReadAgents(file)
	if !stored[0].Open || !stored[0].Background || stored[0].IssueRef != "demo#ab12" || stored[1].Open || !stored[1].Background {
		t.Errorf("agents.json %+v", stored)
	}
}

func TestClosingSkipsAnAgentWhoseEntryReadsOpenNow(t *testing.T) {
	g, dirs := agentGateway(t)
	prompt := writeAgentFile(t, filepath.Join(t.TempDir(), "p.md"), "go")
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Background: true, Open: true, PromptFile: prompt})
	calls := ternScript(g, map[string]func() ([]byte, error){"close": ternOut(`{"block":31}`)})
	got := as[model.AgentClosed](t, run(t, g, model.CloseAgent{Block: "31"}))
	if got.Err != nil || !got.Kept || len(got.Agents) != 1 || !got.Agents[0].Open || len(*calls) != 0 {
		t.Fatalf("%+v, ran %q", got, *calls)
	}
	if stored, _ := ReadAgents(file); len(stored) != 1 {
		t.Errorf("agents.json %+v", stored)
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Errorf("prompt file: %v", err)
	}
}

func TestClosingKeepsASplitAgentAndClosesOneAnotherPaneSentToTheBackground(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31"})
	calls := ternScript(g, map[string]func() ([]byte, error){"close": ternOut(`{"block":31}`)})
	got := as[model.AgentClosed](t, run(t, g, model.CloseAgent{Block: "31"}))
	if got.Err != nil || !got.Kept || len(got.Agents) != 1 || len(*calls) != 0 {
		t.Fatalf("split agent: %+v, ran %q", got, *calls)
	}
	if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry { l[0].Background = true; return l }); err != nil {
		t.Fatal(err)
	}
	got = as[model.AgentClosed](t, run(t, g, model.CloseAgent{Block: "31"}))
	if got.Err != nil || got.Kept || len(got.Agents) != 0 || len(*calls) != 1 {
		t.Fatalf("background agent: %+v, ran %q", got, *calls)
	}
}

func TestClosingAnAgentWhenAgentsJSONCannotBeReadRunsNothing(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Background: true})
	if err := os.WriteFile(file, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := ternScript(g, map[string]func() ([]byte, error){"close": ternOut(`{"block":31}`)})
	got := as[model.AgentClosed](t, run(t, g, model.CloseAgent{Block: "31"}))
	if got.Err == nil || got.Kept || len(*calls) != 0 {
		t.Fatalf("%+v, ran %q", got, *calls)
	}
}

func TestRecordingAnAgentInTheHiddenSessionCanMarkItBackground(t *testing.T) {
	g, dirs := agentGateway(t)
	file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Open: true})
	got := as[model.AgentOpenSet](t, run(t, g, model.SetAgentOpen{Block: "31", Background: true}))
	if got.Err != nil || !got.Background {
		t.Fatalf("%+v", got)
	}
	if stored, _ := ReadAgents(file); !stored[0].Background || stored[0].Open {
		t.Errorf("agents.json %+v", stored)
	}
}

func TestAChangeRunsAgainWhenAnotherPaneChangedTheFileUnderIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.json")
	writeAgents(t, path, model.AgentEntry{Block: "1"})
	var calls int
	other := model.AgentEntry{Block: "2", Open: true}
	got, err := UpdateAgents(path, func(l []model.AgentEntry) []model.AgentEntry {
		calls++
		if calls == 1 {
			// The Pane starting an agent reads; another Pane flags an open agent and appends its own before the write.
			writeAgents(t, path, other)
		}
		return append(l, model.AgentEntry{Block: "3"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("change ran %d times, want 2", calls)
	}
	var blocks []string
	for _, e := range got {
		blocks = append(blocks, e.Block)
	}
	if !slices.Equal(blocks, []string{"1", "2", "3"}) {
		t.Errorf("blocks %v: a change was lost", blocks)
	}
	if stored, _ := ReadAgents(path); !slices.Equal(stored, got) {
		t.Errorf("stored %+v, want %+v", stored, got)
	}
}

func TestAChangeThatKeepsFindingTheFileChangedReportsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.json")
	writeAgents(t, path, model.AgentEntry{Block: "1"})
	var calls int
	_, err := UpdateAgents(path, func(l []model.AgentEntry) []model.AgentEntry {
		calls++
		writeAgents(t, path, model.AgentEntry{Block: "other"})
		return nil
	})
	if !errors.Is(err, guardedfile.ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
	if calls < 2 {
		t.Errorf("change ran %d times", calls)
	}
	stored, _ := ReadAgents(path)
	if len(stored) != calls+1 {
		t.Errorf("stored %+v: the other writer's entries should stand", stored)
	}
}

func writeAgents(t *testing.T, path string, entries ...model.AgentEntry) {
	t.Helper()
	list, err := ReadAgents(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(agentsDoc{Agents: append(list, entries...)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
