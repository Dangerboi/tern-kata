package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func askedDaemons(cmds []model.Cmd) (model.LoadDaemons, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadDaemons); ok {
			return load, true
		}
	}
	return model.LoadDaemons{}, false
}

func askedSwitch(cmds []model.Cmd) (model.SwitchDaemon, bool) {
	for _, cmd := range cmds {
		if c, ok := cmd.(model.SwitchDaemon); ok {
			return c, true
		}
	}
	return model.SwitchDaemon{}, false
}

func askedProject(cmds []model.Cmd) (model.LoadProject, bool) {
	for _, cmd := range cmds {
		if c, ok := cmd.(model.LoadProject); ok {
			return c, true
		}
	}
	return model.LoadProject{}, false
}

// daemonsOpened presses D on s and returns the output.
func daemonsOpened(t *testing.T, s driver.State) driver.Output {
	t.Helper()
	out := step(t, s, key("shift+d"))
	if _, ok := askedDaemons(out.Cmds); !ok || out.State.Model.Page != model.PageDaemons {
		t.Fatalf("D gave page %v and %#v, want the Daemons Page and a read of the catalog", out.State.Model.Page, out.Cmds)
	}
	return out
}

var catalogEntries = []model.DaemonEntry{
	{Name: "home", Endpoint: "local", Current: false},
	{Name: "work", Endpoint: "kata.example.com:7474", Token: true, Current: true},
	{Name: "lab", Endpoint: "10.0.0.5:7474", AllowInsecure: true},
}

// daemonsListed is the Daemons Page after the catalog answered with catalogEntries.
func daemonsListed(t *testing.T) driver.State {
	t.Helper()
	out := daemonsOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedDaemons(out.Cmds)
	return step(t, out.State, model.DaemonsLoaded{Request: load.Request, Entries: catalogEntries, ConfigPath: "/kata/config.toml"}).State
}

func TestDaemonsPageOpensFromEveryPageAndTheMenu(t *testing.T) {
	for name, s := range map[string]driver.State{
		"Issues": projectPane(t, 160, boardCard("a")),
		"Board":  boardPane(t, boardCard("a")),
		"Jobs":   jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false))),
	} {
		out := daemonsOpened(t, s)
		if !pageBarDrawn(t, out.State.Model, "D") || out.State.Model.CurrentMode() != model.ModeDaemons {
			t.Errorf("%s: no Page bar entry for D, or mode %q", name, out.State.Model.CurrentMode())
		}
	}
	menu := step(t, projectPane(t, 160, boardCard("a")), openMenu).State
	out := step(t, menu, model.PickerSelected{Key: "page.daemons"})
	if _, ok := askedDaemons(out.Cmds); !ok || out.State.Model.Page != model.PageDaemons {
		t.Fatalf("the Menu's Daemons entry gave page %v and %#v", out.State.Model.Page, out.Cmds)
	}
}

func TestDaemonsPageListsTheCatalogInKataTuisColumnsAndMarksTheCurrentOne(t *testing.T) {
	s := daemonsListed(t)
	wantText(t, "the list", wireTextOf(t, s), "3 daemons", "Daemon", "Endpoint", "Auth", "State",
		"kata.example.com:7474", "10.0.0.5:7474", "no token", "current")
	if sel, total := s.Model.Position(); sel != 2 || total != 3 {
		t.Errorf("the cursor starts at %d of %d, want the current daemon, 2 of 3", sel, total)
	}
	if text := wireTextOf(t, s); !strings.Contains(text, catalogEntries[1].Row()) || strings.Contains(text, catalogEntries[0].Row()+"current") || catalogEntries[1].State() != "current" {
		t.Errorf("only the current daemon's row ends in current: %s", text)
	}
	wantText(t, "the footer of the current daemon", wireTextOf(t, s), "work · kata.example.com:7474")
	moved := step(t, s, key("j")).State
	wantText(t, "the footer of an insecure daemon", wireTextOf(t, moved), "lab · 10.0.0.5:7474 · allow_insecure")
	top := step(t, s, key("g")).State
	wantText(t, "the footer of a local daemon", wireTextOf(t, top), "home · local")
}

func TestDaemonsPageSaysWhenThereIsNoCatalogAndHowToAddADaemon(t *testing.T) {
	out := daemonsOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedDaemons(out.Cmds)
	wantText(t, "while the read is out", wireTextOf(t, out.State), "loading daemons...")
	empty := step(t, out.State, model.DaemonsLoaded{Request: load.Request, ConfigPath: "/kata/config.toml"}).State
	wantText(t, "an empty catalog", wireTextOf(t, empty), "no named daemons", "add a [[daemon]] entry to /kata/config.toml")
	if out := step(t, empty, key("enter")); len(out.Cmds) != 0 {
		t.Errorf("Enter on an empty catalog sent %#v", out.Cmds)
	}
	failed := step(t, out.State, model.DaemonsLoaded{Request: load.Request, Err: errors.New("parse config.toml: bad\nline 3")}).State
	wantText(t, "an unreadable catalog", wireTextOf(t, failed), "failed to read the daemon catalog: parse config.toml: bad line 3")
}

