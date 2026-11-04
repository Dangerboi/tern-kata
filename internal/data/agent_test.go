package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	line := AgentCommandLine("", "demo#ab12", AgentInbox{}, AgentRoute{}, []string{"omp", "--fast"}, "/state/p.md")
	if want := []string{"tern-kata", "agent", "--ref", "demo#ab12", "--", "omp", "--fast", "@/state/p.md"}; !slices.Equal(line, want) {
		t.Errorf("without an actor %q, want %q", line, want)
	}
	wrapped := AgentCommandLine("me-agent", "demo#ab12", AgentInbox{}, AgentRoute{}, []string{"omp"}, "/state/p.md")
	if want := []string{"tern-kata", "agent", "--as", "me-agent", "--ref", "demo#ab12", "--", "omp", "@/state/p.md"}; !slices.Equal(wrapped, want) {
		t.Errorf("with an actor %q, want %q", wrapped, want)
	}
	// An agent with an inbox: the wrapper gets its address up to the block id, omp loads the extension before the prompt.
	inbox := AgentCommandLine("me-agent", "demo#ab12", AgentInbox{Prefix: "dana/omp", Extension: "/state/omp/tern-kata.ts"}, AgentRoute{}, []string{"omp", "--fast"}, "/state/p.md")
	if want := []string{"tern-kata", "agent", "--as", "me-agent", "--inbox", "dana/omp", "--ref", "demo#ab12", "--", "omp", "--fast", "-e", "/state/omp/tern-kata.ts", "@/state/p.md"}; !slices.Equal(inbox, want) {
		t.Errorf("with an inbox %q, want %q", inbox, want)
	}
	split := AgentSplitArgs("24489903521795", "/work/my repo", line)
	if want := []string{"split", "24489903521795", "right", "--cwd", "/work/my repo", "--json", "--", "tern-kata", "agent", "--ref", "demo#ab12", "--", "omp", "--fast", "@/state/p.md"}; !slices.Equal(split, want) {
		t.Errorf("split %q, want %q", split, want)
	}
	if got := AgentSplitArgs("", "/w", line); got[1] != "@focused" {
		t.Errorf("a Pane that cannot name its block splits %q", got[1])
	}
	tab := AgentTabArgs("/work", wrapped)
	if want := []string{"new", "tab", "--cwd", "/work", "--json", "--", "tern-kata", "agent", "--as", "me-agent", "--ref", "demo#ab12", "--", "omp", "@/state/p.md"}; !slices.Equal(tab, want) {
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
	for _, name := range []string{"KATA_HOME", "KATA_SERVER", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE"} {
		t.Setenv(name, "") // a Pane on the default daemon: the command line is the one agents always had
	}
	g, dirs := agentGateway(t)
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "actor = \"me-agent\"\n[projects.demo]\ncheckout = \"/work/demo\"\n")
	var ran []string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		ran = args
		return []byte(`{"session": 1, "tab": 2, "block": 31}`), nil
	}
	got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentSplit, IssueUID: "U1", IssueRef: "demo#ab12", IssueTitle: "Fix it",
		ProjectName: "demo", PaneBlock: "7", Actor: "dana"}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	e := got.Entry
	if e.Block != "31" || e.IssueUID != "U1" || e.IssueRef != "demo#ab12" || e.PaneBlock != "7" || e.Teammate != "omp" || e.Background || e.Open ||
		e.Inbox != "dana/omp-31" || e.NoInbox != "" ||
		!e.StartedAt.Equal(time.Date(2026, 10, 29, 12, 0, 7, 0, time.UTC)) || filepath.Dir(e.PromptFile) != filepath.Join(dirs.State, AgentPromptsFolder) {
		t.Fatalf("entry %+v", e)
	}
	extension := filepath.Join(dirs.State, OmpExtensionFolder, "tern-kata.ts")
	want := []string{"split", "7", "right", "--cwd", "/work/demo", "--json", "--", "tern-kata", "agent", "--as", "me-agent", "--inbox", "dana/omp", "--ref", "demo#ab12", "--", "omp", "-e", extension, "@" + e.PromptFile}
	if !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	if text, err := os.ReadFile(extension); err != nil || !bytes.Equal(text, ompExtension) {
		t.Fatalf("the extension file holds %d bytes, %v; want the embedded %d", len(text), err, len(ompExtension))
	}
	text, err := os.ReadFile(e.PromptFile)
	if err != nil || string(text) != AgentPrompt("demo#ab12", "Fix it", "dana") {
		t.Fatalf("prompt %q, %v", text, err)
	}
	if stored, err := ReadAgents(filepath.Join(dirs.State, AgentsFile)); err != nil || len(stored) != 1 || stored[0] != e {
		t.Fatalf("agents.json %+v, %v", stored, err)
	}
	// W starts a tab, in the Binding's workspace, and without an actor the command still runs through `tern-kata agent` with the ref.
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentTab, IssueRef: "demo#cd34", ProjectName: "demo", Workspace: "/bound"}))
	if len(ran) < 12 || !slices.Equal(ran[:11], []string{"new", "tab", "--cwd", "/bound", "--json", "--", "tern-kata", "agent", "--ref", "demo#cd34", "--"}) || ran[11] != "omp" {
		t.Fatalf("W ran %q", ran)
	}
}

