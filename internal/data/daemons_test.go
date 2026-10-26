package data

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const profileInstanceUID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

// writeCatalog writes kata's client config into the throwaway KATA_HOME the test service set.
func writeCatalog(t *testing.T, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func remoteEntry(name string, server *kataServer) string {
	return "[[daemon]]\nname = \"" + name + "\"\nurl = \"" + server.URL + "\"\nallow_insecure = true\n\n"
}

// switched runs a switch to name and returns its answer.
func switched(t *testing.T, gateway *Gateway, workspace, name string) model.DaemonSwitched {
	t.Helper()
	return as[model.DaemonSwitched](t, run(t, gateway, model.SwitchDaemon{Workspace: workspace, Request: 1, Name: name}))
}

func TestGatewaySwitchesBetweenTwoInProcessDaemons(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	writeCatalog(t, "active_daemon = \"alpha\"\n\n"+remoteEntry("alpha", alpha)+remoteEntry("beta", beta))
	gateway := newGateway(t, newLocator(alpha.target()))
	betaGateway := newGateway(t, newLocator(beta.target()))

	workspace, alphaProject := project(t, gateway, "shared")
	createIssue(t, gateway, workspace, alphaProject, "Served by alpha")
	_, betaProject := project(t, betaGateway, "shared")
	createIssue(t, betaGateway, workspace, betaProject, "Served by beta")

	query := func(p model.Project) model.SnapshotQuery {
		return model.CollectionQuery(model.Collection{View: "project", ProjectUID: p.UID, Title: p.Name}, model.Filters{})
	}
	titles := func(p model.Project) []string {
		loaded := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query(p), Request: 1}))
		var out []string
		for _, issue := range loaded.Snapshot.Issues {
			out = append(out, issue.Title)
		}
		return out
	}
	if got := titles(alphaProject); !slices.Equal(got, []string{"Served by alpha"}) {
		t.Fatalf("before the switch the gateway served %v", got)
	}
	opened, done := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 1})
	as[model.StreamOpened](t, receive(t, opened))

	answer := switched(t, gateway, workspace, "beta")
	if answer.Err != nil || answer.Name != "beta" || answer.Request != 1 || answer.Daemon.Name != "beta" || answer.Daemon.Kind != model.DaemonRemote || answer.Daemon.Version != "v0.20.0" {
		t.Fatalf("switch answered %+v", answer)
	}
	<-done // the old daemon's event stream ended

	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace, Switched: true, Connection: 4}))
	if loaded.Connection != 4 {
		t.Fatalf("the answer echoed connection %d, want 4", loaded.Connection)
	}
	if loaded.Project.UID != betaProject.UID || loaded.Daemon.Name != "beta" {
		t.Fatalf("the workspace resolves to %+v on %+v, want beta's Project", loaded.Project, loaded.Daemon)
	}
	if got := titles(betaProject); !slices.Equal(got, []string{"Served by beta"}) {
		t.Fatalf("after the switch the gateway served %v", got)
	}
	events, _ := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 2})
	as[model.StreamOpened](t, receive(t, events))
	createIssue(t, betaGateway, workspace, betaProject, "Live on beta")
	for {
		if event := as[model.StreamEvent](t, receive(t, events)); event.Type == "issue.created" {
			break
		}
	}

	daemons := as[model.DaemonsLoaded](t, run(t, gateway, model.LoadDaemons{Request: 2}))
	var current []string
	for _, e := range daemons.Entries {
		if e.Current {
			current = append(current, e.Name)
		}
	}
	if !slices.Equal(current, []string{"beta"}) {
		t.Fatalf("daemons marked current after the switch: %v, want beta only", current)
	}
}

func TestGatewaySwitchToAProjectTheNewDaemonLacksStartsOnTheInbox(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	writeCatalog(t, remoteEntry("alpha", alpha)+remoteEntry("beta", beta))
	gateway := newGateway(t, newLocator(alpha.target()))
	workspace, _ := project(t, gateway, "only-on-alpha")

	if answer := switched(t, gateway, workspace, "beta"); answer.Err != nil {
		t.Fatal(answer.Err)
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace, Switched: true}))
	if loaded.Project.UID != "" || loaded.Daemon.Name != "beta" {
		t.Fatalf("loaded %+v, want no Project on beta", loaded)
	}
	if failed := run(t, gateway, model.LoadProject{Workspace: workspace}); failed == nil {
		t.Fatal("no answer")
	} else if _, isFailure := failed.(model.Failed); !isFailure {
		t.Fatalf("an ordinary load of a Project the daemon lacks answered %T, want a failure", failed)
	}
}

