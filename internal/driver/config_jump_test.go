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

// configAllRows is every row the Config Page lists, in its order, the Docbank row as it reads with no [docbank] section.
func configAllRows() []model.ConfigFile {
	enter := []model.ConfigAction{model.ConfigOpen}
	return []model.ConfigFile{
		{Key: model.ConfigLanes, Title: "lanes.toml", Path: "/cfg/tern-kata/lanes.toml", Actions: enter},
		{Key: model.ConfigFilters, Title: "filters.toml", Path: "/cfg/tern-kata/filters.toml", Actions: enter},
		{Key: model.ConfigViews, Title: "views.toml", Path: "/cfg/tern-kata/views.toml", Actions: enter},
		{Key: model.ConfigSeen, Title: "seen.toml", Path: "/cfg/tern-kata/seen.toml", Actions: enter},
		{Key: model.ConfigWorker, Title: "worker.toml", Path: "/cfg/tern-kata/worker.toml", Actions: enter},
		{Key: model.ConfigAgent, Title: "agent.toml", Path: "/cfg/tern-kata/agent.toml", Actions: []model.ConfigAction{model.ConfigEdit}},
		{Key: model.ConfigDocbank, Title: "Docbank (the [docbank] section of kata's config.toml)", Path: "/kata/config.toml", Status: model.ConfigLoads,
			Summary: "not set up: `kata docbank setup` sets Docbank up"},
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

func TestConfigEnterOnSeenOpensTheRunsPage(t *testing.T) {
	m := step(t, configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigSeen), key("enter")).State.Model
	if m.Page != model.PageRuns {
		t.Fatalf("page %v, want the Runs Page", m.Page)
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
	for key_, want := range map[model.ConfigFileKey]string{model.ConfigLanes: "Board needs a Project", model.ConfigWorker: "Jobs Page needs a Project", model.ConfigSeen: "Runs Page needs a Project"} {
		m := step(t, configOn(t, unbound(), configAllRows(), key_), key("enter")).State.Model
		if m.Page != model.PageIssues || !strings.Contains(m.Notice, want) || m.LanesForm.Open || m.WorkerCard.Open {
			t.Errorf("%s: page %v, notice %q, Lanes card %v, Worker card %v", key_, m.Page, m.Notice, m.LanesForm.Open, m.WorkerCard.Open)
		}
	}
}

func TestConfigFooterAndHelpSheetNameWhatEnterOpensAndNothingForAgent(t *testing.T) {
	labels := map[model.ConfigFileKey]string{
		model.ConfigLanes: "open Lanes card", model.ConfigFilters: "open Filter card", model.ConfigViews: "open Views",
		model.ConfigWorker: "open Worker card", model.ConfigSeen: "open Runs Page", model.ConfigKata: "open Daemons Page",
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

func TestConfigDocbankRowShowsEachFormsTextAndOffersNoS(t *testing.T) {
	for name, tc := range map[string]struct {
		row  model.ConfigFile
		want []string
	}{
		"not set up": {model.ConfigFile{Status: model.ConfigMissing, Summary: "not set up: `kata docbank setup` sets Docbank up"},
			[]string{"missing", "not set up: `kata docbank setup` sets Docbank up"}},
		"local": {model.ConfigFile{Status: model.ConfigLoads, Summary: "the Docbank on this machine: kata starts it when needed and follows its port and key"},
			[]string{"loads", "the Docbank on this machine: kata starts it when needed and follows its port and key"}},
		"url": {model.ConfigFile{Status: model.ConfigLoads, Summary: "Docbank at https://docbank.example"},
			[]string{"loads", "Docbank at https://docbank.example"}},
		"old pinned form": {model.ConfigFile{Status: model.ConfigLoads, Summary: "pinned to http://127.0.0.1:8080 by tern-kata's old setup: kata does not start it"},
			[]string{"loads", "pinned to http://127.0.0.1:8080 by tern-kata's old setup: kata does not start it"}},
		"unreadable": {model.ConfigFile{Status: model.ConfigBroken, Err: "read /kata/config.toml: is a directory"},
			[]string{"does not load", "read /kata/config.toml: is a directory"}},
	} {
		rows := configAllRows()
		i := slices.IndexFunc(rows, func(f model.ConfigFile) bool { return f.Key == model.ConfigDocbank })
		tc.row.Key, tc.row.Title, tc.row.Path = rows[i].Key, rows[i].Title, rows[i].Path
		rows[i] = tc.row
		s := configOn(t, projectPane(t, 160, boardCard("a")), rows, model.ConfigDocbank)
		text := wireTextOf(t, s)
		wantText(t, name, text, append(tc.want, "/kata/config.toml", "the [docbank] section of kata's config.toml")...)
		if out := step(t, s, key("s")); len(out.Cmds) != 0 || out.State.Model.ConfigPage.Message != "" || out.State.Model.ConfigPage.AskSwitch {
			t.Errorf("%s: s sent %#v and said %q", name, out.Cmds, out.State.Model.ConfigPage.Message)
		}
		if strings.Contains(text, "supervision") {
			t.Errorf("%s: the row still speaks of supervision: %s", name, text)
		}
		if out := step(t, s, key("enter")); len(out.Cmds) != 0 || out.State.Model.Page != model.PageConfig {
			t.Errorf("%s: enter did %#v on page %v", name, out.Cmds, out.State.Model.Page)
		}
	}
}

func TestConfigSDoesNothingOnOtherRows(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigLanes)
	if out := step(t, s, key("s")); len(out.Cmds) != 0 {
		t.Errorf("s on lanes.toml sent %#v", out.Cmds)
	}
}

// pinnedDocbankRows is every row with the Docbank row in tern-kata's old pinned form, which is the one that offers the switch.
func pinnedDocbankRows() []model.ConfigFile {
	rows := configAllRows()
	i := slices.IndexFunc(rows, func(f model.ConfigFile) bool { return f.Key == model.ConfigDocbank })
	rows[i].Summary = "pinned to http://127.0.0.1:8080 by tern-kata's old setup: kata does not start it"
	rows[i].Actions = []model.ConfigAction{model.ConfigSwitchDocbank}
	return rows
}

func TestConfigSOnThePinnedDocbankRowAsksAndNSendsNothing(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), pinnedDocbankRows(), model.ConfigDocbank)
	asked := step(t, s, key("s"))
	if len(asked.Cmds) != 0 || !asked.State.Model.ConfigPage.AskSwitch || asked.State.Model.CurrentMode() != model.ModeSwitchDocbank {
		t.Fatalf("s sent %#v, asking %v, mode %v", asked.Cmds, asked.State.Model.ConfigPage.AskSwitch, asked.State.Model.CurrentMode())
	}
	wantText(t, "question", wireTextOf(t, asked.State), "Switch to kata's local Docbank? kata restarts its daemon. y/n")
	kept := step(t, asked.State, key("n"))
	if len(kept.Cmds) != 0 || kept.State.Model.ConfigPage.AskSwitch || kept.State.Model.ConfigPage.Switching || kept.State.Model.ConfigPage.Message != "" {
		t.Errorf("n sent %#v, state %+v", kept.Cmds, kept.State.Model.ConfigPage)
	}
}

func TestConfigYSendsTheSwitchAndIgnoresSWhileItIsOut(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), pinnedDocbankRows(), model.ConfigDocbank)
	sent := step(t, step(t, s, key("s")).State, key("y"))
	if want := []model.Cmd{model.SwitchDocbank{}}; !equalCmds(sent.Cmds, want) {
		t.Fatalf("y sent %#v, want %#v", sent.Cmds, want)
	}
	if c := sent.State.Model.ConfigPage; c.AskSwitch || !c.Switching || c.Message == "" {
		t.Errorf("state after y: %+v", c)
	}
	if again := step(t, sent.State, key("s")); len(again.Cmds) != 0 || again.State.Model.ConfigPage.AskSwitch {
		t.Errorf("s while the switch is out sent %#v, asking %v", again.Cmds, again.State.Model.ConfigPage.AskSwitch)
	}
}