func TestAnAgentThatCannotLoadTheExtensionStartsWithoutAnInboxAndSaysWhy(t *testing.T) {
	for _, name := range []string{"KATA_HOME", "KATA_SERVER", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE"} {
		t.Setenv(name, "")
	}
	g, dirs := agentGateway(t)
	var ran []string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		ran = args
		return []byte(`{"session": 1, "tab": 2, "block": 31}`), nil
	}
	for _, c := range []struct {
		command string
		want    model.AgentNoInbox
	}{
		{`["claude", "--continue"]`, model.AgentNotOmp},
		{`["/opt/omp/omp", "--trusted-extension", "/abs/guard.ts"]`, model.AgentTrustedExtension},
		{`["omp", "--trusted-extension=/abs/guard.ts"]`, model.AgentTrustedExtension},
	} {
		writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "command = "+c.command+"\n[projects.demo]\ncheckout = \"/work/demo\"\n")
		got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentSplit, IssueUID: "U1", IssueRef: "demo#ab12", ProjectName: "demo", PaneBlock: "7", Actor: "dana"}))
		if got.Err != nil || got.Entry.Inbox != "" || got.Entry.NoInbox != c.want {
			t.Errorf("%s: entry %+v, err %v; want no inbox because %s", c.command, got.Entry, got.Err, c.want)
		}
		if slices.Contains(ran, "--inbox") || slices.Contains(ran, "-e") {
			t.Errorf("%s: ran %q, want no inbox and no extension", c.command, ran)
		}
		text, err := os.ReadFile(got.Entry.PromptFile)
		if err != nil || string(text) != AgentPrompt("demo#ab12", "", "") {
			t.Errorf("%s: prompt %q, %v; want the prompt without messages", c.command, text, err)
		}
	}
}