func TestGatewayFailedSwitchKeepsTheCurrentConnection(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	t.Setenv("MISSING_TOKEN", "")
	writeCatalog(t, remoteEntry("alpha", alpha)+
		"[[daemon]]\nname = \"gated\"\nurl = \""+beta.URL+"\"\ntoken_env = \"MISSING_TOKEN\"\nallow_insecure = true\n\n"+
		"[[daemon]]\nname = \"down\"\nurl = \"http://127.0.0.1:1\"\nallow_insecure = true\n")
	gateway := newGateway(t, newLocator(alpha.target()))
	workspace, bound := project(t, gateway, "demo")

	for name, want := range map[string]string{
		"gated":   `token_env "MISSING_TOKEN" is unset or empty`,
		"missing": "not in kata's daemon catalog",
	} {
		answer := switched(t, gateway, workspace, name)
		if answer.Err == nil || answer.Err.Error() != want || answer.Name != name {
			t.Errorf("switch to %q answered %+v, want the error %q without a daemon prefix", name, answer, want)
		}
	}
	if answer := switched(t, gateway, workspace, "down"); answer.Err == nil {
		t.Error("switching to a daemon that does not answer succeeded")
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Project.UID != bound.UID {
		t.Fatalf("after failed switches the gateway resolves %+v, want the original daemon's Project", loaded.Project)
	}
}

// stubKata puts a `kata` script first on PATH. locate answers with url when started is set or startsFirst is false, and refuses
// otherwise; `daemon start` and `daemon recover` record their environment in the returned file, sets started, and fails with failure when that is not "".
type stubKata struct {
	dir, log, env, started string
}

func installStubKata(t *testing.T, url string, startsFirst bool, failure string) stubKata {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub kata is a shell script")
	}
	s := stubKata{dir: t.TempDir()}
	s.log, s.env, s.started = filepath.Join(s.dir, "log"), filepath.Join(s.dir, "env"), filepath.Join(s.dir, "started")
	gate := "true"
	if startsFirst {
		gate = "[ -e '" + s.started + "' ]"
	}
	start := "env | sort > '" + s.env + "'\ntouch '" + s.started + "'\necho started\nexit 0\n"
	if failure != "" {
		start = "env | sort > '" + s.env + "'\necho '" + failure + "' >&2\nexit 1\n"
	}
	script := "#!/bin/sh\necho \"$@\" >> '" + s.log + "'\n" +
		"case \"$*\" in\n" +
		"*\"daemon start\"*|*\"daemon recover\"*)\n" + start + ";;\n" +
		"*\"daemon locate\"*)\n" +
		"if " + gate + "; then\n" +
		"echo '{\"source\":\"daemon_flag\",\"kind\":\"local\",\"network\":\"tcp\",\"scheme\":\"http\",\"address\":\"" + strings.TrimPrefix(url, "http://") + "\",\"request_base_url\":\"" + url + "\"}'\n" +
		"else\necho '{\"error\":{\"message\":\"no daemon is running for the profile\"}}'\nexit 1\nfi;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(s.dir, "kata"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", s.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return s
}

