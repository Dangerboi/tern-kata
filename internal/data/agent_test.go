package data

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	if failed := as[model.AgentsPruned](t, run(t, g, model.PruneAgents{})); failed.Err == nil {
		t.Error("a failed tern ls pruned without error")
	}
	if stored, _ := ReadAgents(file); len(stored) != 1 {
		t.Errorf("entries after a failed ls: %v", stored)
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

func TestRefreshingDropsTheGoneBlocksAndReadsTheFileAsItIs(t *testing.T) {
	g, dirs := agentGateway(t)
	prompts := filepath.Join(dirs.State, AgentPromptsFolder)
	if err := os.MkdirAll(prompts, 0o700); err != nil {
		t.Fatal(err)
	}
	kept := writeAgentFile(t, filepath.Join(prompts, "kept.md"), "x")
	dead := writeAgentFile(t, filepath.Join(prompts, "dead.md"), "x")
	file := filepath.Join(dirs.State, AgentsFile)
	if _, err := UpdateAgents(file, func(l []model.AgentEntry) []model.AgentEntry {
		return append(l, model.AgentEntry{Block: "1", PromptFile: kept}, model.AgentEntry{Block: "144115188075855871", PromptFile: dead})
	}); err != nil {
		t.Fatal(err)
	}
	g.runTern = func(context.Context, []string) ([]byte, error) { t.Error("refreshing ran tern"); return nil, nil }
	if got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{})); got.Err != nil || len(got.Agents) != 2 {
		t.Fatalf("a plain re-read: %+v", got)
	}
	got := as[model.AgentsRefreshed](t, run(t, g, model.RefreshAgents{Gone: []string{"144115188075855871", "42"}}))
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].Block != "1" {
		t.Fatalf("refreshed %+v", got)
	}
	if _, err := os.Stat(dead); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the gone entry's prompt stayed: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("a kept entry's prompt went: %v", err)
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