func TestTheOmpExtensionIsWrittenFromTheBinaryAndReplacedWhenItChanges(t *testing.T) {
	state := t.TempDir()
	path, err := writeOmpExtension(state)
	if err != nil || path != filepath.Join(state, OmpExtensionFolder, "tern-kata.ts") {
		t.Fatalf("path %q, %v", path, err)
	}
	// An older tern-kata's extension is replaced by this one's.
	if err := os.WriteFile(path, []byte("export default function () {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := writeOmpExtension(state); err != nil || again != path {
		t.Fatalf("again %q, %v", again, err)
	}
	if text, err := os.ReadFile(path); err != nil || !bytes.Equal(text, ompExtension) {
		t.Fatalf("after a second write the file holds %q, %v", text, err)
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
	if without := AgentPrompt("demo#ab12", "Fix it", ""); strings.Contains(without, "message") {
		t.Errorf("an agent no message reaches is told about messages:\n%s", without)
	}
	got := AgentPrompt("demo#ab12", "  Fix the\n# crash\tnow ", "dana")
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

func TestAFailedLaterStepGoingBackRecordsWhereTheBlockIs(t *testing.T) {
	fail := func() ([]byte, error) { return []byte("tern: refused\n"), errors.New("exit status 1") }
	for name, c := range map[string]struct {
		ls      string
		replies map[string]func() ([]byte, error)
	}{
		"the move into its own tab fails": {lsWithAgents, map[string]func() ([]byte, error){
			"move 31 right-of": ternOut(`{"block":31}`), "move 31 new-tab": fail}},
		"the shell close fails": {lsWithoutAgents, map[string]func() ([]byte, error){
			"new session": ternOut(`{"session":3,"tab":9,"block":40}`), "move 31 right-of": ternOut(`{"block":31}`), "close": fail}},
	} {
		g, dirs := agentGateway(t)
		file := seedAgent(t, dirs, model.AgentEntry{Block: "31", Open: true, Placement: model.AgentSplit})
		c.replies["ls"] = ternOut(c.ls)
		ternScript(g, c.replies)
		got := as[model.AgentPlaced](t, run(t, g, model.SendAgentBack{Block: "31", IssueRef: "demo#ab12"}))
		if got.Err == nil || got.Err.Error() != "exit status 1: tern: refused" || !got.Moved || got.Open || !got.Back {
			t.Errorf("%s: %+v", name, got)
		}
		if stored, _ := ReadAgents(file); stored[0].Open || !stored[0].Background {
			t.Errorf("%s: agents.json %+v", name, stored)
		}
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

// watchClock makes g's clock tick 11 s on every read, past what a listing is trusted for, so no caller finds the cache fresh,
// and sends on the returned channel at each read. liveBlocks reads the clock while it holds its lock, so a caller that has sent
// is about to join or start the running ask before anything else can finish it. The channel holds every read a test makes.
func watchClock(g *Gateway) <-chan struct{} {
	reads := make(chan struct{}, 64)
	var n atomic.Int64
	base := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	g.lsLive = map[string]bool{"stale": true}
	g.now = func() time.Time {
		reads <- struct{}{}
		return base.Add(time.Duration(n.Add(1)) * 11 * time.Second)
	}
	return reads
}

// waitReads waits for n more clock reads. Failing takes its time, so it only counts when a caller never got that far.
func waitReads(t *testing.T, reads <-chan struct{}, n int) {
	t.Helper()
	for range n {
		select {
		case <-reads:
		case <-time.After(10 * time.Second):
			t.Fatal("a caller never reached the listing")
		}
	}
}

func TestLiveBlocksCallerReturnsAtItsOwnDeadlineWhileAnotherAskRuns(t *testing.T) {
	g, _ := agentGateway(t)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	g.runTern = func(ctx context.Context, _ []string) ([]byte, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return lsOf("1"), nil
	}
	slow := make(chan error, 1)
	go func() { _, _, err := g.liveBlocks(context.Background(), false); slow <- err }()
	<-started
	past, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	short := make(chan error, 1)
	go func() { _, _, err := g.liveBlocks(past, false); short <- err }()
	select {
	case err := <-short:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("the short caller got %v, want its deadline error", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("the short caller never returned at its own deadline")
	}
	close(release)
	if err := <-slow; err != nil {
		t.Errorf("the slow caller: %v", err)
	}
}

func TestLiveBlocksConcurrentCallersShareOneAsk(t *testing.T) {
	g, _ := agentGateway(t)
	reads := watchClock(g)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var asked atomic.Int32
	g.runTern = func(context.Context, []string) ([]byte, error) {
		asked.Add(1)
		<-release
		return lsOf("1"), nil
	}
	const callers = 5
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if live, _, err := g.liveBlocks(context.Background(), false); err != nil || !live["1"] {
				t.Errorf("got %v, %v", live, err)
			}
		}()
	}
	// Each caller reads the clock once to look at the cache; the one that starts the ask reads it once more. All of them
	// get there while the ask is still held, or one of them is waiting on a lock the running ask holds.
	waitReads(t, reads, callers+1)
	once.Do(func() { close(release) })
	wg.Wait()
	if n := asked.Load(); n != 1 {
		t.Errorf("tern ls ran %d times, want 1", n)
	}
}

func TestACallerJoiningARunningAskIsToldItsAnswerMayBeOlderThanItsCall(t *testing.T) {
	g, _ := agentGateway(t)
	reads := watchClock(g)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	g.runTern = func(context.Context, []string) ([]byte, error) {
		started <- struct{}{}
		<-release
		return lsOf("1"), nil
	}
	type answer struct {
		cached bool
		err    error
	}
	first, joiner := make(chan answer, 1), make(chan answer, 1)
	go func() { _, cached, err := g.liveBlocks(context.Background(), false); first <- answer{cached, err} }()
	<-started
	waitReads(t, reads, 2)
	go func() { _, cached, err := g.liveBlocks(context.Background(), false); joiner <- answer{cached, err} }()
	waitReads(t, reads, 1)
	close(release)
	if got := <-first; got.err != nil || got.cached {
		t.Errorf("the caller that started the ask: %+v, want its own fresh answer", got)
	}
	if got := <-joiner; got.err != nil || !got.cached {
		t.Errorf("the caller that joined the ask: %+v, want an answer that may be older than its call", got)
	}
}

func TestAnAskThatBeganBeforeAnotherAndEndsAfterItDoesNotReplaceItsListing(t *testing.T) {
	g, _ := agentGateway(t)
	watchClock(g)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var asks atomic.Int32
	g.runTern = func(context.Context, []string) ([]byte, error) {
		if asks.Add(1) == 1 {
			started <- struct{}{}
			<-release
			return lsOf("1"), nil
		}
		return lsOf("2"), nil
	}
	older := make(chan struct{})
	go func() { _, _, _ = g.liveBlocks(context.Background(), false); close(older) }()
	<-started
	if live, _, err := g.liveBlocks(context.Background(), true); err != nil || !live["2"] {
		t.Fatalf("the fresh ask: %v, %v", live, err)
	}
	close(release)
	<-older
	g.lsMu.Lock()
	defer g.lsMu.Unlock()
	if !g.lsLive["2"] || g.lsLive["1"] {
		t.Errorf("the cached listing is %v, want the one from the ask that began later", g.lsLive)
	}
}

func TestRefreshingDoesNotAdoptAnAgentFromAListingThatBeganBeforeItsPaneOpened(t *testing.T) {
	g, dirs := agentGateway(t)
	file := filepath.Join(dirs.State, AgentsFile)
	putAgents(t, file, model.AgentEntry{Block: "1", PaneBlock: "7"})
	reads := watchClock(g)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var asks atomic.Int32
	g.runTern = func(context.Context, []string) ([]byte, error) {
		if asks.Add(1) == 1 {
			started <- struct{}{}
			<-release
			return lsOf("1", "7", "9"), nil // listed before Pane 8 opened
		}
		return lsOf("1", "2", "7", "8", "9"), nil
	}
	first := make(chan struct{})
	go func() { run(t, g, model.RefreshAgents{PaneBlock: "9"}); close(first) }()
	<-started
	waitReads(t, reads, 2)
	// Pane 8 opens and starts an agent while that listing is still being taken.
	putAgents(t, file, model.AgentEntry{Block: "2", PaneBlock: "8"})
	second := make(chan struct{})
	go func() { run(t, g, model.RefreshAgents{PaneBlock: "9"}); close(second) }()
	waitReads(t, reads, 1)
	close(release)
	<-first
	<-second
	stored, err := ReadAgents(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range stored {
		if e.Block == "2" && e.PaneBlock != "8" {
			t.Fatalf("Pane 8's agent was adopted by Pane 9: %+v", stored)
		}
	}
}

func TestAgentTomlNameIsTheTeammateAndDefaultsToTheProgram(t *testing.T) {
	unset, err := ReadAgentConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := unset.For("demo").Name; got != "omp" {
		t.Errorf("unset name %q, want the default program's", got)
	}
	path := writeAgentFile(t, filepath.Join(t.TempDir(), "agent.toml"), `
name = "builder"
[projects.demo]
name = "demo-builder"
[projects.codex]
command = ["/usr/bin/codex.exe", "--fast"]
[projects.odd]
command = ["/opt/my agent"]
`)
	cfg, err := ReadAgentConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	for project, want := range map[string]string{"other": "builder", "demo": "demo-builder", "codex": "builder"} {
		if got := cfg.For(project).Name; got != want {
			t.Errorf("name for %s %q, want %q", project, got, want)
		}
	}
	bare, err := ReadAgentConfig(writeAgentFile(t, filepath.Join(t.TempDir(), "agent.toml"), "[projects.codex]\ncommand = [\"/usr/bin/codex.exe\"]\n[projects.odd]\ncommand = [\"/opt/my agent\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := bare.For("codex").Name; got != "codex" {
		t.Errorf("name from the command %q, want codex", got)
	}
	if got := bare.For("odd").Name; got != "" {
		t.Errorf("name from an unusable program %q, want none", got)
	}
	for _, text := range []string{"name = \"two words\"\n", "[projects.demo]\nname = \"\"\n"} {
		if _, err := ReadAgentConfig(writeAgentFile(t, filepath.Join(t.TempDir(), "agent.toml"), text)); err == nil || !strings.Contains(err.Error(), "teammate name") {
			t.Errorf("%q: error %v, want one saying the name is no teammate name", text, err)
		}
	}
}

// routeEnv sets the Pane's environment for a route test: the kata variables not named in env are empty.
func routeEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, name := range []string{"KATA_HOME", "KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE", "KATA_TRUST_PRIVATE_NETWORK", "KATA_DB", "KATA_DSN", "KATA_POSTGRES_SCHEMA", "WORK_TOKEN", "LOCAL_TOKEN"} {
		t.Setenv(name, env[name])
	}
	t.Setenv("HOME", t.TempDir()) // the default kata home is <HOME>/.kata
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
}

func startRoutedAgent(t *testing.T, g *Gateway, dirs worker.Dirs, ternErr error) (ran []string, got model.AgentStarted) {
	t.Helper()
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		ran = args
		return []byte(`{"session": 1, "tab": 2, "block": 31}`), ternErr
	}
	return ran, as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentTab, IssueRef: "demo#cd34", ProjectName: "demo", Workspace: "/bound"}))
}

func TestAgentStartedFromAPaneOnAnotherDaemonCarriesThePanesRoute(t *testing.T) {
	home := catalogHome(t, "[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\ntoken_env = \"WORK_TOKEN\"\n[[daemon]]\nname = \"mine\"\nlocal = true\n[[daemon]]\nname = \"owned\"\nlocal = true\ntoken_env = \"LOCAL_TOKEN\"\n")
	for name, tc := range map[string]struct {
		env       map[string]string
		current   string
		want      []string // the route flags, with TOKEN standing for the token file the Pane writes
		wantToken string
	}{
		"KATA_HOME":                                  {env: map[string]string{"KATA_HOME": "/tmp/h"}, want: []string{"--home", "/tmp/h"}},
		"KATA_SERVER":                                {env: map[string]string{"KATA_SERVER": "https://kata.example", "KATA_ALLOW_INSECURE": "true"}, want: []string{"--home", "DEFAULT", "--server", "https://kata.example", "--allow-insecure"}},
		"KATA_SERVER token":                          {env: map[string]string{"KATA_SERVER": "https://kata.example", "KATA_AUTH_TOKEN": "pane-token"}, want: []string{"--home", "DEFAULT", "--server", "https://kata.example", "--token-file", "TOKEN"}, wantToken: "pane-token"},
		"KATA_SERVER file":                           {env: map[string]string{"KATA_SERVER": "https://kata.example", "KATA_AUTH_TOKEN_FILE": "/run/t"}, want: []string{"--home", "DEFAULT", "--server", "https://kata.example", "--auth-file", "/run/t"}},
		"private network":                            {env: map[string]string{"KATA_SERVER": "http://100.64.0.5:7777", "KATA_AUTH_TOKEN": "pane-token", "KATA_TRUST_PRIVATE_NETWORK": "1"}, want: []string{"--home", "DEFAULT", "--server", "http://100.64.0.5:7777", "--token-file", "TOKEN", "--trust-private-network"}, wantToken: "pane-token"},
		"trust alone is no route":                    {env: map[string]string{"KATA_TRUST_PRIVATE_NETWORK": "1", "KATA_AUTH_TOKEN": "pane-token"}},
		"insecure is only 1/true":                    {env: map[string]string{"KATA_SERVER": "https://kata.example", "KATA_ALLOW_INSECURE": "yes"}, want: []string{"--home", "DEFAULT", "--server", "https://kata.example"}},
		"a switch, token_env":                        {env: map[string]string{"KATA_HOME": home, "WORK_TOKEN": "work-token", "KATA_SERVER": "https://ignored.example"}, current: "work", want: []string{"--home", home, "--daemon", "work", "--token-file", "TOKEN"}, wantToken: "work-token"},
		"a local entry's own token beats the Pane's": {env: map[string]string{"KATA_HOME": home, "KATA_AUTH_TOKEN": "pane-token", "LOCAL_TOKEN": "entry-token"}, current: "owned", want: []string{"--home", home, "--daemon", "owned", "--token-file", "TOKEN", "--env-file", "ENV"}, wantToken: "entry-token"},
		"a switch to a local":                        {env: map[string]string{"KATA_HOME": home}, current: "mine", want: []string{"--home", home, "--daemon", "mine"}},
		"a local, the Pane's token":                  {env: map[string]string{"KATA_HOME": home, "KATA_AUTH_TOKEN": "pane-token"}, current: "mine", want: []string{"--home", home, "--daemon", "mine", "--token-file", "TOKEN"}, wantToken: "pane-token"},
		"a local, the Pane's file":                   {env: map[string]string{"KATA_HOME": home, "KATA_AUTH_TOKEN_FILE": "/run/t"}, current: "mine", want: []string{"--home", home, "--daemon", "mine", "--auth-file", "/run/t"}},
	} {
		t.Run(name, func(t *testing.T) {
			routeEnv(t, tc.env)
			g, dirs := agentGateway(t)
			g.current = tc.current
			ran, got := startRoutedAgent(t, g, dirs, nil)
			if got.Err != nil {
				t.Fatal(got.Err)
			}
			tokenFile := got.Entry.PromptFile + ".token"
			want := slices.Clone(tc.want)
			for i := range want {
				switch want[i] {
				case "TOKEN":
					want[i] = tokenFile
				case "ENV":
					want[i] = got.Entry.PromptFile + ".env"
				case "DEFAULT":
					want[i] = filepath.Join(os.Getenv("HOME"), ".kata")
				}
			}
			want = append(append([]string{"new", "tab", "--cwd", "/bound", "--json", "--", "tern-kata", "agent"}, want...), "--ref", "demo#cd34", "--", "omp")
			if len(ran) < len(want) || !slices.Equal(ran[:len(want)], want) {
				t.Fatalf("ran %q, want a prefix %q", ran, want)
			}
			for _, secret := range []string{"pane-token", "work-token"} {
				if strings.Contains(strings.Join(ran, " "), secret) {
					t.Errorf("the command line carries %s: %q", secret, ran)
				}
			}
			if tc.wantToken == "" {
				if _, err := os.Stat(tokenFile); err == nil {
					t.Error("a token file was written for a route without a token")
				}
				return
			}
			info, err := os.Stat(tokenFile)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("token file: %v, %v", info, err)
			}
			if tok, err := readTokenFile(tokenFile, false); err != nil || tok != tc.wantToken {
				t.Errorf("token file holds %q, %v; kata's checks must pass and it must hold %q", tok, err, tc.wantToken)
			}
			removePrompt(got.Entry.PromptFile)
			if _, err := os.Stat(tokenFile); err == nil {
				t.Error("the token file outlived its prompt file")
			}
		})
	}
}

