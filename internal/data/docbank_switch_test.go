package data

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const pinnedKataConfig = `# kata's config
[tui]
confirm_quit = false # asked once

[docbank]
url = "http://127.0.0.1:8080"
api_key_file = "/home/me/.kata/docbank.key"
folder = "/files/kata"
# belongs to the next table

[docbank.extra]
keep = true

[[daemon]]
name = "other"
`

const pinnedKataConfigWithout = `# kata's config
[tui]
confirm_quit = false # asked once

# belongs to the next table

[docbank.extra]
keep = true

[[daemon]]
name = "other"
`

// docbankSwitchFixture is a default kata daemon home holding config, a Docbank home whose api_port is the one config pins, and a
// fake kata on PATH. The fake records its arguments and KATA_HOME, the config.toml it found, and then runs script.
type docbankSwitchFixture struct {
	g          *Gateway
	configPath string
	log        string
}

func newDocbankSwitchFixture(t *testing.T, config, script string) *docbankSwitchFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake kata is a shell script")
	}
	g, _ := setupGateway(t)
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	docbankHome := t.TempDir()
	t.Setenv("DOCBANK_HOME", docbankHome)
	if err := os.WriteFile(filepath.Join(docbankHome, "config.toml"), []byte("[server]\napi_port = 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &docbankSwitchFixture{g: g, configPath: filepath.Join(home, "config.toml"), log: filepath.Join(t.TempDir(), "kata.log")}
	writeCatalog(t, config)
	bin := t.TempDir()
	body := "#!/bin/sh\necho \"$@\" >> '" + f.log + ".args'\necho \"$KATA_HOME\" >> '" + f.log + ".home'\ncat \"$KATA_HOME/config.toml\" > '" + f.log + ".seen'\n" + script
	if err := os.WriteFile(filepath.Join(bin, "kata"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

func (f *docbankSwitchFixture) logged(t *testing.T, suffix string) string {
	t.Helper()
	raw, err := os.ReadFile(f.log + suffix)
	if err != nil {
		t.Fatalf("kata did not run: %v", err)
	}
	return string(raw)
}

func (f *docbankSwitchFixture) config(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSwitchDocbankRemovesOnlyTheDocbankTableBeforeKataRunsWithTheOldFolderAndTheDefaultHome(t *testing.T) {
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "exit 0\n")
	got := as[model.DocbankSwitched](t, run(t, f.g, model.SwitchDocbank{}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if seen := f.logged(t, ".seen"); seen != pinnedKataConfigWithout {
		t.Errorf("kata found config.toml\n%q\nwant\n%q", seen, pinnedKataConfigWithout)
	}
	if args := strings.TrimSpace(f.logged(t, ".args")); args != "docbank setup --yes --folder /files/kata" {
		t.Errorf("kata ran with %q", args)
	}
	if home := strings.TrimSpace(f.logged(t, ".home")); home != os.Getenv("KATA_HOME") {
		t.Errorf("kata ran with KATA_HOME %q, want %q", home, os.Getenv("KATA_HOME"))
	}
	if after := f.config(t); after != pinnedKataConfigWithout {
		t.Errorf("config.toml after the switch is\n%q", after)
	}
}

func TestSwitchDocbankWithoutAFolderUsesKatasDefault(t *testing.T) {
	config := strings.Replace(pinnedKataConfig, "folder = \"/files/kata\"\n", "", 1)
	f := newDocbankSwitchFixture(t, config, "exit 0\n")
	if got := as[model.DocbankSwitched](t, run(t, f.g, model.SwitchDocbank{})); got.Err != nil {
		t.Fatal(got.Err)
	}
	if args := strings.TrimSpace(f.logged(t, ".args")); args != "docbank setup --yes --folder /kata" {
		t.Errorf("kata ran with %q", args)
	}
}

func TestSwitchDocbankRemovesTheTableAtTheEndOfTheFileAndKeepsCRLF(t *testing.T) {
	config := "[tui]\r\nconfirm_quit = false\r\n\r\n[docbank]\r\nurl = \"http://127.0.0.1:8080\"\r\nfolder = \"/kata\"\r\n"
	got, ok := withoutDocbankTable(config)
	if !ok || got != "[tui]\r\nconfirm_quit = false\r\n\r\n" {
		t.Errorf("got %q (found %v)", got, ok)
	}
	if got, ok := withoutDocbankTable("[tui]\nx = 1\n"); ok || got != "[tui]\nx = 1\n" {
		t.Errorf("a file without the table gave %q (found %v)", got, ok)
	}
}

func TestSwitchDocbankKataFailingPutsTheOriginalBytesBackAndReturnsKatasLine(t *testing.T) {
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "echo 'starting' >&2\necho 'docbank is not installed' >&2\necho >&2\nexit 3\n")
	got := as[model.DocbankSwitched](t, run(t, f.g, model.SwitchDocbank{}))
	if got.Err == nil || got.Err.Error() != "docbank is not installed" {
		t.Errorf("error %v, want kata's last line only", got.Err)
	}
	if after := f.config(t); after != pinnedKataConfig {
		t.Errorf("config.toml is\n%q\nwant the original bytes", after)
	}
	if seen := f.logged(t, ".seen"); seen != pinnedKataConfigWithout {
		t.Errorf("kata did not run on the switched file: %q", seen)
	}
}

func TestSwitchDocbankLeavesAFileChangedAfterTheSwitchAndSaysItWasNotRestored(t *testing.T) {
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "echo 'active_daemon = \"x\"' >> \"$KATA_HOME/config.toml\"\necho 'kata failed' >&2\nexit 1\n")
	got := as[model.DocbankSwitched](t, run(t, f.g, model.SwitchDocbank{}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "kata failed") || !strings.Contains(got.Err.Error(), "was not restored because it changed after the switch wrote it, so it was left as it is") || strings.Contains(got.Err.Error(), "no longer has") {
		t.Errorf("error %v, want kata's line and that the file was not restored and why", got.Err)
	}
	if after, want := f.config(t), pinnedKataConfigWithout+"active_daemon = \"x\"\n"; after != want {
		t.Errorf("config.toml is\n%q\nwant the other writer's file\n%q", after, want)
	}
}

// waitForFile blocks until kata's stand-in has logged path, which is after it read the switched config.toml.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
			return
		}
	}
	t.Fatalf("kata did not start: %s is missing", path)
}

func TestSwitchDocbankKataNotAnsweringInTimeIsKilledAndTheOriginalBytesComeBack(t *testing.T) {
	old := kataRunTimeout
	kataRunTimeout = 200 * time.Millisecond
	t.Cleanup(func() { kataRunTimeout = old })
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "exec sleep 30\n")
	start := time.Now()
	got := as[model.DocbankSwitched](t, run(t, f.g, model.SwitchDocbank{}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "kata did not answer within 200ms") {
		t.Errorf("error %v, want that kata did not answer in time", got.Err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("the switch took %v; kata was not killed", time.Since(start))
	}
	if after := f.config(t); after != pinnedKataConfig {
		t.Errorf("config.toml is\n%q\nwant the original bytes", after)
	}
}

func TestSwitchDocbankReturnsOnlyAfterTheRestoreWhenThePanesContextIsCancelled(t *testing.T) {
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "sleep 1\necho 'kata failed' >&2\nexit 1\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	answer := make(chan model.Msg, 1)
	go func() { answer <- f.g.switchDocbank(ctx, model.SwitchDocbank{}) }()
	waitForFile(t, f.log+".seen")
	cancel()
	got := as[model.DocbankSwitched](t, <-answer)
	if got.Err == nil || got.Err.Error() != "kata failed" {
		t.Errorf("error %v, want kata's own line: the cancelled Pane does not kill it", got.Err)
	}
	if after := f.config(t); after != pinnedKataConfig {
		t.Errorf("config.toml is\n%q\nwant the original bytes", after)
	}
}

func TestGatewayCloseWaitsForASwitchInFlightToRestoreTheFile(t *testing.T) {
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "sleep 1\necho 'kata failed' >&2\nexit 1\n")
	answer := make(chan model.Msg, 1)
	go func() { answer <- f.g.switchDocbank(context.Background(), model.SwitchDocbank{}) }()
	waitForFile(t, f.log+".seen")
	f.g.Close()
	if after := f.config(t); after != pinnedKataConfig {
		t.Errorf("Close returned with config.toml\n%q\nwant the original bytes already back", after)
	}
	<-answer
	if got := as[model.DocbankSwitched](t, f.g.switchDocbank(context.Background(), model.SwitchDocbank{})); got.Err == nil {
		t.Error("a switch after Close ran; want it refused")
	}
}

func TestSwitchDocbankRefusesAnyOtherForm(t *testing.T) {
	f := newDocbankSwitchFixture(t, "[docbank]\nlocal = true\n", "exit 0\n")
	got := as[model.DocbankSwitched](t, run(t, f.g, model.SwitchDocbank{}))
	if got.Err == nil || f.config(t) != "[docbank]\nlocal = true\n" {
		t.Errorf("error %v, config %q; want a refusal and no change", got.Err, f.config(t))
	}
	if _, err := os.Stat(f.log + ".args"); err == nil {
		t.Error("kata ran")
	}
}

func TestWithoutDocbankTableSeesAHeaderBehindAByteOrderMarkAndKeepsTheMark(t *testing.T) {
	got, ok := withoutDocbankTable("\uFEFF[docbank]\nurl = \"http://127.0.0.1:8080\"\n\n[tui]\nconfirm_quit = false\n")
	if want := "\uFEFF\n[tui]\nconfirm_quit = false\n"; !ok || got != want {
		t.Errorf("got %q, %v; want %q", got, ok, want)
	}
	if got, ok := withoutDocbankTable("\uFEFF[tui]\n"); ok || got != "\uFEFF[tui]\n" {
		t.Errorf("no header: got %q, %v", got, ok)
	}
}

// A value spanning lines that holds a header-looking line makes the line cut leave a broken file, so the decode check refuses it.
func TestWithoutDocbankTableRefusesACutThatChangesAnythingElse(t *testing.T) {
	hidden := "[docbank]\nurl = \"http://127.0.0.1:8080\"\nnotes = \"\"\"\n[tui]\nnot a table\n\"\"\"\n\n[other]\nx = 1\n"
	if got, ok := withoutDocbankTable(hidden); ok || got != hidden {
		t.Errorf("got %q, %v; want the text unchanged and a refusal", got, ok)
	}
	inline := "[docbank]\nurl = \"u\"\nextra = { a = 1 }\n"
	if got, ok := withoutDocbankTable(inline); ok || got != inline {
		t.Errorf("inline table: got %q, %v; want a refusal", got, ok)
	}
}

func TestConfigFilesDocbankRowOffersTheSwitchOnlyInTheOldPinnedForm(t *testing.T) {
	f := newDocbankSwitchFixture(t, pinnedKataConfig, "exit 0\n")
	row := func() model.ConfigFile {
		return as[model.ConfigFilesLoaded](t, run(t, f.g, model.LoadConfigFiles{Request: 1})).Files[6]
	}
	if r := row(); r.Key != model.ConfigDocbank || !r.Has(model.ConfigSwitchDocbank) {
		t.Errorf("pinned row %+v lacks the switch", r)
	}
	for _, other := range []string{"[docbank]\nlocal = true\n", "[docbank]\nurl = \"https://docbank.example\"\n", ""} {
		if err := os.WriteFile(f.configPath, []byte(other), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := row(); len(r.Actions) != 0 {
			t.Errorf("%q: row %+v offers %v", other, r, r.Actions)
		}
	}
}
