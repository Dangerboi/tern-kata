package model_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

type testToken struct{ n int }

func savedViews() []model.SavedView {
	return []model.SavedView{
		{Name: "work bugs", Base: model.ProjectCollection("alpha-uid", "alpha"), Filters: model.Filters{Status: "open", Label: "bug"}},
		{Name: "Mine", Base: model.Collection{View: "today", Title: "Today"}, Filters: model.Filters{Owner: model.OwnerMe}},
		{Name: "Old", Base: model.ProjectCollection("gone-uid", "gone")},
	}
}

func withViews(m model.Model, views ...model.SavedView) model.Model {
	m, _ = model.Update(m, model.ViewsLoaded{Views: views, Token: testToken{1}})
	return m
}

func viewsPane() model.Model {
	m := withViews(pickerStart(120, true), savedViews()...)
	m.Actor = "ann"
	return m
}

func TestViewsLevelListsSavedViewsByNameAfterTheFixedCollections(t *testing.T) {
	m := viewsPane()
	m, _ = menuKeys(m, "down", "enter")
	want := []string{"Back", "Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook", "Mine", "Old", "work bugs", "New view"}
	if got := pickerTitles(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("Views level = %q, want %q", got, want)
	}
	found, _ := menuKeys(viewsPane(), "m", "i", "n", "e")
	if got := pickerTitles(found); !reflect.DeepEqual(got, []string{"Mine"}) {
		t.Fatalf("filter mine = %q", got)
	}
}

func TestSavedViewRowSaysWhenItsProjectIsGoneOrArchived(t *testing.T) {
	m := viewsPane()
	catalog := append(pickerCatalog(), model.CatalogProject{Project: model.Project{UID: "arch-uid", Name: "arch", Archived: true}})
	m.Snapshot.Catalog = catalog
	m = withViews(m, append(savedViews(), model.SavedView{Name: "Shelved", Base: model.ProjectCollection("arch-uid", "arch")})...)
	got := map[string]string{}
	for _, e := range m.SidebarEntries() {
		got[e.Title] = m.EntryTitle(e)
	}
	if got["Old"] != "Old (Project gone)" || got["Shelved"] != "Shelved (archived)" || got["Mine"] != "Mine" {
		t.Fatalf("row titles = %q", got)
	}
	for _, name := range []string{"Old", "Shelved"} {
		v, _ := findView(m, name)
		after, cmds := openRow(m, v)
		if len(cmds) > 1 || after.Collection != m.Collection || after.Filters != m.Filters || !strings.Contains(after.Notice, name) {
			t.Errorf("%s: cmds=%v collection=%+v filters=%+v notice=%q", name, cmds, after.Collection, after.Filters, after.Notice)
		}
	}
}

func findView(m model.Model, name string) (model.PickerEntry, bool) {
	for _, e := range m.SidebarEntries() {
		if e.Kind == model.EntrySavedView && e.Title == name {
			return e, true
		}
	}
	return model.PickerEntry{}, false
}

// openRow is enter on the saved view's row of the sidebar.
func openRow(m model.Model, e model.PickerEntry) (model.Model, []model.Cmd) {
	return model.Update(m, model.CollectionPointerSelected{Key: e.Key()})
}

func TestOpeningASavedViewSetsBaseAndFiltersTogetherAndNamesItOnTheFilterLine(t *testing.T) {
	m := viewsPane()
	e, _ := findView(m, "work bugs")
	opened, cmds := openRow(m, e)
	if opened.Collection.ProjectUID != "alpha-uid" || opened.Filters != (model.Filters{Status: "open", Label: "bug"}) {
		t.Fatalf("opened: collection=%+v filters=%+v", opened.Collection, opened.Filters)
	}
	var read *model.ReadSnapshot
	for _, c := range cmds {
		if r, ok := c.(model.ReadSnapshot); ok {
			read = &r
		}
	}
	if read == nil || !reflect.DeepEqual(read.Query.Label, []string{"bug"}) {
		t.Fatalf("no read with the view's filters: %#v", cmds)
	}
	if got := opened.FilterSummary(); got != "status=open · label=bug (work bugs)" {
		t.Fatalf("filter line = %q", got)
	}
	changed := opened
	changed.Filters.Label = "other"
	if strings.Contains(changed.FilterSummary(), "work bugs") {
		t.Fatalf("line still names the view after the filters moved: %q", changed.FilterSummary())
	}
	cleared, _ := model.Update(opened, model.KeyMsg{Chord: "c", Text: "c"})
	if cleared.Filters != (model.Filters{}) || cleared.CurrentSavedView() != "" || cleared.FilterSummary() != "" {
		t.Fatalf("c: filters=%+v line=%q", cleared.Filters, cleared.FilterSummary())
	}
	if cleared.Views.List[0].Filters != savedViews()[0].Filters {
		t.Fatal("c changed the saved view")
	}
}

