package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

const (
	topBarVersionID = "main.topbar.right.version"
	topBarHeadingID = "main.topbar.left.heading"
	oldBinaryText   = "tern-kata v0.16.0 is installed: reopen this Pane to use it."
)

// versionPane is a Pane of the given width running tern-kata at version running on a local kata v0.23.0 daemon, first load
// done, and the version check its start asked for.
func versionPane(t *testing.T, cols int, running string) (driver.State, model.CheckVersions) {
	t.Helper()
	started := step(t, driver.State{Model: model.Model{Versions: model.Versions{Running: running}}}, model.Start{Workspace: "/repo"})
	check, ok := askedVersions(started.Cmds)
	if !ok {
		t.Fatalf("the start asked for no version check: %#v", started.Cmds)
	}
	s := step(t, started.State, resize(cols)).State
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: project, Daemon: model.Daemon{Kind: model.DaemonLocal, Version: "v0.23.0"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Catalog: []model.CatalogProject{{Project: project}}}})
	return s, check
}

func askedVersions(cmds []model.Cmd) (model.CheckVersions, bool) {
	for _, cmd := range cmds {
		if check, ok := cmd.(model.CheckVersions); ok {
			return check, true
		}
	}
	return model.CheckVersions{}, false
}

// checked answers check with what the session host holds, and returns the step and the next check it asks for.
func checked(t *testing.T, s driver.State, check model.CheckVersions, answer model.VersionsChecked) (driver.Output, model.CheckVersions) {
	t.Helper()
	answer.Request, answer.Connection = check.Request, check.Connection
	out := step(t, s, answer)
	next, ok := askedVersions(out.Cmds)
	if !ok {
		t.Fatalf("the answer asked for no further check: %#v", out.Cmds)
	}
	return out, next
}

func heading(t *testing.T, s driver.State) string {
	t.Helper()
	text, _ := nodeText(t, s, topBarHeadingID)
	return text
}

func TestTheTopBarShowsTheBinarysVersionAfterTheDaemon(t *testing.T) {
	s, _ := versionPane(t, 200, "v0.15.0")
	if got, ok := nodeText(t, s, topBarVersionID); !ok || got != "tern-kata v0.15.0" {
		t.Fatalf("version cell %q (drawn %v), want tern-kata v0.15.0", got, ok)
	}
	if daemon, _ := nodeText(t, s, "main.topbar.right.daemon"); daemon != "Daemon: local · kata v0.23.0" {
		t.Errorf("daemon cell %q", daemon)
	}
}

func TestAReplacedBinaryShowsTheMarkerAMenuEntryAndOneToast(t *testing.T) {
	s, check := versionPane(t, 200, "v0.15.0")
	if strings.Contains(heading(t, s), model.NoticeMarker) {
		t.Fatalf("marker before any notice: %q", heading(t, s))
	}
	out, next := checked(t, s, check, model.VersionsChecked{Installed: "v0.16.0", KataCLI: "v0.23.0", Daemon: "v0.23.0"})
	if next.After != model.VersionCheckEvery {
		t.Errorf("next check after %s, want %s", next.After, model.VersionCheckEvery)
	}
	if !strings.HasSuffix(heading(t, out.State), model.NoticeMarker) {
		t.Errorf("heading %q lacks the marker", heading(t, out.State))
	}
	if toast := out.State.Model.Toast; toast == nil || toast.Text != oldBinaryText {
		t.Fatalf("toast %+v, want %q", toast, oldBinaryText)
	}
	seq := out.State.Model.ToastSeq

	again, _ := checked(t, out.State, next, model.VersionsChecked{Installed: "v0.16.0", KataCLI: "v0.23.0", Daemon: "v0.23.0"})
	if again.State.Model.ToastSeq != seq {
		t.Errorf("the same notice toasted twice: %+v", again.State.Model.Toast)
	}
	menu := step(t, again.State, openMenu).State
	wantText(t, "Menu", wireTextOf(t, menu), "tern-kata v0.16.0 is installed", "reopen this Pane")
}