func (s stubKata) lines(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(s.log)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (s stubKata) environment(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(s.env)
	if err != nil {
		t.Fatalf("the stub kata never ran `daemon start` or `daemon recover`: %v", err)
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if name, value, ok := strings.Cut(line, "="); ok {
			env[name] = value
		}
	}
	return env
}

func TestGatewaySwitchesToARunningLocalEntryWithoutStartingIt(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	writeCatalog(t, remoteEntry("alpha", alpha)+"[[daemon]]\nname = \"mine\"\nlocal = true\n")
	stub := installStubKata(t, beta.URL, false, "")
	gateway := newGateway(t, newLocator(alpha.target()))

	answer := switched(t, gateway, t.TempDir(), "mine")
	if answer.Err != nil || answer.Daemon.Kind != model.DaemonLocal || answer.Daemon.Name != "mine" {
		t.Fatalf("switch answered %+v", answer)
	}
	if lines := stub.lines(t); !slices.Equal(lines, []string{"--daemon mine daemon locate --json"}) {
		t.Fatalf("kata ran %q, want one locate and no start", lines)
	}
}

func TestGatewayRecoversAStoppedProfileThroughKatasRecoverThenConnects(t *testing.T) {
	profileHome := t.TempDir()
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	current := os.Getenv("KATA_HOME")
	writeCatalog(t, remoteEntry("alpha", alpha)+"[[daemon]]\nname = \"mine\"\nlocal = true\nhome = \""+profileHome+"\"\ninstance_uid = \""+profileInstanceUID+"\"\n")
	stub := installStubKata(t, beta.URL, true, "")
	gateway := newGateway(t, newLocator(alpha.target()))

	answer := switched(t, gateway, t.TempDir(), "mine")
	if answer.Err != nil || answer.Daemon.Name != "mine" || answer.Daemon.Kind != model.DaemonLocal {
		t.Fatalf("switch answered %+v", answer)
	}
	want := []string{"--daemon mine daemon locate --json", "--daemon mine daemon recover", "--daemon mine daemon locate --json"}
	if lines := stub.lines(t); !slices.Equal(lines, want) {
		t.Fatalf("kata ran %q, want %q", lines, want)
	}
	if home := stub.environment(t)["KATA_HOME"]; home != current {
		t.Errorf("`kata daemon recover` ran with KATA_HOME=%q, want the current %q left alone, not the profile's %q", home, current, profileHome)
	}
}

func TestGatewayStartsTheCurrentHomeEntryWithAScrubbedEnvironmentThenConnects(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	current := os.Getenv("KATA_HOME")
	writeCatalog(t, remoteEntry("alpha", alpha)+"[[daemon]]\nname = \"mine\"\nlocal = true\n")
	t.Setenv("KATA_DSN", "postgres://elsewhere")
	t.Setenv("KATA_DB", "/elsewhere.db")
	t.Setenv("KATA_SERVER", "http://elsewhere.example")
	stub := installStubKata(t, beta.URL, true, "")
	gateway := newGateway(t, newLocator(alpha.target()))

	answer := switched(t, gateway, t.TempDir(), "mine")
	if answer.Err != nil || answer.Daemon.Name != "mine" || answer.Daemon.Kind != model.DaemonLocal {
		t.Fatalf("switch answered %+v", answer)
	}
	want := []string{"--daemon mine daemon locate --json", "daemon start", "--daemon mine daemon locate --json"}
	if lines := stub.lines(t); !slices.Equal(lines, want) {
		t.Fatalf("kata ran %q, want %q", lines, want)
	}
	env := stub.environment(t)
	for _, name := range []string{"KATA_DSN", "KATA_DB", "KATA_SERVER"} {
		if v, set := env[name]; set {
			t.Errorf("`kata daemon start` ran with %s=%q", name, v)
		}
	}
	if env["KATA_HOME"] != current {
		t.Errorf("`kata daemon start` ran with KATA_HOME=%q, want %q", env["KATA_HOME"], current)
	}
}

func TestGatewayRefusedRecoverKeepsTheCurrentConnectionAndShowsKatasMessage(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	writeCatalog(t, remoteEntry("alpha", alpha)+"[[daemon]]\nname = \"mine\"\nlocal = true\nhome = \""+t.TempDir()+"\"\ninstance_uid = \""+profileInstanceUID+"\"\n")
	installStubKata(t, beta.URL, true, "kata: missing_profile_storage: restore the profile database")
	gateway := newGateway(t, newLocator(alpha.target()))
	workspace, bound := project(t, gateway, "demo")

	answer := switched(t, gateway, workspace, "mine")
	if answer.Err == nil || answer.Err.Error() != "kata daemon recover: missing_profile_storage: restore the profile database" {
		t.Fatalf("switch answered %+v, want kata's message", answer)
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Project.UID != bound.UID {
		t.Fatalf("the gateway moved off the current daemon: %+v", loaded.Project)
	}
}

func TestGatewayFailedStartKeepsTheCurrentConnectionAndShowsKatasReason(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	beta := startKata(t, "0.20.0")
	writeCatalog(t, remoteEntry("alpha", alpha)+"[[daemon]]\nname = \"mine\"\nlocal = true\n")
	installStubKata(t, beta.URL, true, "port in use")
	gateway := newGateway(t, newLocator(alpha.target()))
	workspace, bound := project(t, gateway, "demo")

	answer := switched(t, gateway, workspace, "mine")
	if answer.Err == nil || answer.Err.Error() != "kata daemon start: port in use" {
		t.Fatalf("switch answered %+v, want kata's reason", answer)
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Project.UID != bound.UID {
		t.Fatalf("the gateway moved off the current daemon: %+v", loaded.Project)
	}
}

func TestGatewayDaemonsListsTheCatalogInKataTuisColumns(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	t.Setenv("BETA_TOKEN", "x")
	home := os.Getenv("KATA_HOME")
	writeCatalog(t, "active_daemon = \"beta\"\n\n"+
		"[[daemon]]\nname = \"mine\"\nlocal = true\n\n"+
		"[[daemon]]\nname = \"alpha\"\nurl = \""+alpha.URL+"\"\nallow_insecure = true\n\n"+
		"[[daemon]]\nname = \"beta\"\nurl = \"https://kata.example.com:7474/base\"\ntoken_env = \"BETA_TOKEN\"\n\n"+
		"[[daemon]]\nname = \"inline\"\nurl = \"https://inline.example.com\"\ntoken = \"secret\"\n")
	gateway := newGateway(t, newLocator(alpha.target()))

	got := as[model.DaemonsLoaded](t, run(t, gateway, model.LoadDaemons{Request: 4}))
	want := []model.DaemonEntry{
		{Name: "mine", Endpoint: "local"},
		{Name: "alpha", Endpoint: strings.TrimPrefix(alpha.URL, "http://"), AllowInsecure: true},
		{Name: "beta", Endpoint: "kata.example.com:7474", Token: true, Current: true},
		{Name: "inline", Endpoint: "inline.example.com", Token: true},
	}
	if got.Err != nil || got.Request != 4 || !slices.Equal(got.Entries, want) || got.ConfigPath != filepath.Join(home, "config.toml") {
		t.Fatalf("daemons %+v, want %+v from %s", got, want, filepath.Join(home, "config.toml"))
	}

	t.Setenv("KATA_SERVER", alpha.URL)
	for _, e := range as[model.DaemonsLoaded](t, run(t, gateway, model.LoadDaemons{Request: 5})).Entries {
		if e.Current {
			t.Errorf("%s is current although KATA_SERVER names another daemon", e.Name)
		}
	}
}

func TestGatewayNamesTheDaemonAWorkspacePinSelectsNotTheActiveOne(t *testing.T) {
	beta := startKata(t, "0.20.0")
	t.Setenv("ALPHA_TOKEN", "alpha-secret")
	writeCatalog(t, "active_daemon = \"alpha\"\n\n"+
		"[[daemon]]\nname = \"alpha\"\nurl = \"https://alpha.example.com\"\ntoken_env = \"ALPHA_TOKEN\"\n\n"+
		"[[daemon]]\nname = \"beta\"\nurl = \""+beta.URL+"\"\nallow_insecure = true\n")
	gateway := newGateway(t, newLocator(beta.target()))
	workspace, _ := project(t, gateway, "demo")
	gateway.mu.Lock()
	clear(gateway.clients)
	gateway.mu.Unlock()
	for name, pin := range map[string]string{
		"daemon": "daemon = \"beta\"",
		"url":    "url = \"" + beta.URL + "/\"",
		"other":  "url = \"https://elsewhere.example.com\"",
	} {
		pinned := "version = 1\n[server]\n" + pin + "\n"
		if err := os.WriteFile(filepath.Join(workspace, ".kata.local.toml"), []byte(pinned), 0o600); err != nil {
			t.Fatal(err)
		}
		gateway.mu.Lock()
		clear(gateway.clients)
		gateway.mu.Unlock()
		loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
		want := "beta"
		if name == "other" {
			want = "" // the pin matches no entry, so no entry's token may follow it
		}
		if loaded.Daemon.Name != want {
			t.Fatalf("%s pin: loaded %+v, want the connection named %q, never the active alpha", name, loaded, want)
		}
		c, err := gateway.client(context.Background(), workspace)
		if err != nil || c.entry.Name != want || c.entry.TokenEnv != "" {
			t.Fatalf("%s pin: entry %+v err %v, want %q's entry", name, c.entry, err, want)
		}
	}
}

func TestGatewayDaemonsWithoutACatalogIsEmptyAndNamesTheFile(t *testing.T) {
	alpha := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(alpha.target()))
	got := as[model.DaemonsLoaded](t, run(t, gateway, model.LoadDaemons{Request: 1}))
	if got.Err != nil || len(got.Entries) != 0 || got.ConfigPath != filepath.Join(os.Getenv("KATA_HOME"), "config.toml") {
		t.Fatalf("daemons %+v", got)
	}

	writeCatalog(t, "[[daemon]]\nname = \"broken\"\n")
	if bad := as[model.DaemonsLoaded](t, run(t, gateway, model.LoadDaemons{Request: 2})); bad.Err == nil || !strings.Contains(bad.Err.Error(), "exactly one of local or url") {
		t.Fatalf("a catalog kata would refuse answered %+v", bad)
	}
}

func TestGatewaySwitchEndsAndAwaitsTheOldDaemonsPointerPass(t *testing.T) {
	f := newAttachFixture(t)
	beta := startKata(t, "0.20.0")
	writeCatalog(t, remoteEntry("alpha", f.kata)+remoteEntry("beta", beta))
	f.setMeta(t, f.uid, map[string]any{"attachment.a.png": map[string]any{"host": f.host, "path": "/a.png"}})

	listing, release := make(chan struct{}, 1), make(chan struct{})
	var open sync.Once
	t.Cleanup(func() { open.Do(func() { close(release) }) })
	hold := func(_ http.ResponseWriter, r *http.Request) bool {
		if !isIssueList(r) {
			return false
		}
		select {
		case listing <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return false
		case <-r.Context().Done():
			return true
		}
	}
	f.kata.intercept.Store(&hold)

	as[model.ProjectLoaded](t, run(t, f.gateway, model.LoadProject{Workspace: f.repo, Actor: "tester"}))
	<-listing // the pass is reading the old daemon's Issues

	if answer := switched(t, f.gateway, f.repo, "beta"); answer.Err != nil {
		t.Fatal(answer.Err)
	}
	ended := make(chan struct{})
	go func() { f.gateway.background.Wait(); close(ended) }()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("the switch answered while the old daemon's pointer pass was still running")
	}
	open.Do(func() { close(release) })
	if _, moved := f.issue(t, f.uid).Metadata["tern-kata.attachment.a.png"]; moved {
		t.Fatal("the old daemon's pointers were rewritten after the switch")
	}
}

func TestWorkspacePinFollowsKatasRules(t *testing.T) {
	t.Setenv("KATA_SERVER", "")
	write := func(dir, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, ".kata.local.toml"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const pin = "version = 1\n[server]\ndaemon = \"beta\"\n"
	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	t.Run("a pin above the workspace boundary is ignored", func(t *testing.T) {
		parent := t.TempDir()
		write(parent, pin)
		workspace := filepath.Join(parent, "ws")
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, ".kata.toml"), []byte("version = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := workspacePin(workspace); err != nil || got.Daemon != "" || got.URL != "" {
			t.Fatalf("pin %+v err %v, want none from above the boundary", got, err)
		}
	})
	t.Run("a pin at the boundary is read", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, ".kata.toml"), []byte("version = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		write(workspace, pin)
		if got, err := workspacePin(workspace); err != nil || got.Daemon != "beta" {
			t.Fatalf("pin %+v err %v, want beta", got, err)
		}
	})
	t.Run("a git-tracked pin is ignored", func(t *testing.T) {
		repo := t.TempDir()
		git(repo, "init", "-q")
		write(repo, pin)
		if got, err := workspacePin(repo); err != nil || got.Daemon != "beta" {
			t.Fatalf("untracked pin %+v err %v, want beta", got, err)
		}
		git(repo, "add", "-f", ".kata.local.toml")
		git(repo, "commit", "-q", "-m", "x")
		if got, err := workspacePin(repo); err != nil || got.Daemon != "" || got.URL != "" {
			t.Fatalf("tracked pin %+v err %v, want none", got, err)
		}
	})
	t.Run("a malformed pin is an error that names no daemon", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, ".kata.toml"), []byte("version = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		write(workspace, "version = 1\n[server\n")
		if _, err := workspacePin(workspace); err == nil {
			t.Fatal("malformed pin read without error")
		}
		catalog := Catalog{Active: "alpha", Daemons: []CatalogEntry{{Name: "alpha", URL: "https://alpha.example.com"}}}
		if name, _ := (&Gateway{}).resolvedEntry(workspace, catalog); name != "" {
			t.Fatalf("named %q, want the active alpha never chosen for a pin kata would refuse", name)
		}
	})
	t.Run("urls match in kata's canonical form", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, ".kata.toml"), []byte("version = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		write(workspace, "version = 1\n[server]\nurl = \"HTTPS://Beta.Example.com:443/\"\n")
		catalog := Catalog{Active: "alpha", Daemons: []CatalogEntry{
			{Name: "alpha", URL: "https://alpha.example.com"},
			{Name: "beta", URL: "https://beta.example.com"},
		}}
		if name, _ := (&Gateway{}).resolvedEntry(workspace, catalog); name != "beta" {
			t.Fatalf("named %q, want beta", name)
		}
	})
}
