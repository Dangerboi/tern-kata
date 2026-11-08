//go:build !windows

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/data/docbanktest"
	"github.com/Dangerboi/tern-kata/internal/docbanksetup"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// docbankSetupFixture is a kata home with a fake `kata` on PATH that records its arguments, and a fake Docbank that checks the key.
type docbankSetupFixture struct {
	home    string
	kataLog string
	fake    *docbanktest.Fake
	locate  data.Locator
	confirm func(string) (bool, error)
	// runner replaces the real program runner when set.
	runner docbanksetup.Runner
	// probeTimeout replaces the probe's deadline when set.
	probeTimeout time.Duration
}

// newDocbankSetupFixture makes the fixture on a kata daemon that already stores attachments, so the final check passes; use
// withoutAttachments for a daemon whose Docbank is off.
func newDocbankSetupFixture(t *testing.T) *docbankSetupFixture {
	t.Helper()
	kataDaemon := newDocbankSubcommandFixture(t)
	return newDocbankSetupFixtureOn(t, kataDaemon.locate)
}

func newDocbankSetupFixtureOn(t *testing.T, locate data.Locator) *docbankSetupFixture {
	t.Helper()
	f := &docbankSetupFixture{home: t.TempDir(), fake: docbanktest.New(t), locate: locate}
	t.Setenv("KATA_HOME", f.home)
	t.Setenv("KATA_DB", "")
	isolateConfig(t) // tern-kata's config folder is where the supervision marker would be
	t.Setenv("KATA_POSTGRES_SCHEMA", "")
	bin := t.TempDir()
	f.kataLog = filepath.Join(t.TempDir(), "kata.log")
	script := "#!/bin/sh\necho \"$@\" >> '" + f.kataLog + "'\necho \"$KATA_HOME\" >> '" + f.kataLog + ".home'\n"
	if err := os.WriteFile(filepath.Join(bin, "kata"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return f
}

// run is `tern-kata docbank setup args...` with stdin as given, through the parser and the runner; the environment is the
// production one except for the daemon the check reaches.
func (f *docbankSetupFixture) run(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	return f.runWith(context.Background(), t, stdin, args...)
}

func (f *docbankSetupFixture) runWith(ctx context.Context, t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	plan, err := PlanLaunch(append([]string{"docbank", "setup"}, args...), func(string) (string, bool) { return "robot", true }, "test")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	env, err := newDocbankEnv(plan, strings.NewReader(stdin), &out)
	if err != nil {
		return out.String(), err
	}
	env.Locate = f.locate
	env.ProbeTimeout = f.probeTimeout
	if f.confirm != nil {
		env.Confirm = f.confirm
	}
	if f.runner != nil {
		env.Runner = f.runner
	}
	err = runDocbank(ctx, plan, env)
	return out.String(), err
}

func (f *docbankSetupFixture) restarts(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(f.kataLog)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

// kataHomes are the KATA_HOME values the fake kata ran with.
func (f *docbankSetupFixture) kataHomes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(f.kataLog + ".home")
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (f *docbankSetupFixture) keyFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(docbanktest.Key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f *docbankSetupFixture) config(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.home, "config.toml"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f *docbankSetupFixture) writeConfig(t *testing.T, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.home, "config.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDocbankSetupPointsKataAtARemoteDocbankAndChecksIt(t *testing.T) {
	f := newDocbankSetupFixture(t)
	key := f.keyFile(t)
	out, err := f.run(t, "", "--url", f.fake.URL+"/", "--key-file", key, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	want := "[docbank]\nurl = \"" + f.fake.URL + "\"\napi_key_file = \"" + key + "\"\nfolder = \"/kata\"\n"
	if got := f.config(t); got != want {
		t.Errorf("config.toml = %q, want %q", got, want)
	}
	info, err := os.Stat(filepath.Join(f.home, "config.toml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("config.toml mode = %v, %v; want 0600", info, err)
	}
	if got := f.restarts(t); len(got) != 1 || got[0] != "daemon restart" {
		t.Errorf("kata ran %q, want one `daemon restart`", got)
	}
	for _, line := range []string{filepath.Join(f.home, "config.toml"), f.fake.URL, "Restarted the kata daemon", "Checked"} {
		if !strings.Contains(out, line) {
			t.Errorf("output lacks %q:\n%s", line, out)
		}
	}
	if strings.Contains(out, docbanktest.Key) {
		t.Errorf("output shows the key:\n%s", out)
	}
}

func TestDocbankSetupLeavesTheRestOfKatasConfigAsItWas(t *testing.T) {
	f := newDocbankSetupFixture(t)
	existing := "# my kata\nactive_daemon = \"\"   # none\n\n[tui]\nconfirm_quit = false # quiet"
	f.writeConfig(t, existing)
	if _, err := f.run(t, "", "--url", f.fake.URL, "--key-env", "DOCBANK_KEY_FOR_TEST", "--no-restart"); err == nil {
		t.Fatal("an unset --key-env variable was accepted")
	}
	if f.config(t) != existing {
		t.Fatalf("a refused setup changed config.toml:\n%s", f.config(t))
	}
	t.Setenv("DOCBANK_KEY_FOR_TEST", docbanktest.Key)
	out, err := f.run(t, "", "--url", f.fake.URL, "--key-env", "DOCBANK_KEY_FOR_TEST", "--folder", "/files/kata", "--no-restart")
	if err != nil {
		t.Fatal(err)
	}
	got := f.config(t)
	if !strings.HasPrefix(got, existing+"\n\n[docbank]\n") || !strings.Contains(got, "api_key_env = \"DOCBANK_KEY_FOR_TEST\"") || !strings.Contains(got, "folder = \"/files/kata\"") {
		t.Errorf("config.toml = %q", got)
	}
	info, err := os.Stat(filepath.Join(f.home, "config.toml"))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("an edited config.toml has mode %v, %v; want its own 0644", info, err)
	}
	if !strings.Contains(out, "$DOCBANK_KEY_FOR_TEST") {
		t.Errorf("output does not say the daemon reads the variable:\n%s", out)
	}
}

func TestDocbankSetupRunTwiceSaysAlreadySetUp(t *testing.T) {
	f := newDocbankSetupFixture(t)
	key := f.keyFile(t)
	args := []string{"--url", f.fake.URL, "--key-file", key, "--yes"}
	if _, err := f.run(t, "", args...); err != nil {
		t.Fatal(err)
	}
	before := f.config(t)
	f.fake.Stop() // a rerun that changes nothing asks nothing of Docbank
	out, err := f.run(t, "", args...)
	if err != nil || !strings.Contains(out, "already set up") {
		t.Fatalf("rerun: %q, %v", out, err)
	}
	if f.config(t) != before || len(f.restarts(t)) != 1 {
		t.Errorf("a rerun changed something: config %q, kata ran %q", f.config(t), f.restarts(t))
	}
}

func TestDocbankSetupRefusesADifferentDocbankSectionAndShowsIt(t *testing.T) {
	f := newDocbankSetupFixture(t)
	existing := "[docbank]\nurl = \"https://other.example\"\napi_key = \"hunter2-secret\"\n"
	f.writeConfig(t, existing)
	_, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "https://other.example") {
		t.Fatalf("err = %v, want a refusal that shows the section", err)
	}
	if strings.Contains(err.Error(), "hunter2-secret") {
		t.Errorf("the refusal shows the inline key: %v", err)
	}
	if f.config(t) != existing || f.restarts(t) != nil {
		t.Errorf("a refusal changed something: config %q, kata ran %q", f.config(t), f.restarts(t))
	}
	if got := exitStatus(err); got != 1 {
		t.Errorf("exit %d, want 1", got)
	}
}

func TestDocbankSetupWritesNothingWhenDocbankRefusesOrDoesNotAnswer(t *testing.T) {
	f := newDocbankSetupFixture(t)
	wrong := filepath.Join(t.TempDir(), "wrong")
	if err := os.WriteFile(wrong, []byte("not-the-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "", "--url", f.fake.URL, "--key-file", wrong, "--yes"); err == nil || err.Error() != data.DocbankUnauthorizedMessage {
		t.Errorf("a refused key: err = %v, want the Pane's words %q", err, data.DocbankUnauthorizedMessage)
	}
	if _, err := f.run(t, "not-the-key\n", "--url", f.fake.URL, "--key-file", "-", "--yes"); err == nil || err.Error() != data.DocbankUnauthorizedMessage {
		t.Errorf("a refused key from stdin: err = %v", err)
	}
	f.fake.Stop()
	if _, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--yes"); err == nil || err.Error() != data.DocbankUnreachableMessage {
		t.Errorf("no answer: err = %v, want the Pane's words %q", err, data.DocbankUnreachableMessage)
	}
	entries, err := os.ReadDir(f.home)
	if err != nil || len(entries) != 0 || f.restarts(t) != nil {
		t.Errorf("a refused setup wrote %v (%v) and ran kata %q", entries, err, f.restarts(t))
	}
}

func TestDocbankSetupRefusesPlainHTTPToAnotherHostBeforeWriting(t *testing.T) {
	f := newDocbankSetupFixture(t)
	_, err := f.run(t, "", "--url", "http://docbank.example", "--key-file", f.keyFile(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("err = %v, want the HTTPS rule", err)
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 {
		t.Errorf("wrote %v", entries)
	}
}

func TestDocbankSetupStoresAKeyFromStandardInputPrivately(t *testing.T) {
	f := newDocbankSetupFixture(t)
	out, err := f.run(t, docbanktest.Key+"\n", "--url", f.fake.URL, "--key-file", "-", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(f.home, "docbank.key")
	raw, err := os.ReadFile(keyPath)
	if err != nil || strings.TrimSpace(string(raw)) != docbanktest.Key {
		t.Fatalf("key file = %q, %v", raw, err)
	}
	if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("key file mode = %v, %v; want 0600", info, err)
	}
	if !strings.Contains(f.config(t), "api_key_file = \""+keyPath+"\"") {
		t.Errorf("config.toml does not name the key file:\n%s", f.config(t))
	}
	if strings.Contains(out, docbanktest.Key) || !strings.Contains(out, keyPath) {
		t.Errorf("output: %s", out)
	}
}

func TestDocbankSetupRerunWritesAgainAStoredKeyFileKataWouldRefuse(t *testing.T) {
	f := newDocbankSetupFixture(t)
	args := []string{"--url", f.fake.URL, "--key-file", "-", "--yes"}
	if _, err := f.run(t, docbanktest.Key+"\n", args...); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(f.home, "docbank.key")
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t, docbanktest.Key+"\n", args...)
	if err != nil || !strings.Contains(out, "kata would have refused the API key file "+keyPath+" (group/world-readable") {
		t.Fatalf("rerun: %q, %v", out, err)
	}
	if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("key file mode = %v, %v; want 0600", info, err)
	}
	if raw, err := os.ReadFile(keyPath); err != nil || strings.TrimSpace(string(raw)) != docbanktest.Key {
		t.Errorf("key file = %q, %v; want the key it held", raw, err)
	}
}

func TestDocbankSetupRestartsOnlyWhenAskedOrConfirmed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     []string
		confirm   func(string) (bool, error)
		restarted bool
	}{
		{"no terminal and no --yes", nil, nil, false},
		{"--no-restart", []string{"--no-restart"}, func(string) (bool, error) { return true, nil }, false},
		{"a yes on the terminal", nil, func(string) (bool, error) { return true, nil }, true},
		{"a no on the terminal", nil, func(string) (bool, error) { return false, nil }, false},
		{"--yes", []string{"--yes"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDocbankSetupFixture(t)
			f.confirm = tc.confirm
			out, err := f.run(t, "", append([]string{"--url", f.fake.URL, "--key-file", f.keyFile(t)}, tc.flags...)...)
			if err != nil {
				t.Fatal(err)
			}
			if ran := len(f.restarts(t)) == 1; ran != tc.restarted {
				t.Errorf("restarted = %v, want %v", ran, tc.restarted)
			}
			if !tc.restarted && !strings.Contains(out, "kata daemon restart") {
				t.Errorf("output does not print the command to run:\n%s", out)
			}
		})
	}
}

func TestDocbankSetupReportsAKataThatHasNoDocbankInThePanesWords(t *testing.T) {
	f := newDocbankSetupFixtureOn(t, newSubcommandFixture(t).locate)
	_, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), model.DocbankOffMessage) {
		t.Fatalf("err = %v, want the Pane's %q", err, model.DocbankOffMessage)
	}
	if len(f.restarts(t)) != 1 {
		t.Errorf("kata ran %q", f.restarts(t))
	}
}

