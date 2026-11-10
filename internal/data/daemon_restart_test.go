package data

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// restartFixture is a connected local daemon and a fake kata on PATH that records its arguments and KATA_HOME and then runs script.
type restartFixture struct {
	g   *Gateway
	log string
}

func newRestartFixture(t *testing.T, remote bool, script string) *restartFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake kata is a shell script")
	}
	kata := startKata(t, "0.21.0")
	target := kata.target()
	target.Remote = remote
	f := &restartFixture{g: newGateway(t, newLocator(target)), log: filepath.Join(t.TempDir(), "kata.log")}
	bin := t.TempDir()
	body := "#!/bin/sh\necho \"$@\" >> '" + f.log + ".args'\necho \"$KATA_HOME\" >> '" + f.log + ".home'\n" + script
	if err := os.WriteFile(filepath.Join(bin, "kata"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

func (f *restartFixture) logged(t *testing.T, suffix string) string {
	t.Helper()
	raw, err := os.ReadFile(f.log + suffix)
	if err != nil {
		t.Fatalf("kata did not run: %v", err)
	}
	return string(raw)
}

func TestRestartDaemonRunsDaemonRestartWithTheDaemonsHome(t *testing.T) {
	f := newRestartFixture(t, false, "exit 0\n")
	home := os.Getenv("KATA_HOME")
	if err := f.g.RestartDaemon(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := f.logged(t, ".args"); got != "daemon restart\n" {
		t.Errorf("kata ran with %q, want only `daemon restart`", got)
	}
	if got := f.logged(t, ".home"); got != home+"\n" {
		t.Errorf("KATA_HOME %q, want %q", got, home)
	}
}

func TestRestartDaemonOfACatalogProfileRunsWithTheProfilesHome(t *testing.T) {
	f := newRestartFixture(t, false, "exit 0\n")
	profileHome := t.TempDir()
	writeCatalog(t, "active_daemon = \"work\"\n\n[[daemon]]\nname = \"work\"\nlocal = true\nhome = '"+profileHome+"'\ninstance_uid = \"01ARZ3NDEKTSV4RRFFQ69G5FAV\"\n")
	if err := f.g.RestartDaemon(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := f.logged(t, ".home"); got != profileHome+"\n" {
		t.Errorf("KATA_HOME %q, want the profile's home %q", got, profileHome)
	}
	if got := f.logged(t, ".args"); strings.Contains(got, "--daemon") {
		t.Errorf("kata ran with %q; `daemon restart` refuses --daemon", got)
	}
}

func TestRestartDaemonKataFailingReturnsItsLastStderrLine(t *testing.T) {
	f := newRestartFixture(t, false, "echo 'restarting' >&2\necho 'daemon pid 7 is from an older kata release' >&2\necho >&2\nexit 3\n")
	err := f.g.RestartDaemon(t.Context(), t.TempDir())
	if err == nil || err.Error() != "daemon pid 7 is from an older kata release" {
		t.Errorf("error %v, want kata's last line only", err)
	}
}

func TestRestartDaemonKataNotAnsweringInTimeIsKilledAndReportedAsATimeout(t *testing.T) {
	old := daemonRestartTimeout
	daemonRestartTimeout = 200 * time.Millisecond
	t.Cleanup(func() { daemonRestartTimeout = old })
	f := newRestartFixture(t, false, "exec sleep 30\n")
	start := time.Now()
	err := f.g.RestartDaemon(t.Context(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "kata did not answer within 200ms") {
		t.Errorf("error %v, want that kata did not answer in time", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("the restart took %v; kata was not killed", time.Since(start))
	}
}

func TestRestartDaemonRefusesARemoteDaemonAndRunsNothing(t *testing.T) {
	f := newRestartFixture(t, true, "exit 0\n")
	err := f.g.RestartDaemon(t.Context(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "kata daemon restart") {
		t.Errorf("error %v, want the instruction to restart it on its machine", err)
	}
	if _, statErr := os.Stat(f.log + ".args"); statErr == nil {
		t.Error("kata ran for a remote daemon")
	}
}