func TestAtMeResolvesToThePanesActorWhenTheViewOpens(t *testing.T) {
	m := viewsPane()
	e, _ := findView(m, "Mine")
	opened, _ := openRow(m, e)
	if opened.Filters.Owner != "ann" || opened.Collection.View != "today" {
		t.Fatalf("opened: %+v %+v", opened.Collection, opened.Filters)
	}
	if got := opened.FilterSummary(); got != "owner=ann (Mine)" {
		t.Fatalf("line = %q", got)
	}
	if m.Views.List[1].Filters.Owner != model.OwnerMe {
		t.Fatal("the saved view lost @me")
	}
	m.Actor = "bob"
	e, _ = findView(m, "Mine")
	if again, _ := openRow(m, e); again.Filters.Owner != "bob" {
		t.Fatalf("owner = %q, want bob", again.Filters.Owner)
	}
}

func newViewCard(t *testing.T, m model.Model) model.Model {
	t.Helper()
	m, _ = menuKeys(m, "down", "enter", "end", "enter")
	if m.TopCard() != model.CardView {
		t.Fatalf("top card %v, want the View card", m.TopCard().Name())
	}
	return m
}

func typeText(m model.Model, text string) model.Model {
	for _, r := range text {
		m, _ = model.Update(m, model.KeyMsg{Chord: string(r), Text: string(r)})
	}
	return m
}

func TestViewCardOpensPrefilledWithTheCurrentCollectionAndFilters(t *testing.T) {
	m := viewsPane()
	m.Filters = model.Filters{Status: "closed", Search: "crash"}
	m = newViewCard(t, m)
	f := m.ViewForm
	if f.Name != "" || f.Base != m.Collection || f.Values != m.Filters || f.Self != "" {
		t.Fatalf("card = %+v", f)
	}
}

func TestViewCardRefusalsNameTheFieldAndKeepTheInput(t *testing.T) {
	for _, tt := range []struct {
		name, typed, want string
		field             model.Field
	}{
		{"empty name", "", "Name", model.ViewName},
		{"a fixed Collection's title", "today", "Name", model.ViewName},
		{"a fixed Collection's key", "all-open", "Name", model.ViewName},
		{"a taken name, any case", "MINE", "Name", model.ViewName},
		{"too long", strings.Repeat("x", 65), "Name", model.ViewName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newViewCard(t, viewsPane())
			m.ViewForm.Field = model.FilterOwner
			m.ViewForm.Values.Owner = "kept"
			m.ViewForm.Field = model.ViewName
			m = typeText(m, tt.typed)
			got, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
			if len(cmds) != 0 || got.ViewForm.Err == nil || !strings.HasPrefix(got.ViewForm.Err.Error(), tt.want) {
				t.Fatalf("cmds=%v err=%v", cmds, got.ViewForm.Err)
			}
			if got.ViewForm.Name != tt.typed || got.ViewForm.Values.Owner != "kept" || !got.ViewForm.Open || got.ViewForm.Saving || got.ViewForm.Field != tt.field {
				t.Fatalf("input not kept: %+v", got.ViewForm)
			}
		})
	}
}

func TestViewCardRefusesAnUnreadFile(t *testing.T) {
	m := viewsPane()
	m, _ = model.Update(m, model.ViewsLoaded{Err: errors.New("views.toml: bad")})
	if !strings.Contains(m.Notice, "views.toml: bad") {
		t.Fatalf("the unreadable file is not reported: notice %q", m.Notice)
	}
	m = newViewCard(t, m)
	m = typeText(m, "fresh")
	got, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if len(cmds) != 0 || got.ViewForm.Err == nil || got.ViewForm.Name != "fresh" {
		t.Fatalf("cmds=%v err=%v name=%q", cmds, got.ViewForm.Err, got.ViewForm.Name)
	}
}

func TestSavingAViewSendsItWithTheReadItRestsOnAndOpensIt(t *testing.T) {
	m := viewsPane()
	m.Filters = model.Filters{Status: "open"}
	m = newViewCard(t, m)
	m = typeText(m, "  Fresh  ")
	saved, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	want := model.SaveView{View: model.SavedView{Name: "Fresh", Base: m.Collection, Filters: model.Filters{Status: "open"}}, Token: testToken{1}}
	if !reflect.DeepEqual(cmds, []model.Cmd{want}) || !saved.ViewForm.Saving {
		t.Fatalf("cmds=%#v saving=%v", cmds, saved.ViewForm.Saving)
	}
	if _, again := model.Update(saved, model.KeyMsg{Chord: "x", Text: "x"}); len(again) != 0 {
		t.Fatal("a key while saving sent a command")
	}
	loaded := model.ViewsLoaded{Views: append(savedViews(), want.View), Token: testToken{2}}
	done, _ := model.Update(saved, model.ViewSaved{Sent: want, Loaded: loaded})
	if done.ViewForm.Open || done.Picker.Open || done.CurrentSavedView() != "Fresh" || done.Views.Token != (testToken{2}) {
		t.Fatalf("after save: card=%v picker=%v view=%q token=%v", done.ViewForm.Open, done.Picker.Open, done.CurrentSavedView(), done.Views.Token)
	}
}