// The Pane claims an old binary only on two versions it can compare that differ: a build without a stamped version never does.
func TestNoOldBinaryNoticeWithoutTwoDifferentStampedVersions(t *testing.T) {
	for name, tc := range map[string]struct{ running, installed string }{
		"unchanged":         {"v0.15.0", ""},
		"same version":      {"v0.15.0", "v0.15.0"},
		"unstamped running": {"dev", "v0.16.0"},
		"unstamped new":     {"v0.15.0", "dev"},
	} {
		t.Run(name, func(t *testing.T) {
			s, check := versionPane(t, 200, tc.running)
			out, _ := checked(t, s, check, model.VersionsChecked{Installed: tc.installed, KataCLI: "v0.23.0", Daemon: "v0.23.0"})
			if strings.Contains(heading(t, out.State), model.NoticeMarker) || out.State.Model.Toast != nil {
				t.Errorf("heading %q, toast %+v; want no notice", heading(t, out.State), out.State.Model.Toast)
			}
		})
	}
}

const daemonNoticeText = "The kata daemon runs v0.23.0 and kata v0.24.0 is installed: restart the daemon from the Menu to use it."

// pickNotice opens the Menu and picks its entry for the notice of kind.
func pickNotice(t *testing.T, s driver.State, kind model.NoticeKind) driver.Output {
	t.Helper()
	menu := step(t, s, openMenu).State
	for _, entry := range menu.Model.PickerEntries() {
		if entry.Kind == model.EntryNotice && model.NoticeKind(entry.Index) == kind {
			return step(t, menu, model.PickerSelected{Key: entry.Key()})
		}
	}
	t.Fatalf("the Menu lists no entry for notice %d: %+v", kind, menu.Model.PickerEntries())
	return driver.Output{}
}

func restartCmd(cmds []model.Cmd) (model.RestartKataDaemon, bool) {
	for _, cmd := range cmds {
		if restart, ok := cmd.(model.RestartKataDaemon); ok {
			return restart, true
		}
	}
	return model.RestartKataDaemon{}, false
}

func TestADaemonOlderThanTheKataCLIIsRestartedFromItsMenuEntryAndTheNoticeClears(t *testing.T) {
	s, check := versionPane(t, 200, "v0.15.0")
	out, _ := checked(t, s, check, model.VersionsChecked{KataCLI: "v0.24.0", Daemon: "v0.23.0"})
	if toast := out.State.Model.Toast; toast == nil || toast.Text != daemonNoticeText {
		t.Fatalf("toast %+v, want %q", toast, daemonNoticeText)
	}
	menu := step(t, out.State, openMenu).State
	wantText(t, "Menu", wireTextOf(t, menu), "kata daemon v0.23.0 is older than kata v0.24.0", "restart the daemon…")

	asked := pickNotice(t, out.State, model.NoticeOldDaemon)
	if _, sent := restartCmd(asked.Cmds); sent {
		t.Fatalf("the pick restarted the daemon without asking: %#v", asked.Cmds)
	}
	wantText(t, "question", wireTextOf(t, asked.State), model.RestartQuestion)

	confirmed := step(t, asked.State, key("y"))
	restart, sent := restartCmd(confirmed.Cmds)
	if !sent || restart.Workspace != "/repo" {
		t.Fatalf("y sent %#v, want one RestartKataDaemon for /repo", confirmed.Cmds)
	}
	if again := step(t, confirmed.State, key("y")); len(again.Cmds) != 0 {
		t.Errorf("a second y while the restart is out sent %#v", again.Cmds)
	}
	restarted := step(t, confirmed.State, model.KataDaemonRestarted{Connection: restart.Connection})
	recheck, ok := askedVersions(restarted.Cmds)
	if !ok || recheck.After != 0 {
		t.Fatalf("the restart asked for %#v, want a version check at once", restarted.Cmds)
	}
	cleared, _ := checked(t, restarted.State, recheck, model.VersionsChecked{KataCLI: "v0.24.0", Daemon: "v0.24.0"})
	if strings.Contains(heading(t, cleared.State), model.NoticeMarker) {
		t.Errorf("the marker stays after the restart: %q", heading(t, cleared.State))
	}
	if daemon, _ := nodeText(t, cleared.State, "main.topbar.right.daemon"); daemon != "Daemon: local · kata v0.24.0" {
		t.Errorf("daemon cell %q, want the restarted daemon's version", daemon)
	}
}

