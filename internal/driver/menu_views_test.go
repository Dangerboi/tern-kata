package driver_test

import (
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

var fixedCollections = []string{"Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook"}

func menuCatalog() []model.CatalogProject {
	return []model.CatalogProject{
		{Project: model.Project{ID: 7, UID: "p", Name: "work"}},
		{Project: model.Project{ID: 8, UID: "q", Name: "other"}},
	}
}

// boundMenu is a Pane bound to the Project "work", with the Menu open at its top level.
func boundMenu(t *testing.T) driver.State {
	t.Helper()
	s := loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"}, model.Snapshot{Issues: []model.Issue{alpha}, Catalog: menuCatalog()})
	return step(t, s, openMenu).State
}

func menuTitles(s driver.State) []string {
	var titles []string
	for _, entry := range s.Model.PickerEntries() {
		titles = append(titles, entry.Title)
	}
	return titles
}

// rowValue is the text drawn on the right of the Menu row with the given node id.
func rowValue(t *testing.T, m model.Model, id string) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if n.ID == id {
			value, _ := n.P["value"].(string)
			return value
		}
	}
	t.Fatalf("no node %s", id)
	return ""
}

func cursorTitle(s driver.State) string {
	return s.Model.PickerEntries()[s.Model.Picker.Cursor].Title
}

func TestMenuTopLevelListsTheProjectViewsProjectsPagesRestoreAndNewProject(t *testing.T) {
	want := []string{"work", "Views", "Projects", "Credentials", "Federation", "Daemons", "Restore a deleted Issue", "New Project"}
	if got := menuTitles(boundMenu(t)); !slices.Equal(got, want) {
		t.Fatalf("top level = %q, want %q", got, want)
	}
}

func TestMenuViewsOpensBackAndTheSixCollectionsByEnterAndClick(t *testing.T) {
	top := boundMenu(t)
	want := append(append([]string{"Back"}, fixedCollections...), model.NewViewTitle)
	byEnter := pressed(top, "down", "enter")
	byClick := step(t, top, &tern.Select{ID: view.PickerListID, Item: rowShowing(t, top.Model, view.PickerListID, "Views")}).State
	for name, s := range map[string]driver.State{"enter": byEnter, "click": byClick} {
		if got := menuTitles(s); s.Model.Picker.Level != model.MenuViews || !slices.Equal(got, want) {
			t.Errorf("%s: level %v entries %q, want the Views level %q", name, s.Model.Picker.Level, got, want)
		}
	}
	for i, title := range fixedCollections {
		opened := pressed(byEnter, slices.Repeat([]string{"down"}, i+1)...)
		opened = pressed(opened, "enter")
		if opened.Model.Picker.Open || opened.Model.Collection.Title != title || opened.Model.Collection.IsProject() {
			t.Errorf("%s: Menu open %v, Collection %+v", title, opened.Model.Picker.Open, opened.Model.Collection)
		}
	}
	clicked := step(t, byEnter, &tern.Select{ID: view.PickerListID, Item: rowShowing(t, byEnter.Model, view.PickerListID, "Today")}).State
	if clicked.Model.Picker.Open || clicked.Model.Collection.View != "today" {
		t.Errorf("click on Today: open %v, Collection %+v", clicked.Model.Picker.Open, clicked.Model.Collection)
	}
}

func TestMenuLeavesTheViewsLevelOnViewsByBackEscAndBackspace(t *testing.T) {
	views := pressed(boundMenu(t), "down", "enter")
	clickBack := step(t, views, &tern.Select{ID: view.PickerListID, Item: rowShowing(t, views.Model, view.PickerListID, "Back")}).State
	for name, s := range map[string]driver.State{
		"esc":           pressed(views, "escape"),
		"backspace":     pressed(views, "backspace"),
		"enter on Back": pressed(views, "enter"),
		"click on Back": clickBack,
	} {
		if !s.Model.Picker.Open || s.Model.Picker.Level != model.MenuTop || cursorTitle(s) != "Views" {
			t.Errorf("%s: open %v level %v cursor on %q, want the top level on Views", name, s.Model.Picker.Open, s.Model.Picker.Level, cursorTitle(s))
		}
	}
	if closed := pressed(pressed(views, "escape"), "escape"); closed.Model.Picker.Open {
		t.Error("esc on the top level left the Menu open")
	}
}