func TestAChangedFileRefusesAndKeepsTheInput(t *testing.T) {
	m := newViewCard(t, viewsPane())
	m = typeText(m, "Fresh")
	saved, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	sent := cmds[0].(model.SaveView)
	loaded := model.ViewsLoaded{Views: savedViews(), Token: testToken{7}}
	got, _ := model.Update(saved, model.ViewSaved{Sent: sent, Changed: true, Err: errors.New("changed"), Loaded: loaded})
	if !got.ViewForm.Open || got.ViewForm.Saving || got.ViewForm.Name != "Fresh" || got.ViewForm.Err == nil || got.Views.Token != (testToken{7}) {
		t.Fatalf("form=%+v token=%v", got.ViewForm, got.Views.Token)
	}
	again, cmds := model.Update(got, model.KeyMsg{Chord: "ctrl+o"})
	if again.ViewForm.Err != nil || len(cmds) != 1 || cmds[0].(model.SaveView).Token != (testToken{7}) {
		t.Fatalf("retry: err=%v cmds=%v", again.ViewForm.Err, cmds)
	}
}

func TestEditOpensThePrefilledCardAndARenameNamesTheOldView(t *testing.T) {
	m := viewsPane()
	m, _ = menuKeys(m, "down", "enter")
	m.Picker.Cursor = entryIdx(m, "work bugs")
	m, _ = model.Update(m, model.KeyMsg{Chord: "f2"})
	f := m.ViewForm
	if m.TopCard() != model.CardView || f.Self != "work bugs" || f.Name != "work bugs" || f.Base.ProjectUID != "alpha-uid" || f.Values.Label != "bug" {
		t.Fatalf("card = %+v", f)
	}
	m = typeText(m, "!")
	_, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	got := cmds[0].(model.SaveView)
	if got.Self != "work bugs" || got.View.Name != "work bugs!" {
		t.Fatalf("save = %+v", got)
	}
	// keeping its own name ignoring case is not a clash
	m.ViewForm.Name = "WORK BUGS"
	if _, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"}); len(cmds) != 1 {
		t.Fatalf("own name refused: %v", cmds)
	}
}

func entryIdx(m model.Model, title string) int {
	for i, e := range m.PickerEntries() {
		if e.Title == title {
			return i
		}
	}
	return -1
}

func TestDeleteAsksFirstAndOnlyYDeletes(t *testing.T) {
	open := func() model.Model {
		m, _ := menuKeys(viewsPane(), "down", "enter")
		m.Picker.Cursor = entryIdx(m, "Mine")
		m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
		return m
	}
	asked := open()
	if asked.Views.Deleting != "Mine" || asked.CurrentMode() != model.ModeDeleteView || model.DeleteViewQuestion("Mine") != "Delete view Mine? y/n" {
		t.Fatalf("asked: %+v mode %s", asked.Views, asked.CurrentMode())
	}
	if kept, cmds := model.Update(asked, model.KeyMsg{Chord: "x", Text: "x"}); len(cmds) != 0 || kept.Views.Deleting != "Mine" {
		t.Fatal("another key answered the question")
	}
	for _, chord := range []string{"n", "escape"} {
		if kept, cmds := model.Update(asked, model.KeyMsg{Chord: chord}); len(cmds) != 0 || kept.Views.Deleting != "" || !kept.Picker.Open {
			t.Fatalf("%s: cmds=%v deleting=%q picker=%v", chord, cmds, kept.Views.Deleting, kept.Picker.Open)
		}
	}
	_, cmds := model.Update(asked, model.KeyMsg{Chord: "y", Text: "y"})
	if !reflect.DeepEqual(cmds, []model.Cmd{model.DeleteView{Name: "Mine", Token: testToken{1}}}) {
		t.Fatalf("y sent %#v", cmds)
	}
	done, _ := model.Update(asked, model.ViewDeleted{Sent: cmds[0].(model.DeleteView), Loaded: model.ViewsLoaded{Views: savedViews()[:1], Token: testToken{2}}})
	if len(done.Views.List) != 1 || !strings.Contains(done.Notice, "Mine") {
		t.Fatalf("after delete: %+v notice %q", done.Views, done.Notice)
	}
}

