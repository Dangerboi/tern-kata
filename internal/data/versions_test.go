package data

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// script writes an executable shell script at path that logs each run's arguments to path.log, then runs body.
func script(t *testing.T, path, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fakes are shell scripts")
	}
	text := "#!/bin/sh\necho \"$@\" >> '" + path + ".log'\n" + body
	if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path + ".log")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

// fakeKata puts a kata on PATH that answers `version --json` with version and `update --check --json` with update (a JSON
// object, or a failure when empty), and returns its path.
func fakeKata(t *testing.T, version, update string) string {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, "kata")
	check := "exit 1"
	if update != "" {
		check = "echo '" + update + "'"
	}
	script(t, path, `case "$1" in
version) echo '{"name":"kata","version":"`+version+`"}' ;;
update) `+check+` ;;
*) exit 2 ;;
esac
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func checkVersions(t *testing.T, g *Gateway) model.VersionsChecked {
	t.Helper()
	return as[model.VersionsChecked](t, run(t, g, model.CheckVersions{}))
}

// sharingConfig is a second Gateway, another Pane on this machine: it keeps its state in g's config folder and reads g's clock.
func sharingConfig(t *testing.T, g *Gateway) *Gateway {
	t.Helper()
	other := newGateway(t, newLocator(Target{}))
	other.workerDirs, other.now = g.workerDirs, func() time.Time { return g.now() }
	return other
}

func TestAReplacedBinaryIsNamedByItsVersionRunOnce(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "1")
	t.Setenv("PATH", t.TempDir())
	exe := filepath.Join(t.TempDir(), "tern-kata")
	script(t, exe, "echo 'tern-kata v0.15.0'\n")
	g := newGateway(t, newLocator(Target{})).WithExecutable(exe)

	if got := checkVersions(t, g).Installed; got != "" {
		t.Fatalf("Installed %q for the binary the Pane started from, want none", got)
	}
	if got := runs(t, exe); got != nil {
		t.Fatalf("the unchanged binary ran: %q", got)
	}

	script(t, exe, "echo 'tern-kata v0.16.0'\n# a longer file than the old one\n")
	for range 2 {
		if got := checkVersions(t, g).Installed; got != "v0.16.0" {
			t.Fatalf("Installed %q after the swap, want v0.16.0", got)
		}
	}
	if got := runs(t, exe); len(got) != 1 || got[0] != "--version" {
		t.Errorf("runs %q, want one --version for the new file", got)
	}
}

func TestANewBinaryThatDoesNotSayItsVersionNamesNone(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "1")
	t.Setenv("PATH", t.TempDir())
	exe := filepath.Join(t.TempDir(), "tern-kata")
	script(t, exe, "exit 0\n")
	g := newGateway(t, newLocator(Target{})).WithExecutable(exe)
	script(t, exe, "echo 'half written'; exit 1\n")
	if got := checkVersions(t, g).Installed; got != "" {
		t.Errorf("Installed %q from a binary that failed --version, want none", got)
	}
}

// A run that failed (a timeout under load, a copier still holding the file) is tried again at the next check.
func TestAFailedVersionRunIsTriedAgain(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "1")
	t.Setenv("PATH", t.TempDir())
	exe := filepath.Join(t.TempDir(), "tern-kata")
	script(t, exe, "exit 0\n")
	g := newGateway(t, newLocator(Target{})).WithExecutable(exe)
	tried := exe + ".tried"
	script(t, exe, "[ -e '"+tried+"' ] || { : > '"+tried+"'; exit 1; }\necho 'tern-kata v0.16.0'\n")
	if got := checkVersions(t, g).Installed; got != "" {
		t.Fatalf("Installed %q from a failed run, want none", got)
	}
	if got := checkVersions(t, g).Installed; got != "v0.16.0" {
		t.Errorf("Installed %q at the next check, want v0.16.0", got)
	}
}

func TestTheKataCLIsVersionIsReadAgainOnlyWhenKataChanges(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "1")
	kata := fakeKata(t, "v0.24.0", "")
	g := newGateway(t, newLocator(Target{}))
	for range 2 {
		if got := checkVersions(t, g).KataCLI; got != "v0.24.0" {
			t.Fatalf("KataCLI %q, want v0.24.0", got)
		}
	}
	if got := runs(t, kata); len(got) != 1 || got[0] != "version --json" {
		t.Errorf("kata ran %q, want one `version --json`", got)
	}
}

// The release check runs at most once an hour per machine: a second Pane, and a restarted one, read the first one's answer.
func TestTheKataReleaseCheckRunsOnceAnHourForEveryPane(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "")
	kata := fakeKata(t, "v0.24.0", `{"update_available":true,"latest_version":"v0.25.0","current_version":"v0.24.0"}`)
	clock := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	g := newGateway(t, newLocator(Target{}))
	g.now = func() time.Time { return clock }

	if got := checkVersions(t, g).KataLatest; got != "v0.25.0" {
		t.Fatalf("KataLatest %q, want v0.25.0", got)
	}
	clock = clock.Add(59 * time.Minute)
	if got := checkVersions(t, sharingConfig(t, g)).KataLatest; got != "v0.25.0" {
		t.Errorf("another Pane's KataLatest %q, want the first one's v0.25.0", got)
	}
	if got := checks(t, kata); got != 1 {
		t.Fatalf("%d release checks within an hour, want 1", got)
	}
	clock = clock.Add(2 * time.Minute)
	checkVersions(t, g)
	if got := checks(t, kata); got != 2 {
		t.Errorf("%d release checks after an hour, want 2", got)
	}
}

func checks(t *testing.T, kata string) int {
	t.Helper()
	n := 0
	for _, args := range runs(t, kata) {
		if args == "update --check --json" {
			n++
		}
	}
	return n
}

func TestAFailedReleaseCheckIsSilentAndCounts(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "")
	kata := fakeKata(t, "v0.24.0", "")
	g := newGateway(t, newLocator(Target{}))
	if got := checkVersions(t, g).KataLatest; got != "" {
		t.Errorf("KataLatest %q from a failed check, want none", got)
	}
	checkVersions(t, sharingConfig(t, g))
	if got := checks(t, kata); got != 1 {
		t.Errorf("%d release checks, want the failed one only", got)
	}
}

func TestNoReleaseCheckWithTheVariableSet(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "1")
	kata := fakeKata(t, "v0.24.0", `{"update_available":true,"latest_version":"v0.25.0"}`)
	g := newGateway(t, newLocator(Target{}))
	if got := checkVersions(t, g).KataLatest; got != "" {
		t.Errorf("KataLatest %q with the variable set, want none", got)
	}
	if got := checks(t, kata); got != 0 {
		t.Errorf("%d release checks with the variable set", got)
	}
}

func TestTheDaemonsVersionIsReadFromTheDaemon(t *testing.T) {
	t.Setenv("TERN_KATA_NO_UPDATE_CHECK", "1")
	t.Setenv("PATH", t.TempDir())
	g := newGateway(t, newLocator(startKata(t, "0.22.1").target()))
	got := as[model.VersionsChecked](t, run(t, g, model.CheckVersions{Workspace: t.TempDir(), Request: 3, Connection: 2}))
	if got.Daemon != "v0.22.1" || got.Request != 3 || got.Connection != 2 {
		t.Errorf("answer %+v, want daemon v0.22.1 for request 3 on connection 2", got)
	}
}