func TestMenuPAndIStillOpenProjectsAndInbox(t *testing.T) {
	bound := loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"}, model.Snapshot{Issues: []model.Issue{alpha}, Catalog: menuCatalog()})
	if got := pressed(bound, "shift+p").Model; !got.Picker.Open || got.Picker.Level != model.MenuProjects {
		t.Errorf("P: Menu open %v level %v", got.Picker.Open, got.Picker.Level)
	}
	if got := pressed(bound, "shift+i").Model; got.Collection != model.InboxCollection() {
		t.Errorf("I: Collection %+v", got.Collection)
	}
}

func TestMenuFilterAtTheTopFindsCollectionsProjectsAndEntries(t *testing.T) {
	top := boundMenu(t)
	for filter, want := range map[string]string{
		"tod":  "Today",
		"inb":  "Inbox",
		"upc":  "Upcoming",
		"dead": "Deadlines",
		"all":  "All Open",
		"logb": "Logbook",
		"oth":  "other",
		"cred": "Credentials",
		"rest": "Restore a deleted Issue",
		"new":  "New Project",
		"view": "Views",
	} {
		typed := top
		for _, r := range filter {
			typed = pressed(typed, string(r))
		}
		if got := menuTitles(typed); !slices.Contains(got, want) || typed.Model.Picker.Level != model.MenuTop {
			t.Errorf("typing %q gave %q, want it to hold %q", filter, got, want)
		}
	}
	typed := pressed(top, "t", "o", "d")
	if got := menuTitles(typed); !slices.Equal(got, []string{"Today"}) {
		t.Fatalf("tod = %q, want only Today", got)
	}
	if opened := pressed(typed, "enter"); opened.Model.Collection.View != "today" || opened.Model.Picker.Open {
		t.Errorf("enter on the match: Collection %+v open %v", opened.Model.Collection, opened.Model.Picker.Open)
	}
}

func TestMenuViewsRowNamesTheCurrentFixedCollectionAndTakesTheCursor(t *testing.T) {
	top := boundMenu(t)
	if got := rowValue(t, top.Model, view.PickerListID+".views"); got != "" {
		t.Fatalf("with a Project current the Views row names %q", got)
	}
	if cursorTitle(top) != "work" {
		t.Fatalf("with a Project current the cursor is on %q, want the pinned Project", cursorTitle(top))
	}
	today := pressed(top, "down", "enter", "down", "down", "enter")
	if today.Model.Collection.View != "today" {
		t.Fatalf("fixture: Collection %+v", today.Model.Collection)
	}
	reopened := step(t, today, openMenu).State
	if got := rowValue(t, reopened.Model, view.PickerListID+".views"); got != "Today" {
		t.Errorf("Views row value = %q, want Today", got)
	}
	if cursorTitle(reopened) != "Views" || reopened.Model.Picker.Level != model.MenuTop {
		t.Errorf("with Today current the Menu opened on %q at level %v, want Views at the top", cursorTitle(reopened), reopened.Model.Picker.Level)
	}
	inViews := pressed(reopened, "enter")
	if cursorTitle(inViews) != "Today" {
		t.Errorf("the Views level opened with the cursor on %q, want Today", cursorTitle(inViews))
	}
}

func TestUnboundSidebarShowsTheViewsLevel(t *testing.T) {
	s := loadedWith(t, 160, model.Project{}, model.Snapshot{Issues: []model.Issue{alpha}, Catalog: menuCatalog()})
	if s.Model.SidebarLevel != model.MenuViews {
		t.Fatalf("sidebar level %v, want Views", s.Model.SidebarLevel)
	}
	var titles []string
	for _, entry := range s.Model.SidebarEntries() {
		titles = append(titles, entry.Title)
	}
	if want := append(append([]string{"Back"}, fixedCollections...), model.NewViewTitle); !slices.Equal(titles, want) {
		t.Fatalf("sidebar rows = %q, want %q", titles, want)
	}
	for _, title := range append([]string{"Back"}, fixedCollections...) {
		rowShowing(t, s.Model, view.CollectionsID, title)
	}
	top := pressed(s, "escape", "escape")
	if top.Model.SidebarLevel != model.MenuTop {
		t.Fatalf("esc did not leave the Views level: %v", top.Model.SidebarLevel)
	}
	rowShowing(t, top.Model, view.CollectionsID, "Views")
	projects := pressed(top, "down", "enter")
	if projects.Model.SidebarLevel != model.MenuProjects {
		t.Fatalf("Projects entry: level %v", projects.Model.SidebarLevel)
	}
	if back := pressed(projects, "home", "enter"); back.Model.SidebarLevel != model.MenuTop {
		t.Fatalf("Back on the Projects level: %v", back.Model.SidebarLevel)
	}
}