func TestEditAndDeleteKeysOnTheSidebarAndNotOnFixedRows(t *testing.T) {
	m := viewsPane()
	if m.SidebarLevel != model.MenuViews {
		t.Fatalf("sidebar level %v", m.SidebarLevel)
	}
	m.Focus = model.ColumnCollections
	m.CollectionCursor = 1 // Inbox
	if got, _ := model.Update(m, model.KeyMsg{Chord: "ctrl+d"}); got.Views.Deleting != "" {
		t.Fatal("delete asked on a fixed Collection")
	}
	if got, _ := model.Update(m, model.KeyMsg{Chord: "f2"}); got.ViewForm.Open {
		t.Fatal("edit opened on a fixed Collection")
	}
	e, _ := findView(m, "Old")
	for i, row := range m.SidebarEntries() {
		if row.Key() == e.Key() {
			m.CollectionCursor = i
		}
	}
	if got, _ := model.Update(m, model.KeyMsg{Chord: "ctrl+d"}); got.Views.Deleting != "Old" || got.CurrentMode() != model.ModeDeleteView {
		t.Fatalf("sidebar delete: %+v", got.Views)
	}
	if got, _ := model.Update(m, model.KeyMsg{Chord: "f2"}); !got.ViewForm.Open || got.ViewForm.Self != "Old" {
		t.Fatal("sidebar edit did not open the card")
	}
}

func TestViewsAreReadWhenThePaneStartsAndEachTimeTheMenuOpens(t *testing.T) {
	_, cmds := model.Update(model.Model{}, model.Start{Workspace: "/repo"})
	if !containsCmd(cmds, model.LoadViews{}) {
		t.Fatalf("start: %#v", cmds)
	}
	_, cmds = model.Update(pickerStart(120, true), model.ActionMsg{Action: model.OpenPicker})
	if !containsCmd(cmds, model.LoadViews{}) {
		t.Fatalf("menu: %#v", cmds)
	}
	m, cmds := pickerKeys(pickerStart(160, false), "escape")
	if !containsCmd(cmds, model.LoadViews{}) || m.Focus != model.ColumnCollections {
		t.Fatalf("sidebar: %#v", cmds)
	}
}

func containsCmd(cmds []model.Cmd, want model.Cmd) bool {
	for _, c := range cmds {
		if reflect.DeepEqual(c, want) {
			return true
		}
	}
	return false
}

func TestAnUnreadableFileIsReportedAndTheFixedCollectionsStillOpen(t *testing.T) {
	m := pickerStart(120, true)
	m, _ = model.Update(m, model.ViewsLoaded{Err: errors.New("/x/views.toml: expected a table")})
	if !strings.Contains(m.Notice, "views.toml") || m.Views.Err == nil {
		t.Fatalf("notice %q err %v", m.Notice, m.Views.Err)
	}
	m, _ = menuKeys(m, "down", "enter", "down", "down", "enter")
	if m.Collection.Title != "Today" {
		t.Fatalf("collection = %+v", m.Collection)
	}
}

func TestEditAndDeleteKeysAreOnTheHelpSheetAndInBothFooters(t *testing.T) {
	var sheet []string
	for _, group := range model.HelpSheet() {
		for _, line := range group.Lines {
			if strings.Contains(line.Text, "saved view") {
				sheet = append(sheet, line.Text+": "+strings.Join(line.Keys, ","))
			}
		}
	}
	if len(sheet) != 2 || !strings.HasSuffix(sheet[0], ": f2") || !strings.HasSuffix(sheet[1], ": ctrl+d") {
		t.Fatalf("help sheet lines = %q", sheet)
	}
	for _, mode := range []model.Mode{model.ModePicker, model.ModeCollections} {
		var actions []model.Action
		for b := range model.BindingsForMode(mode, true) {
			actions = append(actions, b.Action)
		}
		if !slices.Contains(actions, model.EditViewAction) || !slices.Contains(actions, model.AskDeleteView) {
			t.Errorf("footer of %s lacks the view keys: %v", mode, actions)
		}
	}
}

func TestSwitchingCollectionAfterOpeningASavedViewStopsNamingIt(t *testing.T) {
	m := viewsPane()
	e, _ := findView(m, "Mine")
	opened, _ := openRow(m, e)
	other, _ := model.Update(opened, model.CollectionSelected{Collection: model.InboxCollection()})
	if other.Filters != opened.Filters {
		t.Fatalf("fixture: filters %+v changed with the Collection", other.Filters)
	}
	if got := other.CurrentSavedView(); got != "" {
		t.Errorf("CurrentSavedView = %q after moving to Inbox", got)
	}
	if got := other.FilterSummary(); strings.Contains(got, "Mine") {
		t.Errorf("filter line = %q still names the view", got)
	}
	back, _ := model.Update(other, model.CollectionSelected{Collection: opened.Collection})
	if got := back.CurrentSavedView(); got != "Mine" {
		t.Errorf("back on the view's base the view is %q, want Mine", got)
	}
}