func TestEnterSwitchesToTheSelectedDaemonAndTheOldAnswersOfTheOldDaemonAreIgnored(t *testing.T) {
	s := daemonsListed(t)
	out := step(t, step(t, s, key("k")).State, key("enter"))
	switched, ok := askedSwitch(out.Cmds)
	if !ok || switched.Name != "home" || switched.Workspace != "/repo" {
		t.Fatalf("Enter sent %#v, want a switch to home", out.Cmds)
	}
	wantText(t, "while connecting", wireTextOf(t, out.State), "connecting to daemon home...")
	if again := step(t, out.State, key("enter")); len(again.Cmds) != 0 {
		t.Errorf("a second Enter while one switch is out sent %#v", again.Cmds)
	}
}

// switchedTo is the Pane after the answer of a successful switch to daemon.
func switchedTo(t *testing.T, s driver.State, daemon model.Daemon) driver.Output {
	t.Helper()
	out := step(t, step(t, s, key("k")).State, key("enter"))
	switched, _ := askedSwitch(out.Cmds)
	return step(t, out.State, model.DaemonSwitched{Request: switched.Request, Name: switched.Name, Daemon: daemon})
}

func TestASuccessfulSwitchResetsThePaneAndFollowsTheNewDaemonsEvents(t *testing.T) {
	// A Pane holding an undoable change, a search, and a followed event stream, on the Daemons Page.
	pane := daemonsListed(t)
	pane.Model.Undo = undoable(t, 2).Model.Undo
	pane.Model.StreamGeneration, pane.Model.StreamConnected = 1, true
	pane.Model.Filters.Search = "alpha"

	out := switchedTo(t, pane, model.Daemon{Name: "home", Kind: model.DaemonLocal, Version: "0.20.0"})
	m := out.State.Model
	project, ok := askedProject(out.Cmds)
	if !ok || !project.Switched || project.Workspace != "/repo" {
		t.Fatalf("a switch asked %#v, want the Project read again on the new daemon", out.Cmds)
	}
	if m.Page != model.PageIssues || len(m.Issues) != 0 || m.SelectedUID != "" || m.Filters.Search != "" || !m.Loading {
		t.Errorf("page %v, %d Issues, selected %q, search %q, loading %v: the Pane keeps what it held of the old daemon",
			m.Page, len(m.Issues), m.SelectedUID, m.Filters.Search, m.Loading)
	}
	if m.Daemon.Name != "home" || !strings.Contains(m.Notice, `daemon "home": connected`) {
		t.Errorf("daemon %+v, notice %q", m.Daemon, m.Notice)
	}
	if u := step(t, out.State, key("u")).State.Model.Notice; u != "nothing to undo: connected to a different daemon" {
		t.Errorf("u after a switch said %q", u)
	}
	// The old daemon's events no longer reach the Pane.
	stale := step(t, out.State, model.StreamEvent{Generation: 1, ID: 99, Type: "issue.updated"})
	if len(stale.Cmds) != 0 {
		t.Errorf("an event of the old stream asked %#v", stale.Cmds)
	}

	// The new daemon's Project and first snapshot arrive: the Pane follows its events from the snapshot's cursor.
	loaded := step(t, out.State, model.ProjectLoaded{Daemon: model.Daemon{Name: "home", Kind: model.DaemonLocal, Version: "0.20.0", Account: "alice"}})
	read, found := snapshotFor(loaded.Cmds, boardCard("z"))
	if !found {
		t.Fatalf("the new Project asked for no snapshot: %#v", loaded.Cmds)
	}
	read.Snapshot.Cursor = 7
	first := step(t, loaded.State, read)
	var sub *model.Subscribe
	for _, cmd := range first.Cmds {
		if c, ok := cmd.(model.Subscribe); ok {
			sub = &c
		}
	}
	if sub == nil || sub.Generation <= 1 || sub.AfterID != 7 {
		t.Fatalf("the first snapshot of the new daemon sent %#v, want a newer event stream after its cursor 7", first.Cmds)
	}
	if got := first.State.Model.Issues; len(got) != 1 || got[0].UID != "z" {
		t.Errorf("Issues %+v, want the new daemon's", got)
	}
	wantText(t, "the top bar", wireTextOf(t, first.State), "Daemon: home · local · kata 0.20.0 · as alice")
}

