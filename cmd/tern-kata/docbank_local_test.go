//go:build !windows

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/data/docbanktest"
	"github.com/Dangerboi/tern-kata/internal/docbanksetup"
)

// localFixture is a docbankSetupFixture with a fake `docbank` on PATH. Its `daemon restart` reads api_port from
// $DOCBANK_HOME/config.toml, as Docbank does, and `daemon status` then names that port; runner starts the fake HTTP Docbank on it.
type localFixture struct {
	*docbankSetupFixture
	dbHome, dbLog string
	started       *docbanktest.Fake
}

// hookRunner starts the fake Docbank after a successful `docbank daemon restart`, with the port and key its config now holds.
type hookRunner struct {
	docbanksetup.ExecRunner
	after func()
}

func (r hookRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	out, err := r.ExecRunner.Run(ctx, env, name, args...)
	if err == nil && name == "docbank" && len(args) == 2 && args[1] == "restart" {
		r.after()
	}
	return out, err
}

func newLocalFixture(t *testing.T) *localFixture {
	t.Helper()
	f := &localFixture{docbankSetupFixture: newDocbankSetupFixture(t)}
	f.dbHome = filepath.Join(t.TempDir(), "not", "yet", ".docbank") // the folder does not exist
	t.Setenv("DOCBANK_HOME", f.dbHome)
	t.Setenv("DOCBANK_FAKE_STATUS", "")
	f.dbLog = filepath.Join(t.TempDir(), "docbank.log")
	state := filepath.Join(t.TempDir(), "state")
	script := `#!/bin/sh
echo "$@" >> '` + f.dbLog + `'
case "$1 $2" in
"daemon restart")
  while IFS=' =' read -r k v; do [ "$k" = api_port ] && p=$v; done < "$DOCBANK_HOME/config.toml"
  echo "running: pid 4242 at 127.0.0.1:$p (0.17.0)" > '` + state + `'
  echo restarted ;;
"daemon status")
  if [ -n "$DOCBANK_FAKE_STATUS" ]; then echo "$DOCBANK_FAKE_STATUS"; exit 0; fi
  if [ -f '` + state + `' ]; then read -r l < '` + state + `'; echo "$l"; else echo "not running"; exit 1; fi ;;
esac
`
	if err := os.WriteFile(filepath.Join(os.Getenv("PATH"), "docbank"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	f.runner = hookRunner{after: func() {
		var cfg struct {
			Server struct {
				Port int    `toml:"api_port"`
				Key  string `toml:"api_key"`
			} `toml:"server"`
		}
		if _, err := toml.DecodeFile(filepath.Join(f.dbHome, "config.toml"), &cfg); err != nil {
			t.Fatal(err)
		}
		f.started = docbanktest.NewOn(t, "127.0.0.1:"+strconv.Itoa(cfg.Server.Port), cfg.Server.Key)
	}}
	return f
}

func (f *localFixture) docbankRuns(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(f.dbLog)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (f *localFixture) docbankConfig(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.dbHome, "config.toml"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f *localFixture) writeDocbankConfig(t *testing.T, text string) {
	t.Helper()
	if err := os.MkdirAll(f.dbHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dbHome, "config.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *localFixture) keyPath() string { return filepath.Join(f.home, "docbank.key") }

// assertNothingWritten checks that neither Docbank's home, the key file nor kata's config exists, and no program ran but status.
func (f *localFixture) assertNothingWritten(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(f.dbHome); err == nil {
		t.Errorf("Docbank's home was created")
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 {
		t.Errorf("kata's home holds %v", entries)
	}
	for _, run := range f.docbankRuns(t) {
		if strings.Contains(run, "restart") {
			t.Errorf("Docbank was restarted: %q", f.docbankRuns(t))
		}
	}
	if f.restarts(t) != nil {
		t.Errorf("kata ran %q", f.restarts(t))
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestDocbankSetupLocalSetsDocbankUpAndPointsKataAtIt(t *testing.T) {
	f := newLocalFixture(t)
	port := freePort(t)
	out, err := f.run(t, "", "--port", strconv.Itoa(port), "--yes")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	raw, err := os.ReadFile(f.keyPath())
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSpace(string(raw))
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(key) {
		t.Fatalf("key file holds %q, want 32 bytes as hex", key)
	}
	if info, err := os.Stat(f.keyPath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("key file mode = %v, %v; want 0600", info, err)
	}
	want := "[server]\nbind_addr = \"127.0.0.1\"\napi_port = " + strconv.Itoa(port) + "\napi_key = \"" + key + "\"\n"
	if got := f.docbankConfig(t); got != want {
		t.Errorf("Docbank config.toml = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(f.dbHome, "config.toml")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("Docbank config.toml mode = %v, %v; want 0600", info, err)
	}
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	wantKata := "[docbank]\nurl = \"" + url + "\"\napi_key_file = \"" + f.keyPath() + "\"\nfolder = \"/kata\"\n"
	if got := f.config(t); got != wantKata {
		t.Errorf("kata config.toml = %q, want %q", got, wantKata)
	}
	if got := strings.Join(f.docbankRuns(t), "|"); got != "daemon status|daemon restart|daemon status" {
		t.Errorf("docbank ran %q, want status, restart, status", got)
	}
	if got := f.restarts(t); len(got) != 1 || got[0] != "daemon restart" {
		t.Errorf("kata ran %q, want one `daemon restart`", got)
	}
	for _, line := range []string{"DOCBANK_TELEMETRY_ENABLED=0", "127.0.0.1:" + strconv.Itoa(port), "Restarted the kata daemon", "Checked"} {
		if !strings.Contains(out, line) {
			t.Errorf("output lacks %q:\n%s", line, out)
		}
	}
	if strings.Contains(out, key) {
		t.Errorf("output shows the key:\n%s", out)
	}
}

func TestDocbankSetupLocalKeepsEveryOtherByteOfDocbanksConfig(t *testing.T) {
	f := newLocalFixture(t)
	existing := "# mine\n[vault]\nname = \"x\" # keep"
	f.writeDocbankConfig(t, existing)
	if _, err := f.run(t, "", "--port", strconv.Itoa(freePort(t)), "--yes"); err != nil {
		t.Fatal(err)
	}
	got := f.docbankConfig(t)
	if !strings.HasPrefix(got, existing+"\n\n[server]\n") {
		t.Errorf("config.toml = %q", got)
	}
	if info, err := os.Stat(filepath.Join(f.dbHome, "config.toml")); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("an edited config.toml has mode %v, %v; want its own 0644", info, err)
	}
}

func TestDocbankSetupLocalRerunSaysAlreadySetUpAndRestartsNothing(t *testing.T) {
	f := newLocalFixture(t)
	args := []string{"--port", strconv.Itoa(freePort(t)), "--yes"}
	if _, err := f.run(t, "", args...); err != nil {
		t.Fatal(err)
	}
	dbBefore, kataBefore, docbankRuns := f.docbankConfig(t), f.config(t), len(f.docbankRuns(t))
	out, err := f.run(t, "", args...)
	if err != nil || !strings.Contains(out, "already set up") {
		t.Fatalf("rerun: %q, %v", out, err)
	}
	if f.docbankConfig(t) != dbBefore || f.config(t) != kataBefore || len(f.docbankRuns(t)) != docbankRuns || len(f.restarts(t)) != 1 {
		t.Errorf("a rerun changed something: docbank ran %q, kata ran %q", f.docbankRuns(t), f.restarts(t))
	}
	// without --port too: the port is the one in Docbank's config
	if out, err := f.run(t, "", "--yes"); err != nil || !strings.Contains(out, "already set up") {
		t.Errorf("rerun without --port: %q, %v", out, err)
	}
}

func TestDocbankSetupLocalReusesAnExistingServerSectionAndCopiesItsKey(t *testing.T) {
	f := newLocalFixture(t)
	port := freePort(t)
	existing := "[server]\nbind_addr = \"127.0.0.1\"\napi_port = " + strconv.Itoa(port) + "\napi_key = \"" + docbanktest.Key + "\"\n"
	f.writeDocbankConfig(t, existing)
	if _, err := f.run(t, "", "--port", strconv.Itoa(port+1), "--yes"); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("a different --port: err = %v", err)
	}
	if _, err := os.Stat(f.keyPath()); err == nil || f.restarts(t) != nil {
		t.Fatalf("a refused --port changed something")
	}
	if _, err := f.run(t, "", "--port", strconv.Itoa(port), "--yes"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(f.keyPath())
	if err != nil || strings.TrimSpace(string(raw)) != docbanktest.Key {
		t.Errorf("key file = %q, %v; want Docbank's key copied", raw, err)
	}
	if f.docbankConfig(t) != existing {
		t.Errorf("Docbank's config changed:\n%s", f.docbankConfig(t))
	}
	if !strings.Contains(f.config(t), "http://127.0.0.1:"+strconv.Itoa(port)) {
		t.Errorf("kata config.toml = %q", f.config(t))
	}
}

func TestDocbankSetupLocalRefusesAServerSectionMissingAKeyAndNamesIt(t *testing.T) {
	for name, tc := range map[string]struct{ config, names string }{
		"no key":  {"[server]\napi_port = 8123\n", "api_key"},
		"no port": {"[server]\napi_key = \"abc\"\n", "api_port"},
		"neither": {"[server]\nbind_addr = \"127.0.0.1\"\n", "api_port"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newLocalFixture(t)
			f.writeDocbankConfig(t, tc.config)
			_, err := f.run(t, "", "--yes")
			if err == nil || !strings.Contains(err.Error(), tc.names) {
				t.Fatalf("err = %v, want one naming %s", err, tc.names)
			}
			if f.docbankConfig(t) != tc.config || f.restarts(t) != nil || len(f.docbankRuns(t)) != 0 {
				t.Errorf("a refusal changed something")
			}
		})
	}
}

func TestDocbankSetupLocalRefusesABusyPortBeforeWriting(t *testing.T) {
	f := newLocalFixture(t)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	_, err = f.run(t, "", "--port", strconv.Itoa(port), "--yes")
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(port)) || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("err = %v", err)
	}
	f.assertNothingWritten(t)
}

func TestDocbankSetupLocalStopsBeforeKataWhenDocbankDoesNotComeUpWhereAsked(t *testing.T) {
	f := newLocalFixture(t)
	t.Setenv("DOCBANK_FAKE_STATUS", "running: pid 1 at 127.0.0.1:9 (0.17.0)")
	port := freePort(t)
	_, err := f.run(t, "", "--port", strconv.Itoa(port), "--yes")
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1:9") || !strings.Contains(err.Error(), "127.0.0.1:"+strconv.Itoa(port)) {
		t.Fatalf("err = %v, want Docbank's own status shown", err)
	}
	if f.config(t) != "" || f.restarts(t) != nil {
		t.Errorf("kata was touched: config %q, ran %q", f.config(t), f.restarts(t))
	}
}

func TestDocbankSetupLocalRefusesBeforeWritingWithoutDocbankOnPath(t *testing.T) {
	f := newLocalFixture(t)
	if err := os.Remove(filepath.Join(os.Getenv("PATH"), "docbank")); err != nil {
		t.Fatal(err)
	}
	_, err := f.run(t, "", "--port", strconv.Itoa(freePort(t)), "--yes")
	if err == nil || !strings.Contains(err.Error(), "https://github.com/kenn-io/docbank") {
		t.Fatalf("err = %v, want the Docbank address", err)
	}
	f.assertNothingWritten(t)
}

func TestDocbankSetupLocalRefusesAKeyFileHoldingAnotherKey(t *testing.T) {
	f := newLocalFixture(t)
	if err := os.WriteFile(f.keyPath(), []byte("someone-elses-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "", "--port", strconv.Itoa(freePort(t)), "--yes"); err == nil || !strings.Contains(err.Error(), "different key") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(f.dbHome); err == nil || f.restarts(t) != nil {
		t.Errorf("a refusal changed something")
	}
}

func TestDocbankSetupLocalRefusesADifferentKataDocbankSectionBeforeTouchingDocbank(t *testing.T) {
	f := newLocalFixture(t)
	f.writeConfig(t, "[docbank]\nurl = \"https://other.example\"\napi_key_env = \"X\"\n")
	_, err := f.run(t, "", "--port", strconv.Itoa(freePort(t)), "--yes")
	if err == nil || !strings.Contains(err.Error(), "https://other.example") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(f.dbHome); err == nil || len(f.docbankRuns(t)) != 0 {
		t.Errorf("Docbank was touched")
	}
}

func TestDocbankSetupLocalFlagRules(t *testing.T) {
	plan, err := PlanLaunch([]string{"docbank", "setup", "--port", "9000"}, func(string) (string, bool) { return "robot", true }, "test")
	if err != nil || plan.DocbankFlags["port"] != "9000" || plan.DocbankFlags["url"] != "" {
		t.Errorf("local form: %v, %v", plan.DocbankFlags, err)
	}
}

func TestDocbankSetupLocalEditsTheFilesSymlinksPointAt(t *testing.T) {
	f := newLocalFixture(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "docbank.toml")
	keyTarget := filepath.Join(dir, "key")
	for _, p := range []string{target, keyTarget} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.writeDocbankConfig(t, "")
	_ = os.Remove(filepath.Join(f.dbHome, "config.toml"))
	if err := os.Symlink(target, filepath.Join(f.dbHome, "config.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(keyTarget, f.keyPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "", "--port", strconv.Itoa(freePort(t)), "--yes"); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{filepath.Join(f.dbHome, "config.toml"), f.keyPath()} {
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is no longer a symlink: %v, %v", link, info, err)
		}
	}
	if raw, _ := os.ReadFile(target); !strings.Contains(string(raw), "[server]") {
		t.Errorf("Docbank's config target = %q", raw)
	}
	if raw, _ := os.ReadFile(keyTarget); len(strings.TrimSpace(string(raw))) != 64 {
		t.Errorf("key target = %q", raw)
	}
}
