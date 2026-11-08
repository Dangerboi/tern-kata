package driver_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// configAllRows is every row the Config Page lists, in its order, the Docbank row with its marker absent (supervision on).
func configAllRows() []model.ConfigFile {
	enter := []model.ConfigAction{model.ConfigOpen}
	return []model.ConfigFile{
		{Key: model.ConfigLanes, Title: "lanes.toml", Path: "/cfg/tern-kata/lanes.toml", Actions: enter},
		{Key: model.ConfigFilters, Title: "filters.toml", Path: "/cfg/tern-kata/filters.toml", Actions: enter},
		{Key: model.ConfigViews, Title: "views.toml", Path: "/cfg/tern-kata/views.toml", Actions: enter},
		{Key: model.ConfigWorker, Title: "worker.toml", Path: "/cfg/tern-kata/worker.toml", Actions: enter},
		{Key: model.ConfigAgent, Title: "agent.toml", Path: "/cfg/tern-kata/agent.toml", Actions: []model.ConfigAction{model.ConfigEdit}},
		{Key: model.ConfigDocbank, Title: "docbank.manual (Docbank of the default kata daemon)", Path: "/cfg/tern-kata/docbank.manual",
			Summary: "on: the plugin keeps Docbank running", Actions: []model.ConfigAction{model.ConfigSwitch}},
		{Key: model.ConfigKata, Title: "config.toml (kata's, read-only)", Path: "/kata/config.toml", Status: model.ConfigLoads, Actions: enter},
	}
}

// configOn opens the Config Page on s, answers the read with rows and puts the cursor on the row with key.
func configOn(t *testing.T, s driver.State, rows []model.ConfigFile, key model.ConfigFileKey) driver.State {
	t.Helper()
	out := configOpened(t, s)
	load, _ := askedConfigFiles(out.Cmds)
	s = step(t, out.State, model.ConfigFilesLoaded{Request: load.Request, Folder: "/cfg/tern-kata", Files: rows}).State
	for i, f := range rows {
		if f.Key == key {
			for range i {
				s = step(t, s, pressKey("j")).State
			}
			return s
		}
	}
	t.Fatalf("no row %q", key)
	return s
}

func pressKey(chord string) any { return key(chord) }

func equalCmds(a, b []model.Cmd) bool { return reflect.DeepEqual(a, b) }

func hasCmd[T model.Cmd](cmds []model.Cmd) bool {
	return slices.ContainsFunc(cmds, func(c model.Cmd) bool { _, ok := c.(T); return ok })
}

func TestConfigEnterOnLanesOpensTheBoardPageThenTheLanesCardOnceTheListIsRead(t *testing.T) {
	for name, s := range map[string]driver.State{"from Issues": projectPane(t, 160, boardCard("a")), "from the Board": boardPane(t, boardCard("a"))} {
		out := step(t, configOn(t, s, configAllRows(), model.ConfigLanes), key("enter"))
		m := out.State.Model
		if m.Page != model.PageBoard || m.LanesForm.Open {
			t.Fatalf("%s: page %v, card open %v; want the Board with the card waiting for the Lane list", name, m.Page, m.LanesForm.Open)
		}
		if !hasCmd[model.LoadLanes](out.Cmds) {
			t.Fatalf("%s: no read of the Lane list: %#v", name, out.Cmds)
		}
		if again := step(t, out.State, key("shift+k")); !hasCmd[model.LoadConfigFiles](again.Cmds) {
			t.Errorf("%s: K after the jump did not read the files again", name)
		}
		done := step(t, out.State, newestLanes{ProjectUID: "p", Lanes: []string{"todo", "doing"}}).State.Model
		if !done.LanesForm.Open || done.LanesForm.Text != "todo, doing" || done.Page != model.PageBoard {
			t.Errorf("%s: after the read: card open %v with %q on page %v", name, done.LanesForm.Open, done.LanesForm.Text, done.Page)
		}
	}
}

func TestConfigEnterOnLanesStartsTheCardOnTheSharedListWhenTheProjectHasOne(t *testing.T) {
	out := step(t, configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigLanes), key("enter"))
	done := step(t, out.State, newestLanes{ProjectUID: "p", Shared: true, Lanes: []string{"theirs"}, ProjectID: 7, Revision: 9}).State.Model
	if !done.LanesForm.Open || !done.LanesForm.Share || done.LanesForm.Text != "theirs" {
		t.Errorf("card open %v, shared %v, text %q; want the shared list", done.LanesForm.Open, done.LanesForm.Share, done.LanesForm.Text)
	}
}

func TestConfigEnterOnFiltersOpensTheIssuesPageThenTheFilterCard(t *testing.T) {
	out := step(t, configOn(t, boardPane(t, boardCard("a")), configAllRows(), model.ConfigFilters), key("enter"))
	m := out.State.Model
	if m.Page != model.PageIssues || !m.FilterForm.Open {
		t.Fatalf("page %v, Filter card open %v", m.Page, m.FilterForm.Open)
	}
}

