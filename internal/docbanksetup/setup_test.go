package docbanksetup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

func TestWriteKeyFileTightensAReusedKeyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	path := filepath.Join(t.TempDir(), KeyFileName)
	if err := os.WriteFile(path, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := (Env{Out: &out}).writeKeyFile(path, "secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
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
	e.Out = io.Discard
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
}

// fakeRunner finds every program and runs none successfully.
type fakeRunner struct{}

func (fakeRunner) LookPath(name string) (string, error) { return "/bin/" + name, nil }

func (fakeRunner) Run(context.Context, []string, string, ...string) ([]byte, error) {
	return nil, errors.New("not run")
}