func TestAFailedSwitchSaysWhyAndKeepsTheConnection(t *testing.T) {
	s := daemonsListed(t)
	s.Model.Undo = undoable(t, 1).Model.Undo
	s.Model.StreamGeneration = 1
	before := s.Model
	out := step(t, step(t, s, key("k")).State, key("enter"))
	switched, _ := askedSwitch(out.Cmds)
	failed := step(t, out.State, model.DaemonSwitched{Request: switched.Request, Name: "home", Err: errors.New("kata daemon start: no space\nleft")})
	if len(failed.Cmds) != 0 {
		t.Errorf("a failed switch sent %#v", failed.Cmds)
	}
	m := failed.State.Model
	if m.Notice != `daemon "home": kata daemon start: no space left` {
		t.Errorf("notice %q", m.Notice)
	}
	if m.Page != model.PageDaemons || len(m.Issues) != len(before.Issues) || len(m.Undo.Entries) != 1 || m.StreamGeneration != 1 || m.Daemon != before.Daemon || m.Project != before.Project {
		t.Errorf("the Pane changed: page %v, %d Issues, %d undo entries, stream %d, daemon %+v", m.Page, len(m.Issues), len(m.Undo.Entries), m.StreamGeneration, m.Daemon)
	}
	// The Page can switch again after a failure.
	if again := step(t, failed.State, key("enter")); len(again.Cmds) != 1 {
		t.Errorf("Enter after a failed switch sent %#v", again.Cmds)
	}
}

func TestAnAnswerToAnOlderSwitchIsIgnored(t *testing.T) {
	s := daemonsListed(t)
	out := step(t, step(t, s, key("k")).State, key("enter"))
	switched, _ := askedSwitch(out.Cmds)
	late := step(t, out.State, model.DaemonSwitched{Request: switched.Request - 1, Name: "home", Daemon: model.Daemon{Name: "home"}})
	if len(late.Cmds) != 0 || late.State.Model.Daemon.Name == "home" || late.State.Model.Daemons.Switching != "home" {
		t.Errorf("a stale answer changed the Pane: %#v, daemon %+v, switching %q", late.Cmds, late.State.Model.Daemon, late.State.Model.Daemons.Switching)
	}
}

func TestASwitchStillOutWhenThePageIsLeftStillChangesTheConnection(t *testing.T) {
	s := daemonsListed(t)
	out := step(t, step(t, s, key("k")).State, key("enter"))
	switched, _ := askedSwitch(out.Cmds)
	left := step(t, out.State, key("escape")).State
	if left.Model.Page == model.PageDaemons {
		t.Fatal("esc did not leave the Daemons Page")
	}
	done := step(t, left, model.DaemonSwitched{Request: switched.Request, Name: "home", Daemon: model.Daemon{Name: "home"}})
	if _, ok := askedProject(done.Cmds); !ok || done.State.Model.Daemon.Name != "home" {
		t.Errorf("the answer after leaving gave %#v, daemon %+v", done.Cmds, done.State.Model.Daemon)
	}
}

// A switch ends every card the old connection owned: a confirmation opened while the switch was out must not act on the new daemon.
func TestASuccessfulSwitchClosesEveryCardOfTheOldConnection(t *testing.T) {
	for card, open := range cardOpeners() {
		if card == model.CardWorker {
			continue // the machine's worker is not part of the daemon connection
		}
		s := open(t)
		out := step(t, s, model.DaemonSwitched{Name: "home", Daemon: model.Daemon{Name: "home"}})
		if got := out.State.Model.TopCard(); got != model.NoCard {
			t.Errorf("a switch left the %q card open", got.Name())
		}
	}
}

func TestARevokeConfirmedAfterASwitchRevokesNothing(t *testing.T) {
	s := step(t, tokenList(t), key("x")).State
	s = step(t, s, model.DaemonSwitched{Name: "home", Daemon: model.Daemon{Name: "home"}}).State
	for _, chord := range []string{"y", "enter"} {
		for _, cmd := range step(t, s, key(chord)).Cmds {
			if revoke, ok := cmd.(model.RevokeTokenCmd); ok {
				t.Fatalf("%s sent %#v on the new daemon", chord, revoke)
			}
		}
	}
}

func TestASuccessfulSwitchDropsTheDraftsAndSaysSo(t *testing.T) {
	s := feed(t, onAlphaDetail(t), key("c"), key("a")).State
	if len(s.Model.Drafts) != 1 {
		t.Fatalf("fixture: %d drafts held", len(s.Model.Drafts))
	}
	m := step(t, s, model.DaemonSwitched{Name: "home", Daemon: model.Daemon{Name: "home"}}).State.Model
	if len(m.Drafts) != 0 {
		t.Errorf("%d drafts survived the switch", len(m.Drafts))
	}
	if !strings.Contains(m.Notice, "1 unsent draft dropped") {
		t.Errorf("notice %q, want it to say the draft was dropped", m.Notice)
	}
}
