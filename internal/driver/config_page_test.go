package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func askedConfigFiles(cmds []model.Cmd) (model.LoadConfigFiles, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadConfigFiles); ok {
			return load, true
		}
	}
	return model.LoadConfigFiles{}, false
}

// configOpened presses K on s and returns the output.
func configOpened(t *testing.T, s driver.State) driver.Output {
	t.Helper()
	out := step(t, s, key("shift+k"))
	if _, ok := askedConfigFiles(out.Cmds); !ok || out.State.Model.Page != model.PageConfig {
		t.Fatalf("K gave page %v and %#v, want the Config Page and a read of the files", out.State.Model.Page, out.Cmds)
	}
	return out
}

var configRowsFixture = []model.ConfigFile{
	{Key: model.ConfigLanes, Title: "lanes.toml", Path: "/cfg/tern-kata/lanes.toml", Status: model.ConfigLoads, Summary: "2 Projects with a Lane list"},
	{Key: model.ConfigFilters, Title: "filters.toml", Path: "/cfg/tern-kata/filters.toml", Summary: "not created: no Project has a default filter"},
	{Key: model.ConfigViews, Title: "views.toml", Path: "/cfg/tern-kata/views.toml", Status: model.ConfigBroken,
		Err: "/cfg/tern-kata/views.toml: toml: line 3: expected value, found newline"},
	{Key: model.ConfigKata, Title: "config.toml (kata's, read-only)", Path: "/kata/config.toml", Status: model.ConfigLoads, Summary: "1 daemon · q asks before quitting"},
}

func configListed(t *testing.T) driver.State {
	t.Helper()
	out := configOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedConfigFiles(out.Cmds)
	return step(t, out.State, model.ConfigFilesLoaded{Request: load.Request, Folder: "/cfg/tern-kata", Files: configRowsFixture}).State
}

func TestConfigPageOpensFromEveryPageAndTheMenuAndShowsInThePageBarWhileShown(t *testing.T) {
	for name, s := range map[string]driver.State{
		"Issues": projectPane(t, 160, boardCard("a")),
		"Board":  boardPane(t, boardCard("a")),
		"Jobs":   jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false))),
	} {
		if pageBarDrawn(t, s.Model, "K") {
			t.Errorf("%s: the Page bar shows K before the Page is open", name)
		}
		out := configOpened(t, s)
		if !pageBarDrawn(t, out.State.Model, "K") || out.State.Model.CurrentMode() != model.ModeConfig {
			t.Errorf("%s: no Page bar entry for K, or mode %q", name, out.State.Model.CurrentMode())
		}
	}
	menu := step(t, projectPane(t, 160, boardCard("a")), openMenu).State
	out := step(t, menu, model.PickerSelected{Key: "page.config"})
	if _, ok := askedConfigFiles(out.Cmds); !ok || out.State.Model.Page != model.PageConfig {
		t.Fatalf("the Menu's Config entry gave page %v and %#v", out.State.Model.Page, out.Cmds)
	}
}

func TestConfigPageBackReturnsToThePageShownBefore(t *testing.T) {
	for _, back := range []string{"escape", "backspace"} {
		out := step(t, configOpened(t, boardPane(t, boardCard("a"))).State, key(back))
		if out.State.Model.Page != model.PageBoard {
			t.Errorf("%s from the Config Page gave page %v, want the Board", back, out.State.Model.Page)
		}
	}
}

func TestConfigPageListsEveryRowWithPathStatusAndSummaryAndTheSelectedRowsFullError(t *testing.T) {
	s := configListed(t)
	wantText(t, "the list", wireTextOf(t, s), "4 files in /cfg/tern-kata",
		"lanes.toml", "/cfg/tern-kata/lanes.toml", "loads", "2 Projects with a Lane list",
		"filters.toml", "missing", "not created: no Project has a default filter",
		"views.toml", "does not load", "config.toml (kata's, read-only)", "/kata/config.toml", "1 daemon · q asks before quitting")
	if text := wireTextOf(t, s); strings.Contains(text, "expected value, found newline") {
		t.Errorf("an error shows before its row is selected: %s", text)
	}
	moved := step(t, step(t, s, key("j")).State, key("j")).State
	wantText(t, "the broken row selected", wireTextOf(t, moved), "/cfg/tern-kata/views.toml: toml: line 3: expected value, found newline")
	if sel, total := moved.Model.Position(); sel != 3 || total != 4 {
		t.Errorf("the cursor is at %d of %d, want 3 of 4", sel, total)
	}
	if sel, _ := step(t, moved, key("g")).State.Model.Position(); sel != 1 {
		t.Errorf("g moved to row %d, want the first", sel)
	}
}

func TestConfigPageSaysWhenTheConfigFolderCannotBeFoundAndListsNoRows(t *testing.T) {
	out := configOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedConfigFiles(out.Cmds)
	wantText(t, "while the read is out", wireTextOf(t, out.State), "loading config files...")
	failed := step(t, out.State, model.ConfigFilesLoaded{Request: load.Request, Err: errors.New("neither $XDG_CONFIG_HOME nor $HOME are defined")}).State
	wantText(t, "a missing folder", wireTextOf(t, failed), "tern-kata's config folder could not be found: neither $XDG_CONFIG_HOME nor $HOME are defined")
	if len(failed.Model.ConfigPage.Files) != 0 {
		t.Errorf("rows listed without a folder: %v", failed.Model.ConfigPage.Files)
	}
}

func TestConfigPageDropsAnAnswerForAnEarlierEntryAndOneStillOutWhenLeft(t *testing.T) {
	first := configOpened(t, projectPane(t, 160, boardCard("a")))
	firstLoad, _ := askedConfigFiles(first.Cmds)

	left := step(t, first.State, key("escape")).State
	late := step(t, left, model.ConfigFilesLoaded{Request: firstLoad.Request, Folder: "/cfg/tern-kata", Files: configRowsFixture}).State
	if len(late.Model.ConfigPage.Files) != 0 || late.Model.ConfigPage.Folder != "" {
		t.Errorf("an answer landed after the Page was left: %+v", late.Model.ConfigPage)
	}

	second := configOpened(t, left)
	secondLoad, _ := askedConfigFiles(second.Cmds)
	if secondLoad.Request == firstLoad.Request {
		t.Fatalf("both entries were numbered %d", firstLoad.Request)
	}
	stale := step(t, second.State, model.ConfigFilesLoaded{Request: firstLoad.Request, Folder: "/old", Files: configRowsFixture}).State
	if len(stale.Model.ConfigPage.Files) != 0 || !stale.Model.ConfigPage.Loading {
		t.Errorf("an answer for an earlier entry landed: %+v", stale.Model.ConfigPage)
	}
	fresh := step(t, stale, model.ConfigFilesLoaded{Request: secondLoad.Request, Folder: "/cfg/tern-kata", Files: configRowsFixture[:1]}).State
	if len(fresh.Model.ConfigPage.Files) != 1 {
		t.Errorf("the current answer was dropped: %+v", fresh.Model.ConfigPage)
	}
}
