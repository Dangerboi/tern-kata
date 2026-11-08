package docbanksetup

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data"
)

func TestWriteSectionRefusesAConfigThatChangedSinceItWasRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("a = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	const changed = "a = 1\nuser = 2\n"
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	err = writeSection(path, read, kataSection{URL: "http://127.0.0.1:8080", APIKeyEnv: "K"})
	if err == nil || !strings.Contains(err.Error(), "changed while setup ran") {
		t.Fatalf("writeSection = %v; want the changed-while-setup-ran error", err)
	}
	if got, _ := os.ReadFile(path); string(got) != changed {
		t.Errorf("config = %q; want the user's line kept: %q", got, changed)
	}
}

// noRunRunner finds every program and fails the test when one is run: a setup that must stop early runs none.
type noRunRunner struct{ t *testing.T }

func (noRunRunner) LookPath(name string) (string, error) { return "/bin/" + name, nil }

func (r noRunRunner) Run(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	r.t.Errorf("ran %s %v", name, args)
	return nil, errors.New("not run")
}

func TestSetupRefusesADanglingConfigSymlinkBeforeWritingAnything(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	for name, run := range map[string]func(e Env) error{
		"remote": func(e Env) error {
			return e.SetupRemote(context.Background(), Remote{URL: "http://127.0.0.1:8080", KeyEnv: "K"})
		},
		"local": func(e Env) error { return e.SetupLocal(context.Background(), Local{}) },
	} {
		for _, which := range []string{"kata", "docbank"} {
			if name == "remote" && which == "docbank" {
				continue
			}
			e := Env{KataHome: t.TempDir(), DocbankHome: t.TempDir(), Out: io.Discard}
			e.Runner = noRunRunner{t}
			home := e.KataHome
			if which == "docbank" {
				home = e.DocbankHome
			}
			link := filepath.Join(home, "config.toml")
			target := filepath.Join(home, "missing", "config.toml")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			err := run(e)
			if err == nil || !strings.Contains(err.Error(), link) || !strings.Contains(err.Error(), target) {
				t.Fatalf("%s/%s: setup = %v; want an error naming the link and its target", name, which, err)
			}
			if got, err := os.Readlink(link); err != nil || got != target {
				t.Errorf("%s/%s: link = %q, %v; want it unchanged", name, which, got, err)
			}
			for _, dir := range []string{e.KataHome, e.DocbankHome} {
				entries, _ := os.ReadDir(dir)
				for _, en := range entries {
					if en.Name() != "config.toml" {
						t.Errorf("%s/%s: setup wrote %s", name, which, filepath.Join(dir, en.Name()))
					}
				}
			}
		}
	}
}

func TestSetupLocalTightensAReusedKeyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	const key = "k"
	e := supervisionEnv(t, "", "[server]\nbind_addr = \"127.0.0.1\"\napi_port = 8080\napi_key = \""+key+"\"\n")
	keyPath := filepath.Join(e.KataHome, KeyFileName)
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	e.Runner = fakeRunner{}
	var out strings.Builder
	e.Out = &out
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the run after the key file is not the subject; Docbank's status is the first step to fail
	_ = e.SetupLocal(ctx, Local{})
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o; want 600", info.Mode().Perm())
	}
	if !strings.Contains(out.String(), "0600") {
		t.Errorf("output does not say the mode was tightened: %q", out.String())
	}
}

// fakeRunner finds every program and runs none successfully.
type fakeRunner struct{}

func (fakeRunner) LookPath(name string) (string, error) { return "/bin/" + name, nil }

func (fakeRunner) Run(context.Context, []string, string, ...string) ([]byte, error) {
	return nil, errors.New("not run")
}

func TestShellWordQuotesOnlyWhatAShellWouldSplitOrExpand(t *testing.T) {
	for in, want := range map[string]string{
		"/home/me/.kata": "/home/me/.kata",
		"C:/kata/db_1":   "C:/kata/db_1",
		"":               "''",
		"a b":            "'a b'",
		"it's":           `'it'\''s'`,
		"$HOME":          "'$HOME'",
	} {
		if got := ShellWord(in); got != want {
			t.Errorf("ShellWord(%q) = %q, want %q", in, got, want)
		}
	}
}

// scriptRunner finds every program; `docbank daemon status` names addr once `docbank daemon restart` has run.
type scriptRunner struct {
	addr      string
	restarted *bool
}

func (scriptRunner) LookPath(name string) (string, error) { return "/bin/" + name, nil }

func (r scriptRunner) Run(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	switch strings.Join(args, " ") {
	case "daemon restart":
		*r.restarted = true
		return nil, nil
	case "daemon status":
		if *r.restarted {
			return []byte("running at " + r.addr + "\n"), nil
		}
		return []byte("not running\n"), errors.New("exit status 1")
	}
	return nil, errors.New("unexpected " + name + " " + strings.Join(args, " "))
}

func TestSetupLocalRerunStartsDocbankThatIsNotRunning(t *testing.T) {
	const key = "k"
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	addr := "127.0.0.1:" + strconv.Itoa(port)
	e := supervisionEnv(t, "", "[server]\nbind_addr = \"127.0.0.1\"\napi_port = "+strconv.Itoa(port)+"\napi_key = \""+key+"\"\n")
	keyPath := filepath.Join(e.KataHome, KeyFileName)
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	section := kataSection{URL: "http://" + addr, Folder: DefaultFolder, APIKeyFile: keyPath}
	if err := os.WriteFile(filepath.Join(e.KataHome, "config.toml"), []byte(section.text()), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := false
	e.Runner = scriptRunner{addr: addr, restarted: &restarted}
	e.Locate = func(context.Context, string) (data.Target, error) { return data.Target{}, errors.New("no daemon") }
	var out strings.Builder
	e.Out = &out
	if err := e.setupLocal(context.Background(), Local{Common: Common{NoRestart: true}}); err != nil {
		t.Fatalf("setupLocal = %v\n%s", err, out.String())
	}
	if !restarted {
		t.Errorf("Docbank was not running and was not started; output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "nothing changed") {
		t.Errorf("a rerun with Docbank down said nothing changed:\n%s", out.String())
	}
}

func TestSetupRemoteRefusesAKeyFileKataWouldRefuse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	dir := t.TempDir()
	loose := filepath.Join(dir, "loose.key")
	big := filepath.Join(dir, "big.key")
	for path, content := range map[string]string{loose: "k\n", big: strings.Repeat("k", 64<<10+1)} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(loose, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ path, want string }{
		"group/world readable": {loose, "chmod 600"},
		"over 64 KiB":          {big, "exceeds 64 KiB"},
		"not a regular file":   {dir, "not a readable regular file"},
	} {
		t.Run(name, func(t *testing.T) {
			e := Env{KataHome: t.TempDir(), Out: io.Discard, Runner: noRunRunner{t}}
			err := e.SetupRemote(context.Background(), Remote{URL: "https://docbank.example", KeyFile: tc.path})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("SetupRemote = %v; want an error saying %q", err, tc.want)
			}
		})
	}
}