func TestConfigSwitchSuccessReadsTheRowsAgainAndSaysTheKeyFilesCanGo(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), pinnedDocbankRows(), model.ConfigDocbank)
	sent := step(t, step(t, s, key("s")).State, key("y"))
	done := step(t, sent.State, model.DocbankSwitched{})
	if !hasCmd[model.LoadConfigFiles](done.Cmds) {
		t.Errorf("success sent %#v, want the rows read again", done.Cmds)
	}
	c := done.State.Model.ConfigPage
	if c.Switching || c.MessageErr || !strings.Contains(c.Message, "docbank.key") || !strings.Contains(c.Message, "docbank.manual") {
		t.Errorf("state after success: %+v", c)
	}
}

func TestConfigSwitchFailureShowsKatasLineAndKeepsTheRows(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), pinnedDocbankRows(), model.ConfigDocbank)
	sent := step(t, step(t, s, key("s")).State, key("y"))
	failed := step(t, sent.State, model.DocbankSwitched{Err: errors.New("docbank is not installed")})
	if len(failed.Cmds) != 0 {
		t.Errorf("failure sent %#v, want no read", failed.Cmds)
	}
	c := failed.State.Model.ConfigPage
	if c.Switching || !c.MessageErr || !strings.Contains(c.Message, "docbank is not installed") || len(c.Files) != len(pinnedDocbankRows()) {
		t.Errorf("state after failure: %+v", c)
	}
	if again := step(t, failed.State, key("s")); !again.State.Model.ConfigPage.AskSwitch {
		t.Error("s does not ask again after a failure")
	}
}

func TestConfigFooterAndHelpSheetListSOnlyOnThePinnedDocbankRow(t *testing.T) {
	hasS := func(m model.Model) (footer, sheet bool) {
		for b := range model.BindingsForMode(m.CurrentMode(), true) {
			if b.Chord == "s" && b.Offered(m) {
				footer = true
			}
		}
		for _, g := range model.HelpSheetOn(model.PageConfig, m) {
			for _, l := range g.Lines {
				if slices.Contains(l.Keys, "s") {
					sheet = true
				}
			}
		}
		return
	}
	on := configOn(t, projectPane(t, 160, boardCard("a")), pinnedDocbankRows(), model.ConfigDocbank).Model
	if footer, sheet := hasS(on); !footer || !sheet {
		t.Errorf("on the pinned row: footer %v, help sheet %v, want both", footer, sheet)
	}
	for _, key_ := range []model.ConfigFileKey{model.ConfigLanes, model.ConfigAgent, model.ConfigKata} {
		if footer, sheet := hasS(configOn(t, projectPane(t, 160, boardCard("a")), pinnedDocbankRows(), key_).Model); footer || sheet {
			t.Errorf("on %s: footer %v, help sheet %v, want neither", key_, footer, sheet)
		}
	}
	other := configOn(t, projectPane(t, 160, boardCard("a")), configAllRows(), model.ConfigDocbank).Model
	if footer, sheet := hasS(other); footer || sheet {
		t.Errorf("on another Docbank form: footer %v, help sheet %v, want neither", footer, sheet)
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