func TestFailedAgentStartsLeaveNoTokenFile(t *testing.T) {
	routeEnv(t, map[string]string{"KATA_SERVER": "https://kata.example", "KATA_AUTH_TOKEN": "pane-token"})
	g, dirs := agentGateway(t)
	_, got := startRoutedAgent(t, g, dirs, errors.New("tern failed"))
	if got.Err == nil {
		t.Fatal("the start succeeded")
	}
	if left, _ := filepath.Glob(filepath.Join(dirs.State, AgentPromptsFolder, "*")); len(left) != 0 {
		t.Errorf("a failed start left %q", left)
	}
}

func TestAgentStartRefusedWhenTheSwitchedEntrysTokenCannotBeResolved(t *testing.T) {
	home := catalogHome(t, "[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\ntoken_env = \"WORK_TOKEN\"\n")
	routeEnv(t, map[string]string{"KATA_HOME": home})
	g, dirs := agentGateway(t)
	g.current = "work"
	g.runTern = func(context.Context, []string) ([]byte, error) { t.Error("tern was asked"); return nil, nil }
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentTab, IssueRef: "demo#cd34", ProjectName: "demo", Workspace: "/bound"}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "WORK_TOKEN") {
		t.Errorf("err = %v", got.Err)
	}
}

func catalogHome(t *testing.T, config string) string {
	t.Helper()
	home := t.TempDir()
	writeAgentFile(t, filepath.Join(home, "config.toml"), config)
	return home
}

