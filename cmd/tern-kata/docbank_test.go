//go:build !windows

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/data/docbanktest"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// docbankSetupFixture is a kata home with a fake `kata` on PATH that records its arguments, and a fake Docbank that checks the key.
type docbankSetupFixture struct {
	home    string
	kataLog string
	fake    *docbanktest.Fake
	locate  data.Locator
	confirm func(string) (bool, error)
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
	t.Setenv("KATA_POSTGRES_SCHEMA", "")
	bin := t.TempDir()
	f.kataLog = filepath.Join(t.TempDir(), "kata.log")
	script := "#!/bin/sh\necho \"$@\" >> '" + f.kataLog + "'\n"
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
	if f.confirm != nil {
		env.Confirm = f.confirm
	}
	err = runDocbank(context.Background(), plan, env)
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
	if got := f.restarts(t); len(got) != 1 || got[0] != "--daemon side daemon restart" {
		t.Errorf("kata ran %q, want the profile's restart", got)
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
		"no form":                 {"docbank"},
		"another form":            {"docbank", "start"},
		"no --url":                {"docbank", "setup"},
		"no key":                  {"docbank", "setup", "--url", "https://d.example"},
		"both keys":               {"docbank", "setup", "--url", "https://d.example", "--key-file", "k", "--key-env", "K"},
		"a key as a flag":         {"docbank", "setup", "--url", "https://d.example", "--key", "secret"},
		"--yes with --no-restart": {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--yes", "--no-restart"},
		"a value that is a flag":  {"docbank", "setup", "--url", "--key-env", "K"},
		"--yes with a value":      {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--yes=1"},
		"unknown flag":            {"docbank", "setup", "--url", "https://d.example", "--key-env", "K", "--port", "1"},
	} {
		_, err := PlanLaunch(args, func(string) (string, bool) { return "robot", true }, "test")
		if err == nil || exitStatus(err) != 2 {
			t.Errorf("%s: err = %v, exit %d; want a usage error", name, err, exitStatus(err))
		}
	}
	plan, err := PlanLaunch([]string{"docbank", "setup", "--url=--x", "--key-env", "K"}, func(string) (string, bool) { return "robot", true }, "test")
	if err != nil || plan.DocbankFlags["url"] != "--x" {
		t.Errorf("--url=--x: %v, %v", plan.DocbankFlags, err)
	}
}
