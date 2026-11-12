package driver_test

import (
	"slices"
	"strings"
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
	want := []string{"work", "Views", "Projects", "Credentials", "Federation", "Daemons", "Config", "Restore a deleted Issue", "New Project"}
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

var mineView = model.SavedView{Name: "Mine", Base: model.Collection{View: "today", Title: "Today"}, Filters: model.Filters{Status: "open"}}

// withSavedViews is s after the Pane read views.toml holding views.
func withSavedViews(t *testing.T, s driver.State, views ...model.SavedView) driver.State {
	t.Helper()
	return step(t, s, model.ViewsLoaded{Views: views, Token: "read-1"}).State
}

// savedViewsMenu is boundMenu with three saved views, on its Views level.
func savedViewsMenu(t *testing.T) driver.State {
	t.Helper()
	views := []model.SavedView{
		{Name: "work bugs", Base: model.ProjectCollection("p", "work"), Filters: model.Filters{Label: "bug"}},
		mineView,
		{Name: "Lost", Base: model.ProjectCollection("gone-uid", "gone")},
	}
	return pressed(withSavedViews(t, boundMenu(t), views...), "down", "enter")
}

// menuCursorOn moves the Menu cursor down to the row titled title.
func menuCursorOn(t *testing.T, s driver.State, title string) driver.State {
	t.Helper()
	for range 40 {
		if cursorTitle(s) == title {
			return s
		}
		s = pressed(s, "down")
	}
	t.Fatalf("no Menu row %q in %q", title, menuTitles(s))
	return s
}

func savedRowID(t *testing.T, m model.Model, name string) string {
	t.Helper()
	for _, e := range m.PickerEntries() {
		if e.Kind == model.EntrySavedView && e.Title == name {
			return view.PickerItemID(e.Key())
		}
	}
	t.Fatalf("no saved view row %q", name)
	return ""
}

func TestMenuViewsLevelListsSavedViewsByNameAfterTheFixedCollectionsAndNamesAGoneProject(t *testing.T) {
	s := savedViewsMenu(t)
	want := append(append([]string{"Back"}, fixedCollections...), "Lost", "Mine", "work bugs", model.NewViewTitle)
	if got := menuTitles(s); !slices.Equal(got, want) {
		t.Fatalf("Views level = %q, want %q", got, want)
	}
	rowShowing(t, s.Model, view.PickerListID, "Lost (Project gone)")
	rowShowing(t, s.Model, view.PickerListID, "Mine")
	if got := rowsShowing(t, s.Model, view.PickerListID, "work bugs (Project gone)"); len(got) != 0 {
		t.Errorf("a live Project's view says it is gone: %v", got)
	}
}

func TestMenuF2EditsTheSavedViewUnderTheCursor(t *testing.T) {
	s := menuCursorOn(t, savedViewsMenu(t), "Mine")
	edit := step(t, s, key("f2")).State
	form := edit.Model.ViewForm
	if !form.Open || form.Self != "Mine" || form.Name != "Mine" || form.Base.View != "today" || form.Values.Status != "open" {
		t.Fatalf("f2 opened %+v", form)
	}
	if !drawnIDs(t, view.Render(edit.Model, now))[view.LayerID(model.CardView)] {
		t.Error("the View card is not drawn")
	}
	onFixed := step(t, menuCursorOn(t, savedViewsMenu(t), "Today"), key("f2")).State
	if onFixed.Model.ViewForm.Open {
		t.Error("f2 on a fixed Collection opened the View card")
	}
}

func TestMenuCtrlDAsksBeforeDeletingAndOnlyYSendsTheDelete(t *testing.T) {
	asked := step(t, menuCursorOn(t, savedViewsMenu(t), "Mine"), key("ctrl+d")).State
	if asked.Model.Views.Deleting != "Mine" {
		t.Fatalf("ctrl+d asked about %q", asked.Model.Views.Deleting)
	}
	question := ""
	for id := range drawnIDs(t, view.Render(asked.Model, now)) {
		if strings.HasSuffix(id, "delete-view") {
			question = id
		}
	}
	if question == "" {
		t.Error("the delete question is not drawn")
	}
	for _, chord := range []string{"n", "escape"} {
		out := step(t, asked, key(chord))
		if out.State.Model.Views.Deleting != "" || len(out.Cmds) != 0 || !out.State.Model.Picker.Open {
			t.Errorf("%s: asked %q, cmds %v, menu open %v", chord, out.State.Model.Views.Deleting, out.Cmds, out.State.Model.Picker.Open)
		}
	}
	if other := step(t, asked, key("x")); other.State.Model.Views.Deleting != "Mine" || len(other.Cmds) != 0 {
		t.Errorf("another key answered the question: %q, %v", other.State.Model.Views.Deleting, other.Cmds)
	}
	yes := step(t, asked, key("y"))
	if len(yes.Cmds) != 1 || yes.Cmds[0] != (model.DeleteView{Name: "Mine", Token: "read-1"}) {
		t.Fatalf("y sent %#v", yes.Cmds)
	}
	fixed := step(t, menuCursorOn(t, savedViewsMenu(t), "Today"), key("ctrl+d")).State
	if fixed.Model.Views.Deleting != "" {
		t.Error("ctrl+d on a fixed Collection asked to delete")
	}
}

func TestMenuViewsRowNamesTheOpenedSavedViewUntilAnotherCollectionIsChosen(t *testing.T) {
	opened := pressed(menuCursorOn(t, savedViewsMenu(t), "Mine"), "enter")
	if opened.Model.Picker.Open || opened.Model.Collection.View != "today" || opened.Model.Filters.Status != "open" {
		t.Fatalf("opening Mine: menu open %v, Collection %+v, filters %+v", opened.Model.Picker.Open, opened.Model.Collection, opened.Model.Filters)
	}
	reopened := step(t, opened, openMenu).State
	if got := rowValue(t, reopened.Model, view.PickerListID+".views"); got != "Mine" {
		t.Errorf("Views row = %q, want the saved view Mine", got)
	}
	inViews := pressed(reopened, "enter")
	if got := rowValue(t, inViews.Model, savedRowID(t, inViews.Model, "Mine")); got != "current" {
		t.Errorf("Mine row value = %q, want current", got)
	}

	// Inbox is one row above Today, the Collection the view opened, and the filters stay.
	inbox := pressed(inViews, "up", "enter")
	if inbox.Model.Collection != model.InboxCollection() || inbox.Model.Filters.Status != "open" {
		t.Fatalf("fixture: Collection %+v, filters %+v", inbox.Model.Collection, inbox.Model.Filters)
	}
	if got := filterLineText(t, inbox); strings.Contains(got, "Mine") {
		t.Errorf("filter line %q still names the view", got)
	}
	again := step(t, inbox, openMenu).State
	if got := rowValue(t, again.Model, view.PickerListID+".views"); got != "Inbox" {
		t.Errorf("Views row = %q after choosing Inbox, want the fixed Collection Inbox", got)
	}
	back := pressed(again, "enter")
	if got := rowValue(t, back.Model, savedRowID(t, back.Model, "Mine")); got != "" {
		t.Errorf("Mine row value = %q after choosing Inbox, want none", got)
	}
}

// The Menu is modal, so its delete question carries its own Delete and Keep buttons, which a phone answers it with.
func TestMenuDeleteQuestionsButtonsDeleteAndKeep(t *testing.T) {
	asked := step(t, menuCursorOn(t, savedViewsMenu(t), "Mine"), key("ctrl+d")).State
	buttons := map[string]tern.WireNode{}
	for _, n := range wireNodes(t, view.Render(asked.Model, now)) {
		if _, ok := n.P["actions"].(map[string]any); ok && strings.Contains(n.ID, "delete-view.") {
			buttons[n.ID[strings.LastIndex(n.ID, ".")+1:]] = n
		}
	}
	press := func(answer string) driver.Output {
		n, ok := buttons[answer]
		if !ok {
			t.Fatalf("the Menu draws no %s button for its delete question", answer)
		}
		if !strings.HasPrefix(n.ID, view.LayerID(model.CardPicker)+".") {
			t.Errorf("the %s button %s is not in the Menu", answer, n.ID)
		}
		action, _ := n.P["actions"].(map[string]any)["click"].(string)
		return driver.Step(asked, click(n.ID, action), now)
	}
	if yes := press("yes"); len(yes.Cmds) != 1 || yes.Cmds[0] != (model.DeleteView{Name: "Mine", Token: "read-1"}) {
		t.Errorf("Delete sent %#v", yes.Cmds)
	}
	if no := press("no"); no.State.Model.Views.Deleting != "" || len(no.Cmds) != 0 || !no.State.Model.Picker.Open {
		t.Errorf("Keep: asked %q, cmds %v, menu open %v", no.State.Model.Views.Deleting, no.Cmds, no.State.Model.Picker.Open)
	}
}