func TestDocbankSetupRefusesBeforeWritingWithoutKataOnPath(t *testing.T) {
	f := newDocbankSetupFixture(t)
	t.Setenv("PATH", t.TempDir())
	_, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "kata is not on PATH") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 {
		t.Errorf("wrote %v", entries)
	}
}

// profileCatalog writes a kata config that lists a local profile with its own home and a remote entry.
func (f *docbankSetupFixture) profileCatalog(t *testing.T) (profileHome string) {
	t.Helper()
	profileHome = t.TempDir()
	f.writeConfig(t, "[[daemon]]\nname = \"side\"\nlocal = true\nhome = \""+profileHome+"\"\ninstance_uid = \"01ARZ3NDEKTSV4RRFFQ69G5FAV\"\n\n[[daemon]]\nname = \"far\"\nurl = \"https://kata.example\"\n")
	return profileHome
}

func TestDocbankSetupWritesTheChosenLocalProfilesConfigAndRestartsThatProfile(t *testing.T) {
	f := newDocbankSetupFixture(t)
	profileHome := f.profileCatalog(t)
	catalog := f.config(t)
	if _, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--daemon", "side", "--yes"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(profileHome, "config.toml"))
	if err != nil || !strings.Contains(string(raw), "[docbank]") {
		t.Fatalf("profile config = %q, %v", raw, err)
	}
	if f.config(t) != catalog {
		t.Errorf("the default home's config changed:\n%s", f.config(t))
	}
	if got := f.restarts(t); len(got) != 1 || got[0] != "daemon restart" {
		t.Errorf("kata ran %q, want `daemon restart` (kata's daemon commands take no --daemon)", got)
	}
	if got := f.kataHomes(t); len(got) != 1 || got[0] != profileHome {
		t.Errorf("kata ran with KATA_HOME %q, want the profile's home %q", got, profileHome)
	}

}