func tokenFile(t *testing.T, text string) string {
	t.Helper()
	return writeAgentFile(t, filepath.Join(t.TempDir(), "token"), text)
}

// checkout is a git work tree with the given .kata.local.toml (none when empty) that becomes the working directory.
func checkout(t *testing.T, pin string) {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if pin != "" {
		writeAgentFile(t, filepath.Join(dir, ".kata.local.toml"), pin)
	}
	t.Chdir(dir)
}

func TestAgentRouteEnvironment(t *testing.T) {
	t.Setenv("WORK_TOKEN", "from-env")
	t.Setenv("MISSING_TOKEN", "")
	catalog := func(active, auth string) string {
		return catalogHome(t, "active_daemon = \""+active+"\"\n"+auth+
			"[[daemon]]\nname = \"mine\"\nlocal = true\n"+
			"[[daemon]]\nname = \"other\"\nlocal = true\n"+
			"[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\ntoken_env = \"WORK_TOKEN\"\nallow_insecure = true\n"+
			"[[daemon]]\nname = \"inline\"\nurl = \"https://i.example\"\ntoken = \"inline-token\"\n"+
			"[[daemon]]\nname = \"bare\"\nurl = \"https://b.example\"\n"+
			"[[daemon]]\nname = \"unset\"\nurl = \"https://u.example\"\ntoken_env = \"MISSING_TOKEN\"\n"+
			"[[daemon]]\nname = \"profile\"\nlocal = true\nhome = \"/profiles/p\"\ninstance_uid = \"01HZZZZZZZZZZZZZZZZZZZZZZZ\"\n")
	}
	local, remote, authed := catalog("mine", ""), catalog("work", ""), catalog("mine", "[auth]\ntoken_file = \"/etc/kata-token\"\n")
	const pane = "PANE" // replaced by a new token file per case: the wrapper deletes the one it reads
	// Whatever the block inherited is dropped first, so none of it reaches the Pane's daemon.
	allCreds := []string{"KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE", "KATA_TRUST_PRIVATE_NETWORK", "KATA_DSN", "KATA_DB", "KATA_POSTGRES_SCHEMA"}
	for name, tc := range map[string]struct {
		route   AgentRoute
		pin     string
		set     []string
		wantErr string
	}{
		"zero":                                         {route: AgentRoute{}},
		"home":                                         {route: AgentRoute{Home: "/h"}, set: []string{"KATA_HOME=/h"}},
		"home with the Pane's token":                   {route: AgentRoute{Home: "/h", TokenFile: pane}, set: []string{"KATA_HOME=/h", "KATA_AUTH_TOKEN=pane-token"}},
		"server":                                       {route: AgentRoute{Server: "https://s"}, set: []string{"KATA_SERVER=https://s"}},
		"server with a file":                           {route: AgentRoute{Server: "https://s", TokenFile: pane, AllowInsecure: true}, set: []string{"KATA_SERVER=https://s", "KATA_AUTH_TOKEN=pane-token", "KATA_ALLOW_INSECURE=1"}},
		"server, private network":                      {route: AgentRoute{Server: "http://100.64.0.5:7777", TokenFile: tokenFile(t, "t2\n"), TrustPrivate: true}, set: []string{"KATA_SERVER=http://100.64.0.5:7777", "KATA_AUTH_TOKEN=t2", "KATA_TRUST_PRIVATE_NETWORK=1"}},
		"the Pane's own token file is passed on":       {route: AgentRoute{Server: "https://s", AuthFile: "/run/t"}, set: []string{"KATA_SERVER=https://s", "KATA_AUTH_TOKEN_FILE=/run/t"}},
		"local entry, the Pane's token":                {route: AgentRoute{Home: local, Daemon: "mine", TokenFile: tokenFile(t, "t3\n")}, set: []string{"KATA_HOME=" + local, "KATA_AUTH_TOKEN=t3"}},
		"local entry, the Pane's file":                 {route: AgentRoute{Home: local, Daemon: "mine", AuthFile: "/run/t"}, set: []string{"KATA_HOME=" + local, "KATA_AUTH_TOKEN_FILE=/run/t"}},
		"a file kata would refuse":                     {route: AgentRoute{Server: "https://s", TokenFile: "relative"}, wantErr: "absolute"},
		"remote entry":                                 {route: AgentRoute{Home: local, Daemon: "work", TokenFile: pane}, set: []string{"KATA_HOME=" + local, "KATA_SERVER=https://kata.example", "KATA_AUTH_TOKEN=pane-token", "KATA_ALLOW_INSECURE=1"}},
		"remote entry, no file":                        {route: AgentRoute{Home: local, Daemon: "work"}, set: []string{"KATA_HOME=" + local, "KATA_SERVER=https://kata.example", "KATA_AUTH_TOKEN=from-env", "KATA_ALLOW_INSECURE=1"}},
		"inline token":                                 {route: AgentRoute{Home: local, Daemon: "inline"}, set: []string{"KATA_HOME=" + local, "KATA_SERVER=https://i.example", "KATA_AUTH_TOKEN=inline-token"}},
		"entry with no token":                          {route: AgentRoute{Home: local, Daemon: "bare"}, set: []string{"KATA_HOME=" + local, "KATA_SERVER=https://b.example"}},
		"no token but [auth]":                          {route: AgentRoute{Home: authed, Daemon: "bare"}, wantErr: "[auth] token_file that kata would send"},
		"[auth] but a token":                           {route: AgentRoute{Home: authed, Daemon: "inline"}, set: []string{"KATA_HOME=" + authed, "KATA_SERVER=https://i.example", "KATA_AUTH_TOKEN=inline-token"}},
		"token_env unset":                              {route: AgentRoute{Home: local, Daemon: "unset"}, wantErr: `token_env "MISSING_TOKEN" is unset`},
		"local profile":                                {route: AgentRoute{Home: local, Daemon: "profile"}, set: []string{"KATA_HOME=/profiles/p"}},
		"profile, the Pane's token":                    {route: AgentRoute{Home: local, Daemon: "profile", TokenFile: tokenFile(t, "t4\n")}, set: []string{"KATA_HOME=/profiles/p", "KATA_AUTH_TOKEN=t4"}},
		"the current local":                            {route: AgentRoute{Home: local, Daemon: "other"}, set: []string{"KATA_HOME=" + local}},
		"unreachable local":                            {route: AgentRoute{Home: remote, Daemon: "mine"}, wantErr: "no variable that names a local daemon"},
		"local, pin to a url":                          {route: AgentRoute{Home: local, Daemon: "mine"}, pin: "version = 1\n[server]\nurl = \"https://pinned.example\"\n", wantErr: "pins https://pinned.example"},
		"local, pin to another":                        {route: AgentRoute{Home: local, Daemon: "mine"}, pin: "version = 1\n[server]\ndaemon = \"other\"\n", wantErr: "pins daemon other"},
		"local without a home, pin to itself":          {route: AgentRoute{Home: local, Daemon: "mine"}, pin: "version = 1\n[server]\ndaemon = \"mine\"\n", wantErr: "pins daemon mine"},
		"profile, pin to itself keeps the Pane's home": {route: AgentRoute{Home: local, Daemon: "profile"}, pin: "version = 1\n[server]\ndaemon = \"profile\"\n", set: []string{"KATA_HOME=" + local}},
		"profile, pin to another":                      {route: AgentRoute{Home: local, Daemon: "profile"}, pin: "version = 1\n[server]\ndaemon = \"other\"\n", wantErr: "pins daemon other"},
		"remote beats a pin":                           {route: AgentRoute{Home: local, Daemon: "bare"}, pin: "version = 1\n[server]\nurl = \"https://pinned.example\"\n", set: []string{"KATA_HOME=" + local, "KATA_SERVER=https://b.example"}},
		"an entry not listed":                          {route: AgentRoute{Home: local, Daemon: "nope"}, wantErr: `daemon "nope" is not in the catalog`},
	} {
		t.Run(name, func(t *testing.T) {
			checkout(t, tc.pin)
			if tc.route.TokenFile == pane {
				tc.route.TokenFile = tokenFile(t, "pane-token\n")
			}
			set, unset, err := tc.route.Env("")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			wantUnset := allCreds
			if tc.route.IsZero() {
				wantUnset = nil
			}
			if err != nil || !slices.Equal(set, tc.set) || !slices.Equal(unset, wantUnset) {
				t.Errorf("set %q, unset %q, err %v; want %q, %q", set, unset, err, tc.set, wantUnset)
			}
		})
	}
}

