package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// boundCheckout is a temp directory bound to a kata Project by a .kata.toml.
func boundCheckout(t *testing.T, project string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".kata.toml"), []byte("version = 1\n\n[project]\nname = \""+project+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// setupEnv is a worker environment with its own config folder, working in dir.
func setupEnv(t *testing.T, dir string) workerEnv {
	t.Helper()
	return workerEnv{Dirs: worker.Dirs{Config: t.TempDir(), State: t.TempDir()}, Dir: dir, Hostname: "My Laptop.local"}
}

// runSetup runs `tern-kata worker <sub>` through the argument parser and the runner.
func runSetup(t *testing.T, env workerEnv, sub string, flags ...string) (string, error) {
	t.Helper()
	plan, err := PlanLaunch(append([]string{"worker", sub}, flags...), func(string) (string, bool) { return "robot", true }, "test")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = runWorkerCommand(context.Background(), plan, env, &out)
	return out.String(), err
}

func readConfig(t *testing.T, env workerEnv) string {
	t.Helper()
	raw, err := os.ReadFile(env.Dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestWorkerInitWritesALoadableFileForTheBoundProject(t *testing.T) {
	checkout := boundCheckout(t, "Acme Shop")
	env := setupEnv(t, checkout)
	out, err := runSetup(t, env, "init")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := worker.LoadConfig(env.Dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Executor != "my-laptop-local" {
		t.Errorf("executor %q", cfg.Executor)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0].Name != "Acme Shop" || cfg.Projects[0].Capacity != 1 ||
		len(cfg.Projects[0].Checkouts) != 1 || cfg.Projects[0].Checkouts[0] != "acme-shop" {
		t.Errorf("projects %+v", cfg.Projects)
	}
	if cfg.Checkouts["acme-shop"] != checkout {
		t.Errorf("checkouts %v, want %s", cfg.Checkouts, checkout)
	}
	if cfg.Profiles["default"].Command != "omp" {
		t.Errorf("profiles %+v", cfg.Profiles)
	}
	if info, _ := os.Stat(env.Dirs.ConfigPath()); info.Mode().Perm() != 0o600 && filepath.Separator == '/' {
		t.Errorf("mode %v", info.Mode())
	}
	for _, want := range []string{env.Dirs.ConfigPath(), "worker doctor", "10 seconds", "run prompts and commands on this machine as you"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(readConfig(t, env), `# actor = "robot"`) {
		t.Errorf("no commented actor:\n%s", readConfig(t, env))
	}
}

func TestWorkerInitRefusesAnExistingFileAndAnUnboundDirectory(t *testing.T) {
	env := setupEnv(t, boundCheckout(t, "demo"))
	if _, err := runSetup(t, env, "init"); err != nil {
		t.Fatal(err)
	}
	before := readConfig(t, env)
	if _, err := runSetup(t, env, "init"); err == nil || !strings.Contains(err.Error(), "tern-kata worker add") {
		t.Fatalf("error %v", err)
	}
	if readConfig(t, env) != before {
		t.Error("the existing file changed")
	}

	unbound := setupEnv(t, t.TempDir())
	if _, err := runSetup(t, unbound, "init"); err == nil || !strings.Contains(err.Error(), "kata init") {
		t.Fatalf("error %v", err)
	}
	if _, err := os.Stat(unbound.Dirs.ConfigPath()); err == nil {
		t.Error("a file was written for an unbound directory")
	}
}

func TestWorkerAddAppendsASecondProjectAndKeepsTheRestByteForByte(t *testing.T) {
	first, second := boundCheckout(t, "alpha"), boundCheckout(t, "Beta")
	env := setupEnv(t, first)
	if _, err := runSetup(t, env, "init"); err != nil {
		t.Fatal(err)
	}
	// Put a comment and an odd line right after the [checkouts] key, and a table after it.
	edited := strings.Replace(readConfig(t, env), "[profiles.default]", "# my notes   \n\n[profiles.default]", 1)
	if err := os.WriteFile(env.Dirs.ConfigPath(), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	env.Dir = second
	out, err := runSetup(t, env, "add")
	if err != nil {
		t.Fatal(err)
	}
	after := readConfig(t, env)
	cfg, err := worker.LoadConfig(env.Dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 2 || cfg.Projects[1].Name != "Beta" || cfg.Projects[1].Checkouts[0] != "beta" || cfg.Checkouts["beta"] != second || cfg.Checkouts["alpha"] != first {
		t.Fatalf("config %+v", cfg)
	}
	// Every original line is still there, in order, with the checkout line inside [checkouts].
	rest := after
	for _, line := range strings.SplitAfter(edited, "\n") {
		i := strings.Index(rest, line)
		if i < 0 {
			t.Fatalf("line %q lost or reordered:\n%s", line, after)
		}
		rest = rest[i+len(line):]
	}
	at, next := strings.Index(after, "[checkouts]"), strings.Index(after, "[profiles.default]")
	if k := strings.Index(after, "beta = "); k < at || k > next {
		t.Errorf("checkout line outside [checkouts]:\n%s", after)
	}
	for _, want := range []string{"worker.toml", "worker status", "only at start"} {
		if !strings.Contains(out, want) && want != "worker.toml" {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestWorkerAddCreatesTheCheckoutsTableWhenAbsent(t *testing.T) {
	env := setupEnv(t, boundCheckout(t, "beta"))
	body := "executor = \"x\" # keep\n[[projects]]\nname = \"alpha\"\n[profiles.default]\ncommand = \"omp\""
	if err := os.WriteFile(env.Dirs.ConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runSetup(t, env, "add"); err != nil {
		t.Fatal(err)
	}
	after := readConfig(t, env)
	if !strings.HasPrefix(after, body) {
		t.Errorf("original content changed:\n%s", after)
	}
	if cfg, err := worker.LoadConfig(env.Dirs.ConfigPath()); err != nil || len(cfg.Projects) != 2 || cfg.Checkouts["beta"] != env.Dir {
		t.Fatalf("%+v, %v", cfg, err)
	}
}

func TestWorkerAddRefusals(t *testing.T) {
	checkout := boundCheckout(t, "demo")
	env := setupEnv(t, checkout)
	if _, err := runSetup(t, env, "add"); err == nil || !strings.Contains(err.Error(), "tern-kata worker init") {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := runSetup(t, env, "init"); err != nil {
		t.Fatal(err)
	}
	before := readConfig(t, env)
	if _, err := runSetup(t, env, "add"); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("listed Project: %v", err)
	}
	unbound := env
	unbound.Dir = t.TempDir()
	if _, err := runSetup(t, unbound, "add"); err == nil || !strings.Contains(err.Error(), "kata init") {
		t.Fatalf("unbound: %v", err)
	}
	if readConfig(t, env) != before {
		t.Error("a refusal changed the file")
	}
}

func TestWorkerAddSuffixesAKeyClashAndReusesASameDirectory(t *testing.T) {
	env := setupEnv(t, boundCheckout(t, "Shop"))
	if _, err := runSetup(t, env, "init"); err != nil {
		t.Fatal(err)
	}
	other := boundCheckout(t, "SHOP")
	env.Dir = other
	if _, err := runSetup(t, env, "add"); err != nil {
		t.Fatal(err)
	}
	cfg, err := worker.LoadConfig(env.Dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Checkouts["shop-2"] != other || cfg.Projects[1].Checkouts[0] != "shop-2" {
		t.Fatalf("config %+v", cfg)
	}

	// The same key and directory is reused: no second line.
	third := boundCheckout(t, "reuse")
	body := strings.Replace(readConfig(t, env), "[checkouts]\n", "[checkouts]\nreuse = "+quote(third)+"\n", 1)
	if err := os.WriteFile(env.Dirs.ConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env.Dir = third
	if _, err := runSetup(t, env, "add"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(readConfig(t, env), "reuse = "); got != 1 {
		t.Errorf("%d reuse lines", got)
	}
}

func TestWorkerSetupNeverWritesAFileThatWouldNotLoad(t *testing.T) {
	env := setupEnv(t, boundCheckout(t, "beta"))
	broken := "executor = \"x\"\nbogus = 1\n[[projects]]\nname = \"alpha\"\n"
	if err := os.WriteFile(env.Dirs.ConfigPath(), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runSetup(t, env, "add")
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error %v", err)
	}
	if readConfig(t, env) != broken {
		t.Error("the file changed")
	}
	entries, _ := os.ReadDir(env.Dirs.Config)
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}

	// An executor the config rejects (init derives it from the host name) is refused before anything is written.
	fresh := setupEnv(t, boundCheckout(t, "demo"))
	fresh.Hostname = strings.Repeat("a", 300)
	if _, err := runSetup(t, fresh, "init"); err != nil {
		t.Fatalf("a long host name must be shortened, got %v", err)
	}
}

func TestWorkerSetupFormsAreListedAndParsed(t *testing.T) {
	for _, want := range []string{"tern-kata worker init", "tern-kata worker add"} {
		if !strings.Contains(usage, want) {
			t.Errorf("--help lacks %q", want)
		}
	}
	for _, args := range [][]string{{"worker", "init", "x"}, {"worker", "--takeover", "init"}} {
		if _, err := PlanLaunch(args, func(string) (string, bool) { return "", false }, "t"); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

func TestWorkerInitFlagsLandInTheFileAndItLoads(t *testing.T) {
	env := setupEnv(t, boundCheckout(t, "demo"))
	if _, err := runSetup(t, env, "init", "--executor", "box-1", "--actor=ci-bot", "--command", "/opt/omp", "--model", "openai/gpt-5", "--thinking=low"); err != nil {
		t.Fatal(err)
	}
	cfg, err := worker.LoadConfig(env.Dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["default"]
	if cfg.Executor != "box-1" || cfg.Actor != "ci-bot" || p.Command != "/opt/omp" || p.Model != "openai/gpt-5" || p.Thinking != "low" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWorkerInitRefusesAValueTheLoaderRefusesAndWritesNothing(t *testing.T) {
	env := setupEnv(t, boundCheckout(t, "demo"))
	_, err := runSetup(t, env, "init", "--executor", "has space")
	if err == nil || !strings.Contains(err.Error(), "executor") {
		t.Fatalf("error %v", err)
	}
	if entries, _ := os.ReadDir(env.Dirs.Config); len(entries) != 0 {
		t.Errorf("files written: %v", entries)
	}
}

func TestWorkerInitFlagsApplyToInitOnly(t *testing.T) {
	for _, args := range [][]string{{"worker", "add", "--model", "x"}, {"worker", "--executor", "x"}, {"worker", "init", "--model"}, {"worker", "init", "--model", ""}, {"worker", "init", "--model=a", "--model=b"}} {
		if _, err := PlanLaunch(args, func(string) (string, bool) { return "", false }, "t"); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	for _, flag := range []string{"--executor", "--actor", "--command", "--model", "--thinking"} {
		if !strings.Contains(usage, flag) {
			t.Errorf("--help lacks %s", flag)
		}
	}
}