func TestDocbankSetupRefusesARemoteKataDaemon(t *testing.T) {
	f := newDocbankSetupFixture(t)
	f.profileCatalog(t)
	before := f.config(t)
	for name, flags := range map[string][]string{"a remote entry": {"--daemon", "far"}, "--server": {"--server", "https://kata.example"}} {
		_, err := f.run(t, "", append([]string{"--url", f.fake.URL, "--key-file", f.keyFile(t), "--yes"}, flags...)...)
		if err == nil || !strings.Contains(err.Error(), "remote kata daemon") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if f.config(t) != before || f.restarts(t) != nil {
		t.Errorf("a refusal changed something: config %q, kata ran %q", f.config(t), f.restarts(t))
	}
}

func TestDocbankSetupUsageErrorsExitTwo(t *testing.T) {
	for name, args := range map[string][]string{
		"no form":                  {"docbank"},
		"another form":             {"docbank", "start"},
		"no key with --url":        {"docbank", "setup", "--url", "https://d.example"},
		"--key-env without --url":  {"docbank", "setup", "--key-env", "K"},
		"--key-file without --url": {"docbank", "setup", "--key-file", "k"},
		"--port with --url":        {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--port", "8080"},
		"a port that is no number": {"docbank", "setup", "--port", "http"},
		"a port out of range":      {"docbank", "setup", "--port", "70000"},
		"both keys":                {"docbank", "setup", "--url", "https://d.example", "--key-file", "k", "--key-env", "K"},
		"a key as a flag":          {"docbank", "setup", "--url", "https://d.example", "--key", "secret"},
		"--yes with --no-restart":  {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--yes", "--no-restart"},
		"a value that is a flag":   {"docbank", "setup", "--url", "--key-env", "K"},
		"--yes with a value":       {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--yes=1"},
		"unknown flag":             {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--bogus", "1"},
	} {
		_, err := PlanLaunch(args, func(string) (string, bool) { return "robot", true }, "test")
		if err == nil || exitStatus(err) != 2 {
			t.Errorf("%s: err = %v, exit %d; want a usage error", name, err, exitStatus(err))
		}
	}
	plan, err := PlanLaunch([]string{"docbank", "setup", "--url=--x", "--key-env", "K"}, func(string) (string, bool) { return "robot", true }, "test")
	if err != nil || plan.Docbank.URL != "--x" {
		t.Errorf("--url=--x: %+v, %v", plan.Docbank, err)
	}
}

func TestDocbankSetupEditsTheFileASymlinkedConfigPointsAt(t *testing.T) {
	f := newDocbankSetupFixture(t)
	target := filepath.Join(t.TempDir(), "dotfiles", "kata.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("[tui]\nconfirm_quit = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.home, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--no-restart"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config.toml is no longer a symlink: %v, %v", info, err)
	}
	raw, err := os.ReadFile(target)
	if err != nil || !strings.HasPrefix(string(raw), "[tui]\nconfirm_quit = false\n\n[docbank]\n") {
		t.Errorf("target = %q, %v", raw, err)
	}
}

func TestDocbankSetupRefusesAKeyFromAStandardInputThatIsATerminal(t *testing.T) {
	f := newDocbankSetupFixture(t)
	old := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = old })
	_, err := f.run(t, "", "--url", f.fake.URL, "--key-file", "-", "--yes")
	if err == nil || exitStatus(err) != 2 || !strings.Contains(err.Error(), "pipe the key in") {
		t.Fatalf("err = %v, exit %d; want a usage error saying to pipe the key in", err, exitStatus(err))
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 {
		t.Errorf("wrote %v", entries)
	}
}

func TestDocbankSetupPrintsAProfilesRestartWithItsKataHome(t *testing.T) {
	f := newDocbankSetupFixture(t)
	profileHome := f.profileCatalog(t)
	out, err := f.run(t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--daemon", "side", "--no-restart")
	if err != nil || !strings.Contains(out, "KATA_HOME="+profileHome+" kata daemon restart") {
		t.Errorf("--no-restart: %q, %v; want the command with the profile's KATA_HOME", out, err)
	}
}

func TestDocbankSupervisionFormsParseAndRefuseSetupFlags(t *testing.T) {
	lookup := func(string) (string, bool) { return "robot", true }
	for _, args := range [][]string{{"docbank", "supervised"}, {"docbank", "supervise", "on"}, {"docbank", "supervise", "off"}} {
		if _, err := PlanLaunch(args, lookup, "test"); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"docbank", "supervise"}, {"docbank", "supervise", "maybe"}, {"docbank", "supervised", "--yes"}, {"docbank", "supervise", "on", "--home", "/x"}, {"docbank", "supervised", "--port", "9"}, {"docbank", "supervised", "--home", "/x"}, {"docbank", "supervised", "--daemon", "d"}, {"docbank", "supervised", "--db", "f"}, {"docbank", "supervised", "--schema", "s"}, {"docbank", "supervised", "--server", "http://x"}} {
		if _, err := PlanLaunch(args, lookup, "test"); err == nil {
			t.Errorf("%v: no error", args)
		} else if got := exitStatus(err); got != 2 {
			t.Errorf("%v: exit %d, want 2", args, got)
		}
	}
}

func TestDocbankSupervisedAnswersOnAndOffAndSuperviseWritesTheMarker(t *testing.T) {
	kata, dbHome, config := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "tern-kata")
	for path, text := range map[string]string{
		filepath.Join(kata, "config.toml"):   "[docbank]\nurl = \"http://127.0.0.1:8080\"\napi_key = \"k\"\n",
		filepath.Join(dbHome, "config.toml"): "[server]\napi_port = 8080\napi_key = \"k\"\n",
	} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := func(out *strings.Builder) docbanksetup.Env {
		return docbanksetup.Env{KataHome: kata, DefaultKataHome: kata, DocbankHome: dbHome, ConfigDir: config, Out: out}
	}
	var out strings.Builder
	if err := printDocbankSupervised(env(&out)); err != nil || out.String() != "on\n" {
		t.Fatalf("supervised = %q, %v; want on", out.String(), err)
	}
	out.Reset()
	plan := LaunchPlan{Subcommand: "docbank", Operands: []string{"supervise", "off"}}
	if err := runDocbankSupervise(plan, env(&out)); err != nil || !strings.Contains(out.String(), "Created ") {
		t.Fatalf("supervise off = %q, %v", out.String(), err)
	}
	out.Reset()
	err := printDocbankSupervised(env(&out))
	if exitStatus(err) != 1 || out.String() != "" {
		t.Fatalf("supervised with the marker = %q, %v; want exit 1 and no stdout", out.String(), err)
	}
	plan.Operands[1] = "on"
	out.Reset()
	if err := runDocbankSupervise(plan, env(&out)); err != nil || !strings.Contains(out.String(), "Removed ") {
		t.Fatalf("supervise on = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := printDocbankSupervised(env(&out)); err != nil || out.String() != "on\n" {
		t.Fatalf("supervised after on = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runDocbankSupervise(plan, env(&out)); err != nil || !strings.Contains(out.String(), "already on") || strings.Contains(out.String(), "Removed ") {
		t.Fatalf("supervise on twice = %q, %v; want it said it was already on", out.String(), err)
	}
	plan.Operands[1] = "off"
	out.Reset()
	if err := runDocbankSupervise(plan, env(&out)); err != nil || !strings.Contains(out.String(), "Created ") {
		t.Fatalf("supervise off = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runDocbankSupervise(plan, env(&out)); err != nil || !strings.Contains(out.String(), "already off") || strings.Contains(out.String(), "Created ") {
		t.Fatalf("supervise off twice = %q, %v; want it said it was already off", out.String(), err)
	}
}

// answeringServer serves answer for every request.
func answeringServer(t *testing.T, answer http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(answer)
	t.Cleanup(server.Close)
	return server.URL
}

func TestDocbankSetupRefusesARedirectAndNeverSendsTheKeyOn(t *testing.T) {
	f := newDocbankSetupFixture(t)
	url := answeringServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, f.fake.URL+r.URL.RequestURI(), http.StatusFound) // the target accepts the key, so following would succeed
	})
	_, err := f.run(t, "", "--url", url, "--key-file", f.keyFile(t), "--yes")
	if err == nil || err.Error() != data.DocbankUnreachableMessage {
		t.Fatalf("err = %v, want the Pane's %q", err, data.DocbankUnreachableMessage)
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 || f.restarts(t) != nil {
		t.Errorf("a refused setup wrote %v and ran kata %q", entries, f.restarts(t))
	}
}

func TestDocbankSetupRefusesAnAnswerThatIsNotADocbank(t *testing.T) {
	for name, body := range map[string]string{
		"html":              "<html>welcome</html>",
		"json without kind": `{"ok":true}`,
		"a file node":       `{"id":1,"kind":"file"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankSetupFixture(t)
			url := answeringServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			_, err := f.run(t, "", "--url", url, "--key-file", f.keyFile(t), "--yes")
			if err == nil || !strings.Contains(err.Error(), "not as a Docbank does") {
				t.Fatalf("err = %v, want the not-a-Docbank refusal", err)
			}
			if entries, _ := os.ReadDir(f.home); len(entries) != 0 || f.restarts(t) != nil {
				t.Errorf("a refused setup wrote %v and ran kata %q", entries, f.restarts(t))
			}
		})
	}
}

func TestDocbankSetupRerunWithAttachmentsOffOffersTheRestart(t *testing.T) {
	f := newDocbankSetupFixtureOn(t, newSubcommandFixture(t).locate) // a kata daemon that has no Docbank loaded
	args := []string{"--url", f.fake.URL, "--key-file", f.keyFile(t)}
	if _, err := f.run(t, "", append(args, "--no-restart")...); err != nil {
		t.Fatal(err)
	}
	before := f.config(t)
	out, err := f.run(t, "", append(args, "--no-restart")...)
	if err != nil || strings.Contains(out, "nothing changed") || !strings.Contains(out, "kata daemon restart") || f.restarts(t) != nil {
		t.Fatalf("rerun with --no-restart: %q, %v, kata ran %q; want the command to run and no restart", out, err, f.restarts(t))
	}
	out, err = f.run(t, "", append(args, "--yes")...)
	if got := f.restarts(t); len(got) != 1 || got[0] != "daemon restart" {
		t.Errorf("rerun with --yes: kata ran %q, want one `daemon restart`", got)
	}
	if err == nil || !strings.Contains(err.Error(), model.DocbankOffMessage) || strings.Contains(out, "nothing changed") {
		t.Errorf("rerun with --yes: %q, %v; want the check after the restart to report attachments off", out, err)
	}
	if f.config(t) != before {
		t.Errorf("a rerun changed config.toml:\n%s", f.config(t))
	}
}

func TestDocbankSetupCancelledContextWritesNothing(t *testing.T) {
	f := newDocbankSetupFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.runWith(ctx, t, "", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--yes")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 || f.restarts(t) != nil {
		t.Errorf("a cancelled setup wrote %v and ran kata %q", entries, f.restarts(t))
	}
}

func TestDocbankSetupReportsADocbankThatNeverAnswersAsUnreachable(t *testing.T) {
	f := newDocbankSetupFixture(t)
	f.probeTimeout = 100 * time.Millisecond
	url := answeringServer(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	_, err := f.run(t, "", "--url", url, "--key-file", f.keyFile(t), "--yes")
	if err == nil || err.Error() != data.DocbankUnreachableMessage {
		t.Fatalf("err = %v, want the Pane's %q", err, data.DocbankUnreachableMessage)
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 {
		t.Errorf("a refused setup wrote %v", entries)
	}
}

func TestDocbankSetupRerunSaysWhenTheKataDaemonCannotBeAsked(t *testing.T) {
	f := newDocbankSetupFixtureOn(t, func(context.Context, string) (data.Target, error) {
		return data.Target{}, errors.New("no daemon socket")
	})
	args := []string{"--url", f.fake.URL, "--key-file", f.keyFile(t), "--no-restart"}
	if _, err := f.run(t, "", args...); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t, "", args...)
	if err != nil || !strings.Contains(out, "could not be asked") || !strings.Contains(out, "no daemon socket") || !strings.Contains(out, "kata daemon restart") || strings.Contains(out, "nothing changed") {
		t.Fatalf("rerun: %q, %v", out, err)
	}
}

func TestDocbankSetupQuotesTheRestartCommandItPrints(t *testing.T) {
	f := newDocbankSetupFixture(t)
	home := filepath.Join(t.TempDir(), "my kata home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t, "", "--home", home, "--db", "it's a db", "--url", f.fake.URL, "--key-file", f.keyFile(t), "--no-restart")
	if err != nil {
		t.Fatal(err)
	}
	want := "KATA_HOME='" + home + "' KATA_DB='it'\\''s a db' kata daemon restart"
	if !strings.Contains(out, want) {
		t.Errorf("output does not print %q:\n%s", want, out)
	}
}

func TestDocbankSuperviseReportsAMarkerItCannotStat(t *testing.T) {
	config := t.TempDir()
	env := docbanksetup.Env{ConfigDir: config, Out: io.Discard}
	if err := os.Symlink(env.ManualPath(), env.ManualPath()); err != nil { // a loop: stat fails with something other than "not there"
		t.Skip("no symlinks here:", err)
	}
	var out strings.Builder
	env.Out = &out
	plan := LaunchPlan{Subcommand: "docbank", Operands: []string{"supervise", "on"}}
	if err := runDocbankSupervise(plan, env); err == nil || out.String() != "" {
		t.Fatalf("supervise on with an unstatable marker = %q, %v; want the stat error and no claim", out.String(), err)
	}
}