func TestAFailedRestartSaysWhyAndKeepsTheNotice(t *testing.T) {
	s, check := versionPane(t, 200, "v0.15.0")
	out, _ := checked(t, s, check, model.VersionsChecked{KataCLI: "v0.24.0", Daemon: "v0.23.0"})
	confirmed := step(t, pickNotice(t, out.State, model.NoticeOldDaemon).State, key("y"))
	restart, _ := restartCmd(confirmed.Cmds)
	failed := step(t, confirmed.State, model.KataDaemonRestarted{Err: errors.New("kata is not on PATH"), Connection: restart.Connection}).State
	wantText(t, "Pane", wireTextOf(t, failed), "The kata daemon was not restarted: kata is not on PATH")
	if !strings.HasSuffix(heading(t, failed), model.NoticeMarker) {
		t.Errorf("the marker went with a failed restart: %q", heading(t, failed))
	}
}

// A remote daemon is restarted on its own machine, so the session host's kata says nothing about it.
func TestARemoteDaemonGetsNoDaemonNotice(t *testing.T) {
	s, check := versionPane(t, 200, "v0.15.0")
	s.Model.Daemon.Kind = model.DaemonRemote
	out, _ := checked(t, s, check, model.VersionsChecked{KataCLI: "v0.24.0", Daemon: "v0.23.0"})
	if _, ok := out.State.Model.NoticeOf(model.NoticeOldDaemon); ok || out.State.Model.Toast != nil {
		t.Errorf("a remote daemon got the notice; toast %+v", out.State.Model.Toast)
	}
}

// After `kata update` the release notice gives way to the daemon's: the CLI is new, the daemon still the old one.
func TestANewerKataReleaseShowsItsNoticeUntilKataIsUpdated(t *testing.T) {
	s, check := versionPane(t, 200, "v0.15.0")
	out, next := checked(t, s, check, model.VersionsChecked{KataCLI: "v0.23.0", KataLatest: "v0.24.0", Daemon: "v0.23.0"})
	const text = "kata v0.24.0 is out: run kata update to install it."
	if toast := out.State.Model.Toast; toast == nil || toast.Text != text {
		t.Fatalf("toast %+v, want %q", toast, text)
	}
	wantText(t, "Menu", wireTextOf(t, step(t, out.State, openMenu).State), "kata v0.24.0 is out", "run kata update")
	if picked := pickNotice(t, out.State, model.NoticeKataRelease).State; picked.Model.Notice != text {
		t.Errorf("the pick's Notice %q, want %q", picked.Model.Notice, text)
	}

	updated, _ := checked(t, out.State, next, model.VersionsChecked{KataCLI: "v0.24.0", KataLatest: "v0.24.0", Daemon: "v0.23.0"})
	if _, ok := updated.State.Model.NoticeOf(model.NoticeKataRelease); ok {
		t.Error("the release notice stays once kata is at that release")
	}
	if _, ok := updated.State.Model.NoticeOf(model.NoticeOldDaemon); !ok {
		t.Error("no daemon notice after kata was updated under a running daemon")
	}
}

// The marker sits in the title's cell, which never shrinks: the narrowest Pane that draws the top bar still shows it, while the
// version, like the Daemon, is left out below FoldedLegendCols.
func TestTheMarkerStaysAtTheNarrowestWidth(t *testing.T) {
	s, check := versionPane(t, model.MinCols, "v0.15.0")
	out, _ := checked(t, s, check, model.VersionsChecked{Installed: "v0.16.0", KataCLI: "v0.23.0", Daemon: "v0.23.0"})
	if !strings.HasSuffix(heading(t, out.State), model.NoticeMarker) {
		t.Errorf("heading %q lacks the marker at %d columns", heading(t, out.State), model.MinCols)
	}
	if _, drawn := nodeText(t, out.State, topBarVersionID); drawn {
		t.Errorf("the version is drawn at %d columns", model.MinCols)
	}
}
