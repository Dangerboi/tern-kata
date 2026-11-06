package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeTern puts a `tern` alone on PATH that records its arguments, one line per call, in f.ternLog; `tern plugin dir` prints
// pluginDir and every other call exits with installStatus. On Windows it is a tern.bat.
func (f *updateFixture) fakeTern(t *testing.T, pluginDir string, installStatus int) {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	f.ternLog = filepath.Join(t.TempDir(), "tern.log")
	if runtime.GOOS == "windows" {
		script := "@echo off\r\necho %*>>\"" + f.ternLog + "\"\r\nif \"%1 %2\"==\"plugin dir\" (echo " + pluginDir + "& exit /b 0)\r\necho tern said no 1>&2\r\nexit /b " + strconv.Itoa(installStatus) + "\r\n"
		if err := os.WriteFile(filepath.Join(bin, "tern.bat"), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	script := "#!/bin/sh\necho \"$@\" >> '" + f.ternLog + "'\n" +
		"if [ \"$1 $2\" = \"plugin dir\" ]; then echo '" + pluginDir + "'; exit 0; fi\n" +
		"echo 'tern said no' >&2\nexit " + strconv.Itoa(installStatus) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "tern"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// ternCalls is what the fake tern was asked, one call per line.
func (f *updateFixture) ternCalls() []string {
	text, _ := os.ReadFile(f.ternLog)
	return strings.Split(strings.TrimSpace(string(text)), "\n")
}

func installPlugin(t *testing.T, pluginDir, version string) {
	t.Helper()
	dir := filepath.Join(pluginDir, "tern-kata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte("schema = 1\nid = \"tern-kata\"\nversion = \""+version+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tern is a shell script")
	}
}

func TestUpdateInstallsTheReleasePluginOverAnOlderInstalledCopy(t *testing.T) {
	skipOnWindows(t)
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	pluginDir := t.TempDir()
	installPlugin(t, pluginDir, "0.1.0")
	f.fakeTern(t, pluginDir, 0)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	calls := f.ternCalls()
	if len(calls) != 2 || calls[0] != "plugin dir" || !strings.HasPrefix(calls[1], "plugin install ") || !strings.HasSuffix(calls[1], string(filepath.Separator)+filepath.Join("unpacked", "plugin")+" --force") {
		t.Errorf("tern was called with %q", calls)
	}
	if want := "Updated this machine's tern-kata plugin from 0.1.0 to 0.2.0\n"; !strings.Contains(f.out.String(), want) {
		t.Errorf("output lacks %q:\n%s", want, f.out.String())
	}
	f.noLeftovers(t)
}

func TestUpdateInstallsOverAnInstalledVersionThatCannotBeCompared(t *testing.T) {
	skipOnWindows(t)
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	pluginDir := t.TempDir()
	installPlugin(t, pluginDir, "dev")
	f.fakeTern(t, pluginDir, 0)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if calls := f.ternCalls(); len(calls) != 2 || !strings.HasPrefix(calls[1], "plugin install ") {
		t.Errorf("tern was called with %q", calls)
	}
}

func TestUpdateLeavesALinkedPluginAndSaysSo(t *testing.T) {
	skipOnWindows(t)
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	pluginDir := t.TempDir()
	installPlugin(t, pluginDir, "0.1.0")
	if err := os.WriteFile(filepath.Join(pluginDir, "tern-kata.path"), []byte("/work/tern-kata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.fakeTern(t, pluginDir, 0)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if calls := f.ternCalls(); len(calls) != 1 || calls[0] != "plugin dir" {
		t.Errorf("tern was called with %q", calls)
	}
	if !strings.Contains(f.out.String(), "The tern-kata plugin is linked to /work/tern-kata: left as it is") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestUpdateDoesNothingToThePluginWhenItIsCurrentOrAbsent(t *testing.T) {
	skipOnWindows(t)
	for name, setup := range map[string]func(*testing.T, string){
		"same version": func(t *testing.T, dir string) { installPlugin(t, dir, "0.2.0") },
		"newer plugin": func(t *testing.T, dir string) { installPlugin(t, dir, "0.3.0") },
		"no copy":      func(*testing.T, string) {},
	} {
		f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
		pluginDir := t.TempDir()
		setup(t, pluginDir)
		f.fakeTern(t, pluginDir, 0)
		if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if calls := f.ternCalls(); len(calls) != 1 || calls[0] != "plugin dir" {
			t.Errorf("%s: tern was called with %q", name, calls)
		}
		if strings.Contains(f.out.String(), "plugin") {
			t.Errorf("%s: output mentions the plugin:\n%s", name, f.out.String())
		}
	}
}

func TestUpdateSaysItLeftThePluginWhenThereIsNoTern(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	t.Setenv("PATH", t.TempDir())
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if want := "This machine's tern-kata plugin was not updated: tern is not on PATH."; !strings.Contains(f.out.String(), want) {
		t.Errorf("output lacks %q:\n%s", want, f.out.String())
	}
}

func TestUpdateWarnsWhenThePluginInstallFailsAndStillSucceeds(t *testing.T) {
	skipOnWindows(t)
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	pluginDir := t.TempDir()
	installPlugin(t, pluginDir, "0.1.0")
	f.fakeTern(t, pluginDir, 1)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatalf("a plugin failure failed the update: %v", err)
	}
	if want := "warning: this machine's tern-kata plugin was not updated: `tern plugin install "; !strings.Contains(f.out.String(), want) || !strings.Contains(f.out.String(), "tern said no") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if !strings.Contains(f.out.String(), "Updated tern-kata from v0.1.0 to v0.2.0") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestUpToDateUpdateDownloadsTheArchiveOnlyWhenThePluginNeedsIt(t *testing.T) {
	skipOnWindows(t)
	f := newUpdateFixture(t, "v0.2.0", "0.2.0", nil)
	pluginDir := t.TempDir()
	installPlugin(t, pluginDir, "0.2.0")
	f.fakeTern(t, pluginDir, 0)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 1 || f.out.String() != "tern-kata v0.2.0 is up to date\n" {
		t.Errorf("%d requests, output %q", f.requests.Load(), f.out.String())
	}

	installPlugin(t, pluginDir, "0.1.0")
	f.requests.Store(0)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	calls := f.ternCalls()
	if f.requests.Load() != 3 || !strings.HasPrefix(calls[len(calls)-1], "plugin install ") {
		t.Errorf("%d requests, tern calls %q", f.requests.Load(), calls)
	}
	want := "tern-kata v0.2.0 is up to date\nUpdated this machine's tern-kata plugin from 0.1.0 to 0.2.0\n"
	if f.out.String() != want {
		t.Errorf("output %q, want %q", f.out.String(), want)
	}
	f.unchanged(t)
}