func TestConfigEnterOnWorkerOpensTheJobsPageThenTheWorkerCardAndReadsTheSetup(t *testing.T) {
	out := step(t, configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigWorker), key("enter"))
	m := out.State.Model
	if m.Page != model.PageJobs || !m.WorkerCard.Open {
		t.Fatalf("page %v, Worker card open %v", m.Page, m.WorkerCard.Open)
	}
	if !hasCmd[model.LoadWorkerSetup](out.Cmds) || !hasCmd[model.LoadWorker](out.Cmds) {
		t.Errorf("the card did not read the setup and ask the worker's status: %#v", out.Cmds)
	}
}

func TestConfigEnterOnViewsOpensTheMenuAtTheViewsLevelAndBackReturnsToTheTopLevel(t *testing.T) {
	out := step(t, configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigViews), key("enter"))
	m := out.State.Model
	if m.OnConfig() || !m.Picker.Open || m.Picker.Level != model.MenuViews {
		t.Fatalf("page %v, Menu open %v at level %v; want the popup at the Views level", m.Page, m.Picker.Open, m.Picker.Level)
	}
	if !hasCmd[model.LoadViews](out.Cmds) {
		t.Errorf("the Views level did not read the views: %#v", out.Cmds)
	}
	back := step(t, out.State, key("escape")).State.Model
	if !back.Picker.Open || back.Picker.Level != model.MenuTop || back.OnConfig() {
		t.Errorf("esc: Menu open %v at level %v on page %v; want the top level", back.Picker.Open, back.Picker.Level, back.Page)
	}
}

func TestConfigEnterOnKataConfigOpensTheDaemonsPageWhoseBackReturnsToThePageBeforeTheConfigPage(t *testing.T) {
	out := step(t, configOn(t, boardPane(t, boardCard("a")), configAllRows(), model.ConfigKata), key("enter"))
	if out.State.Model.Page != model.PageDaemons || !hasCmd[model.LoadDaemons](out.Cmds) {
		t.Fatalf("page %v and %#v; want the Daemons Page and its read", out.State.Model.Page, out.Cmds)
	}
	if back := step(t, out.State, key("escape")).State.Model.Page; back != model.PageBoard {
		t.Errorf("back from the Daemons Page gave page %v, want the Board shown before the Config Page", back)
	}
}

func TestConfigEnterOnAgentDoesNothing(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigAgent)
	out := step(t, s, key("enter"))
	if out.State.Model.Page != model.PageConfig || len(out.Cmds) != 0 {
		t.Errorf("page %v and %#v after enter on agent.toml", out.State.Model.Page, out.Cmds)
	}
}

func TestConfigJumpsThatNeedAProjectShowThePagesRefusalAndOpenNoCard(t *testing.T) {
	unbound := func() driver.State {
		s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
		s.Model = loadFirst(s.Model, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}})
		return s
	}
	for key_, want := range map[model.ConfigFileKey]string{model.ConfigLanes: "Board needs a Project", model.ConfigWorker: "Jobs Page needs a Project"} {
		m := step(t, configOn(t, unbound(), configAllRows(), key_), key("enter")).State.Model
		if m.Page != model.PageIssues || !strings.Contains(m.Notice, want) || m.LanesForm.Open || m.WorkerCard.Open {
			t.Errorf("%s: page %v, notice %q, Lanes card %v, Worker card %v", key_, m.Page, m.Notice, m.LanesForm.Open, m.WorkerCard.Open)
		}
	}
}

func TestConfigFooterAndHelpSheetNameWhatEnterOpensAndNothingForAgent(t *testing.T) {
	labels := map[model.ConfigFileKey]string{
		model.ConfigLanes: "open Lanes card", model.ConfigFilters: "open Filter card", model.ConfigViews: "open Views",
		model.ConfigWorker: "open Worker card", model.ConfigKata: "open Daemons Page",
	}
	enterHint := func(m model.Model) (string, bool) {
		for b := range model.BindingsForMode(m.CurrentMode(), true) {
			if b.Chord == "enter" && b.Offered(m) {
				return b.Described(m), true
			}
		}
		return "", false
	}
	sheetHas := func(m model.Model, text string) bool {
		for _, g := range model.HelpSheetOn(model.PageConfig, m) {
			for _, l := range g.Lines {
				if l.Text == text && slices.Contains(l.Keys, "↵") {
					return true
				}
			}
		}
		return false
	}
	for key_, want := range labels {
		m := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), key_).Model
		if got, ok := enterHint(m); !ok || got != want {
			t.Errorf("%s: footer says %q (shown %v), want %q", key_, got, ok, want)
		}
		if !sheetHas(m, want) {
			t.Errorf("%s: the help sheet lacks %q on enter", key_, want)
		}
	}
	agent := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigAgent).Model
	if got, ok := enterHint(agent); ok {
		t.Errorf("the footer shows enter (%q) on agent.toml", got)
	}
	for _, g := range model.HelpSheetOn(model.PageConfig, agent) {
		for _, l := range g.Lines {
			if slices.Contains(l.Keys, "↵") {
				t.Errorf("the help sheet lists enter (%q) on agent.toml", l.Text)
			}
		}
	}
}

