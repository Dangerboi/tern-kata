package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestPageBarShowsIssuesAndBoardAndKeysAndClicksSwitchPages(t *testing.T) {
	s := projectPane(t, 160, boardCard("a"), boardCard("b"))
	for _, entry := range []struct{ name, chord string }{{"issues", "1"}, {"board", "2"}} {
		if _, ok := pageBarEntryAt(t, s.Model, entry.chord); !ok {
			t.Errorf("the Page bar draws no %s entry", entry.name)
		}
	}
	for name, to := range map[string]func(driver.State) driver.State{
		"key": func(s driver.State) driver.State {
			return feed(t, s, key("2"), model.LanesLoaded{ProjectUID: "p"}).State
		},
		"click": func(s driver.State) driver.State {
			return feed(t, s, click(pageBarEntry(t, s.Model, "2"), view.KeyClick("2")), model.LanesLoaded{ProjectUID: "p"}).State
		},
		"b": func(s driver.State) driver.State {
			return feed(t, s, key("b"), model.LanesLoaded{ProjectUID: "p"}).State
		},
	} {
		board := to(s)
		if board.Model.Page != model.PageBoard {
			t.Fatalf("%s: page %v, want the Board Page", name, board.Model.Page)
		}
		if heads, _ := lanes(t, board); len(heads) == 0 {
			t.Errorf("%s: the Board is not drawn", name)
		}
		for back, press := range map[string]any{"1": key("1"), "b": key("b"), "click": click(pageBarEntry(t, board.Model, "1"), view.KeyClick("1"))} {
			issues := step(t, board, press).State
			if issues.Model.Page != model.PageIssues {
				t.Errorf("%s then %s: page %v, want the Issues Page", name, back, issues.Model.Page)
			}
		}
	}
}

func TestEachPageKeepsItsSelectionWhileAnotherIsShown(t *testing.T) {
	s := projectPane(t, 160, updatedAt(boardCard("a"), 9), boardCard("b"), boardCard("c"))
	s = feed(t, s, key("j"), key("j")).State
	if s.Model.SelectedUID != "c" {
		t.Fatalf("fixture: selected %q", s.Model.SelectedUID)
	}
	board := feed(t, s, key("2"), model.LanesLoaded{ProjectUID: "p"}).State
	if board.Model.SelectedUID != "c" {
		t.Errorf("the Board Page selects %q, want c", board.Model.SelectedUID)
	}
	back := feed(t, board, key("1")).State
	if back.Model.SelectedUID != "c" || !rowOfDrawn(t, back.Model, view.IssueListID, "c") {
		t.Errorf("the Issues Page selects %q and does not draw it", back.Model.SelectedUID)
	}
}

func TestBoardPageShowsTheBindingsProjectFromAFixedCollection(t *testing.T) {
	s := projectPane(t, 160, boardCard("a"))
	s = feed(t, s, model.CollectionSelected{Collection: model.FixedCollections()[0]}).State
	if s.Model.Collection.IsProject() {
		t.Fatal("fixture: still in the Project Collection")
	}
	out := step(t, s, key("2"))
	m := out.State.Model
	if m.Page != model.PageBoard || m.Collection.ProjectUID != "p" || len(out.Cmds) == 0 {
		t.Errorf("page %v, Collection %+v, commands %v; want the Board Page of the Binding's Project with its Issues loading", m.Page, m.Collection, out.Cmds)
	}
}

func TestBoardPageWithoutAProjectSaysSoAndOpensTheMenusProjects(t *testing.T) {
	s := loadedWith(t, 160, model.Project{}, model.Snapshot{Issues: []model.Issue{alpha}})
	for _, press := range []string{"2", "b"} {
		m := step(t, s, key(press)).State.Model
		if m.Page != model.PageIssues || !strings.Contains(m.Notice, "Project") || m.Focus != model.ColumnCollections || !m.SidebarProjects {
			t.Errorf("%s: page %v, notice %q, focus %v, Projects level %v", press, m.Page, m.Notice, m.Focus, m.SidebarProjects)
		}
	}
}

func TestEveryPageBarEntryClicksLikeItsKey(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnCollections, model.ColumnDetail} {
		s := clickAt(focus)
		for _, entry := range s.Model.PageBar() {
			keyboard, keyCmds := keys(entry.Chord)(s)
			pointer, clickCmds := clicked(s, click(pageBarEntry(t, s.Model, entry.Chord), view.KeyClick(entry.Chord)))
			sameAs(t, entry.Label+" entry differs from its key", pointer, clickCmds, keyboard, keyCmds)
		}
	}
}

// pageBarTexts is what each Page entry of the top bar draws, by entry id, wherever the Page bar sits.
func pageBarTexts(t *testing.T, s driver.State) map[string]string {
	t.Helper()
	texts := map[string]string{}
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		for _, entry := range s.Model.PageBar() {
			if id, ok := pageBarEntryAt(t, s.Model, entry.Chord); ok && n.ID == id {
				texts[entry.ID] = wireText(n)
			}
		}
	}
	return texts
}

func TestPageBarCompactsThenMovesToItsOwnLineBeforeAnythingLeavesTheTopBar(t *testing.T) {
	long := model.Project{ID: 7, UID: "p", Name: "a-rather-long-project-name-for-the-top-bar"}
	for _, tt := range []struct {
		cols      int
		compact   bool
		ownLine   bool
		wantLabel string
	}{
		{200, false, false, "Workflows"},
		{100, true, false, ""},
		{80, true, true, ""},
	} {
		s := loadedWith(t, tt.cols, long, model.Snapshot{Issues: []model.Issue{alpha}})
		texts := pageBarTexts(t, s)
		if len(texts) != len(s.Model.PageBar()) {
			t.Fatalf("%d columns: %d Page bar entries drawn, want %d", tt.cols, len(texts), len(s.Model.PageBar()))
		}
		if got := strings.Contains(texts["workflows"], `"t":"Workflows"`); got == tt.compact {
			t.Errorf("%d columns: Workflows entry %s, compact %v", tt.cols, texts["workflows"], tt.compact)
		}
		if !strings.Contains(texts["issues"], `"t":"Issues"`) {
			t.Errorf("%d columns: the current Page lost its label: %s", tt.cols, texts["issues"])
		}
		_, own := drawnIDs(t, view.Render(s.Model, now))["main.pagebar"]
		if own != tt.ownLine {
			t.Errorf("%d columns: Page bar on its own line = %v, want %v", tt.cols, own, tt.ownLine)
		}
	}
}