func TestAgentRouteEnvDeletesThePaneWrittenTokenFileButNotThePanesOwn(t *testing.T) {
	written, own := tokenFile(t, "pane-token\n"), tokenFile(t, "own-token\n")
	if _, _, err := (AgentRoute{Server: "https://s", TokenFile: written, AuthFile: own}).Env(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(written); err == nil {
		t.Error("the Pane-written token file outlived the wrapper's read")
	}
	if _, err := os.Stat(own); err != nil {
		t.Errorf("the Pane's own KATA_AUTH_TOKEN_FILE was touched: %v", err)
	}
	// The Pane's preflight reads nothing and deletes nothing.
	kept := tokenFile(t, "pane-token\n")
	if err := (AgentRoute{Server: "https://s", TokenFile: kept}).Preflight(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("Preflight removed the token file: %v", err)
	}
}

func TestAgentStartIsRefusedBeforeAnythingIsWrittenWhenTheRouteCannotWork(t *testing.T) {
	home := catalogHome(t, "active_daemon = \"work\"\n[[daemon]]\nname = \"mine\"\nlocal = true\n[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\n")
	routeEnv(t, map[string]string{"KATA_HOME": home})
	g, dirs := agentGateway(t)
	g.current = "mine"
	g.runTern = func(context.Context, []string) ([]byte, error) { t.Error("tern was asked"); return nil, nil }
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	got := as[model.AgentStarted](t, run(t, g, model.StartAgent{Placement: model.AgentBackground, IssueRef: "demo#cd34", ProjectName: "demo", Workspace: t.TempDir()}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "active_daemon") {
		t.Errorf("err = %v", got.Err)
	}
	if left, _ := filepath.Glob(filepath.Join(dirs.State, AgentPromptsFolder, "*")); len(left) != 0 {
		t.Errorf("a refused start left %q", left)
	}
}

func TestRouteFromAPaneWithoutKataHomeNamesTheDefaultHomeOnlyWhenRouted(t *testing.T) {
	routeEnv(t, nil)
	g := NewGateway(nil)
	if route, err := g.CurrentAgentRoute(); err != nil || !route.IsZero() {
		t.Errorf("a default Pane's route is %+v, %v; want zero", route, err)
	}
	t.Setenv("KATA_SERVER", "https://kata.example")
	route, err := g.CurrentAgentRoute()
	if want := filepath.Join(os.Getenv("HOME"), ".kata"); err != nil || route.Home != want {
		t.Errorf("a routed Pane's Home = %q, %v; want %q", route.Home, err, want)
	}
}

func TestTokenFileSurvivesAFailedRead(t *testing.T) {
	file := tokenFile(t, "")
	if _, _, err := (AgentRoute{Server: "https://s", TokenFile: file}).Env(""); err == nil {
		t.Fatal("an empty token file was accepted")
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("a file that could not be read was removed: %v", err)
	}
}

func TestRoutedAgentCarriesThePanesStorageButNeverADSNOnTheCommandLine(t *testing.T) {
	routeEnv(t, map[string]string{"KATA_HOME": "rel-home", "KATA_DB": "rel.db", "KATA_POSTGRES_SCHEMA": "kata_s", "KATA_DSN": "postgres://u:pw-secret@h/db"})
	cwd := t.TempDir()
	t.Chdir(cwd)
	g, dirs := agentGateway(t)
	ran, got := startRoutedAgent(t, g, dirs, nil)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	dsnFile := got.Entry.PromptFile + ".dsn"
	want := []string{"tern-kata", "agent", "--home", filepath.Join(cwd, "rel-home"), "--db", filepath.Join(cwd, "rel.db"), "--schema", "kata_s", "--dsn-file", dsnFile, "--ref", "demo#cd34", "--", "omp"}
	if i := slices.Index(ran, "tern-kata"); i < 0 || !slices.Equal(ran[i:i+len(want)], want) {
		t.Fatalf("ran %q, want ... %q", ran, want)
	}
	if strings.Contains(strings.Join(ran, " "), "pw-secret") {
		t.Errorf("the command line carries the DSN password: %q", ran)
	}
	if dsn, err := readTokenFile(dsnFile, false); err != nil || dsn != "postgres://u:pw-secret@h/db" {
		t.Errorf("DSN file holds %q, %v", dsn, err)
	}
	removePrompt(got.Entry.PromptFile)
	if _, err := os.Stat(dsnFile); err == nil {
		t.Error("the DSN file outlived its prompt file")
	}
}

func TestRoutedAgentDoesNotCarryStorageToARemoteRoute(t *testing.T) {
	routeEnv(t, map[string]string{"KATA_SERVER": "https://kata.example", "KATA_DB": "/x.db", "KATA_DSN": "postgres://u:pw@h/db"})
	g, dirs := agentGateway(t)
	ran, got := startRoutedAgent(t, g, dirs, nil)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if joined := strings.Join(ran, " "); strings.Contains(joined, "--db") || strings.Contains(joined, "--dsn-file") {
		t.Errorf("a server route carries storage: %q", ran)
	}
}

func TestRoutedAgentCarriesTheCredentialVariablesTheHomeNamesNotAGlobalToken(t *testing.T) {
	config := "active_daemon = \"work\"\n[auth]\ntoken_env = \"PANE_AUTH\"\n" +
		"[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\ntoken_env = \"WORK_TOKEN\"\n" +
		"[[daemon]]\nname = \"p\"\nlocal = true\nhome = \"/profiles/p\"\ninstance_uid = \"01HZZZZZZZZZZZZZZZZZZZZZZZ\"\ntoken_env = \"PROFILE_TOKEN\"\n" +
		"[[daemon]]\nname = \"unused\"\nurl = \"https://u.example\"\ntoken_env = \"UNSET_TOKEN\"\n"
	home := catalogHome(t, config)
	routeEnv(t, map[string]string{"KATA_HOME": home})
	t.Setenv("PANE_AUTH", "auth-value")
	t.Setenv("PROFILE_TOKEN", "profile-value")
	t.Setenv("WORK_TOKEN", "work-value")
	t.Setenv("UNSET_TOKEN", "")
	g, dirs := agentGateway(t)
	ran, got := startRoutedAgent(t, g, dirs, nil)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if strings.Contains(strings.Join(ran, " "), "--token-file") {
		t.Errorf("a global token is carried for a connection kata scopes by entry: %q", ran)
	}
	text, err := readTokenFile(got.Entry.PromptFile+".env", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"WORK_TOKEN=work-value", "PROFILE_TOKEN=profile-value", "PANE_AUTH=auth-value"} {
		if !strings.Contains(text, line+"\n") && !strings.HasSuffix(text, line) {
			t.Errorf("env file %q lacks %s", text, line)
		}
	}
	if strings.Contains(text, "UNSET_TOKEN") {
		t.Errorf("an unset variable was written: %q", text)
	}
	removePrompt(got.Entry.PromptFile)
	if _, err := os.Stat(got.Entry.PromptFile + ".env"); err == nil {
		t.Error("the env file outlived its prompt file")
	}
	// The route's own environment carries no KATA_AUTH_TOKEN, so a checkout pinned to another url receives none.
	checkout(t, "version = 1\n[server]\nurl = \"http://127.0.0.1:18777\"\n")
	envFile := tokenFile(t, "WORK_TOKEN=work-value\nKATA_SERVER=evil\nPATH=/x\n")
	set, _, err := (AgentRoute{Home: home, EnvFile: envFile}).Env("")
	if err != nil || slices.Contains(set, "KATA_AUTH_TOKEN=work-value") || !slices.Contains(set, "WORK_TOKEN=work-value") || slices.Contains(set, "KATA_SERVER=evil") {
		t.Errorf("set %q, err %v", set, err)
	}
	if _, err := os.Stat(envFile); err == nil {
		t.Error("the env file outlived the wrapper's read")
	}
}

func TestAgentRouteEnvFileFollowsTheRoute(t *testing.T) {
	home := catalogHome(t, "[auth]\ntoken_env = \"PANE_AUTH\"\n[[daemon]]\nname = \"mine\"\nlocal = true\n[[daemon]]\nname = \"work\"\nurl = \"https://w.example\"\ntoken_env = \"WORK_TOKEN\"\n")
	for name, tc := range map[string]struct {
		env     map[string]string
		current string
		want    string
	}{
		"a server route sends only [auth]":                {env: map[string]string{"KATA_SERVER": "https://s", "KATA_HOME": home}, want: "PANE_AUTH=a\n"},
		"a plain local entry (own [auth] token_env)":      {env: map[string]string{"KATA_HOME": home}, current: "mine", want: "WORK_TOKEN=w\nPANE_AUTH=a\n"},
		"a switched remote entry uses its explicit token": {env: map[string]string{"KATA_HOME": home}, current: "work", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			routeEnv(t, tc.env)
			t.Setenv("PANE_AUTH", "a")
			t.Setenv("WORK_TOKEN", "w")
			g, _ := agentGateway(t)
			g.current = tc.current
			route, err := g.CurrentAgentRoute()
			if err != nil || route.Creds != tc.want {
				t.Errorf("Creds %q, %v; want %q", route.Creds, err, tc.want)
			}
		})
	}
}

func TestAgentRouteStorageEnvironment(t *testing.T) {
	local := catalogHome(t, "[[daemon]]\nname = \"mine\"\nlocal = true\n[[daemon]]\nname = \"profile\"\nlocal = true\nhome = \"/profiles/p\"\ninstance_uid = \"01HZZZZZZZZZZZZZZZZZZZZZZZ\"\n")
	for name, tc := range map[string]struct {
		route AgentRoute
		set   []string
	}{
		"home only":    {AgentRoute{Home: local, DB: "/p.db", Schema: "s", DSNFile: "replaced"}, []string{"KATA_HOME=" + local, "KATA_DB=/p.db", "KATA_POSTGRES_SCHEMA=s", "KATA_DSN=postgres://u:pw@h/db"}},
		"plain local":  {AgentRoute{Home: local, Daemon: "mine", DB: "/p.db"}, []string{"KATA_HOME=" + local, "KATA_DB=/p.db"}},
		"profile home": {AgentRoute{Home: local, Daemon: "profile", DB: "/p.db", Schema: "s"}, []string{"KATA_HOME=/profiles/p"}},
		"server route": {AgentRoute{Home: local, Server: "https://s", DB: "/p.db", Schema: "s"}, []string{"KATA_HOME=" + local, "KATA_SERVER=https://s"}},
	} {
		t.Run(name, func(t *testing.T) {
			checkout(t, "")
			if tc.route.DSNFile != "" {
				tc.route.DSNFile = tokenFile(t, "postgres://u:pw@h/db\n")
			}
			set, unset, err := tc.route.Env("")
			if err != nil || !slices.Equal(set, tc.set) || !slices.Contains(unset, "KATA_DSN") || !slices.Contains(unset, "KATA_DB") || !slices.Contains(unset, "KATA_POSTGRES_SCHEMA") {
				t.Errorf("set %q, unset %q, err %v; want %q and the storage variables unset", set, unset, err, tc.set)
			}
			if tc.route.DSNFile != "" {
				if _, err := os.Stat(tc.route.DSNFile); err == nil {
					t.Error("the DSN file outlived the wrapper's read")
				}
			}
		})
	}
}

func TestCredentialNamesAcceptKataTokenVariablesButNotTheOnesTheRouteOwns(t *testing.T) {
	for name, want := range map[string]bool{
		"KATA_WORK_TOKEN": true, "KATA_SPOKE_TOKEN": true, "WORK_TOKEN": true,
		"KATA_SERVER": false, "kata_home": false, "KATA_AUTH_TOKEN": false, "KATA_REF": false, "KATA_DSN": false, "": false, "A-B": false, "1X": false,
	} {
		if got := credentialName(name); got != want {
			t.Errorf("credentialName(%q) = %v, want %v", name, got, want)
		}
	}
	home := catalogHome(t, "[[daemon]]\nname = \"w\"\nurl = \"https://w.example\"\ntoken_env = \"KATA_WORK_TOKEN\"\n[[daemon]]\nname = \"mine\"\nlocal = true\n")
	t.Setenv("KATA_WORK_TOKEN", "kw")
	if got := paneCredentialEnv(home, true); got != "KATA_WORK_TOKEN=kw\n" {
		t.Errorf("env = %q", got)
	}
	set, _, err := (AgentRoute{Home: home, EnvFile: tokenFile(t, "KATA_WORK_TOKEN=kw\n")}).Env("")
	if err != nil || !slices.Contains(set, "KATA_WORK_TOKEN=kw") {
		t.Errorf("set %q, err %v", set, err)
	}
}

func TestSwitchedProfileWithItsOwnHomeGetsNoneOfThePanesCredentialVariables(t *testing.T) {
	home := catalogHome(t, "[[daemon]]\nname = \"p\"\nlocal = true\nhome = \"/profiles/p\"\ninstance_uid = \"01HZZZZZZZZZZZZZZZZZZZZZZZ\"\n")
	checkout(t, "")
	envFile := tokenFile(t, "HUB_ADMIN_TOKEN=admin\n")
	set, _, err := (AgentRoute{Home: home, Daemon: "p", EnvFile: envFile}).Env("")
	if err != nil || !slices.Equal(set, []string{"KATA_HOME=/profiles/p"}) {
		t.Errorf("set %q, err %v", set, err)
	}
	if _, err := os.Stat(envFile); err == nil {
		t.Error("the env file was not deleted")
	}
	// A pin naming the profile keeps the Pane's home, and with it the Pane's variables.
	checkout(t, "version = 1\n[server]\ndaemon = \"p\"\n")
	set, _, err = (AgentRoute{Home: home, Daemon: "p", EnvFile: tokenFile(t, "HUB_ADMIN_TOKEN=admin\n")}).Env("")
	if err != nil || !slices.Contains(set, "HUB_ADMIN_TOKEN=admin") {
		t.Errorf("pinned: set %q, err %v", set, err)
	}
}