func TestConfigClickOnARowsActionDoesWhatEnterDoes(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigLanes)
	out := step(t, s, click(view.ConfigFileID(model.ConfigWorker)+".open", view.ConfigActClick(model.ConfigWorker, model.ConfigOpen)))
	if m := out.State.Model; m.Page != model.PageJobs || !m.WorkerCard.Open {
		t.Errorf("page %v, Worker card open %v after clicking the worker row's action", m.Page, m.WorkerCard.Open)
	}
	if m := step(t, s, click(view.ConfigFileID(model.ConfigAgent)+".open", view.ConfigActClick(model.ConfigAgent, model.ConfigOpen))).State.Model; m.Page != model.PageConfig {
		t.Errorf("a click on agent.toml's missing action left page %v", m.Page)
	}
}

func TestConfigDocbankRowShowsItsStateAndSTurnsSupervisionOffThenOn(t *testing.T) {
	rows := configAllRows()
	s := configOn(t, projectPane(t, 160, boardCard("a")), rows, model.ConfigDocbank)
	wantText(t, "supervision on", wireTextOf(t, s), "/cfg/tern-kata/docbank.manual", "absent", "on: the plugin keeps Docbank running", "Docbank of the default kata daemon")

	off := step(t, s, key("s"))
	if want := []model.Cmd{model.SetDocbankSupervision{On: false}}; !equalCmds(off.Cmds, want) {
		t.Fatalf("s with the marker absent sent %#v, want %#v", off.Cmds, want)
	}
	if ignored := step(t, off.State, key("s")); len(ignored.Cmds) != 0 {
		t.Errorf("s while a switch is out sent %#v", ignored.Cmds)
	}
	done := step(t, off.State, model.DocbankSupervisionSet{On: false})
	if !hasCmd[model.LoadConfigFiles](done.Cmds) {
		t.Fatalf("the rows were not read again: %#v", done.Cmds)
	}
	wantText(t, "after off", wireTextOf(t, done.State), "a Docbank that is running keeps running")
	load, _ := askedConfigFiles(done.Cmds)
	marker := configAllRows()
	marker[5].Status, marker[5].Summary = model.ConfigLoads, "off: docbank.manual exists in /cfg/tern-kata"
	reread := step(t, done.State, model.ConfigFilesLoaded{Request: load.Request, Folder: "/cfg/tern-kata", Files: marker}).State
	if f := reread.Model.SelectedConfigFile(); f == nil || f.Key != model.ConfigDocbank {
		t.Fatalf("the cursor left the Docbank row after the read: %+v", f)
	}
	wantText(t, "marker present", wireTextOf(t, reread), "exists", "off: docbank.manual exists in /cfg/tern-kata")

	on := step(t, reread, key("s"))
	if want := []model.Cmd{model.SetDocbankSupervision{On: true}}; !equalCmds(on.Cmds, want) {
		t.Fatalf("s with the marker present sent %#v, want %#v", on.Cmds, want)
	}
	wantText(t, "after on", wireTextOf(t, step(t, on.State, model.DocbankSupervisionSet{On: true}).State), "within 10 seconds")
}

func TestConfigDocbankSwitchThatFailsShowsTheErrorAndLeavesTheRow(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigDocbank)
	sent := step(t, s, key("s"))
	failed := step(t, sent.State, model.DocbankSupervisionSet{Err: errors.New("mkdir /cfg: permission denied")})
	if hasCmd[model.LoadConfigFiles](failed.Cmds) {
		t.Errorf("a failed switch read the rows again: %#v", failed.Cmds)
	}
	wantText(t, "failed switch", wireTextOf(t, failed.State), "mkdir /cfg: permission denied", "on: the plugin keeps Docbank running")
	if f := failed.State.Model.SelectedConfigFile(); f == nil || f.Status != model.ConfigMissing {
		t.Errorf("the row changed: %+v", f)
	}
	if again := step(t, failed.State, key("s")); len(again.Cmds) != 1 {
		t.Errorf("s after a failed switch sent %#v, want a new switch", again.Cmds)
	}
}

func TestConfigSDoesNothingOnOtherRows(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigLanes)
	if out := step(t, s, key("s")); len(out.Cmds) != 0 {
		t.Errorf("s on lanes.toml sent %#v", out.Cmds)
	}
}

func TestConfigEnterOnViewsFocusesTheSidebarWhenOneIsShown(t *testing.T) {
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}})
	m := step(t, configOn(t, s, configAllRows(), model.ConfigViews), key("enter")).State.Model
	if m.OnConfig() || m.Picker.Open || m.SidebarLevel != model.MenuViews || m.Focus != model.ColumnCollections {
		t.Errorf("page %v, popup %v, sidebar level %v, focus %v; want the sidebar at the Views level with the keyboard", m.Page, m.Picker.Open, m.SidebarLevel, m.Focus)
	}
}
