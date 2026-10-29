package model_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The bound Project sorts last in the catalog so that pinning it first is visible.
func pickerCatalog() []model.CatalogProject {
	return []model.CatalogProject{
		{Project: model.Project{UID: "alpha-uid", Name: "alpha"}},
		{Project: model.Project{UID: "smoke-uid", Name: "smoke"}},
	}
}

func pickerStart(cols int, bound bool) model.Model {
	loaded, snapshot := model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{
		Catalog: pickerCatalog(),
		Issues:  []model.Issue{{UID: "a", ShortID: "aaaa", Title: "First"}, {UID: "b", ShortID: "bbbb", Title: "Second"}},
	}}
	if bound {
		loaded.Project = model.Project{UID: "smoke-uid", Name: "smoke"}
	}
	m, _ := loadFirst(model.Model{Cols: cols}, loaded, snapshot)
	return m
}

func TestStartupColumnsFollowBinding(t *testing.T) {
	for _, tt := range []struct {
		name          string
		bound, hidden bool
		cols          int
		collection    string
		want          []model.Column
	}{
		{"bound opens the Project without the sidebar", true, false, 160, "project", []model.Column{model.ColumnList, model.ColumnDetail}},
		{"bound with detail hidden", true, true, 160, "project", []model.Column{model.ColumnList}},
		{"unbound keeps three columns on Inbox", false, false, 160, "inbox", []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}},
		{"unbound with detail hidden", false, true, 160, "inbox", []model.Column{model.ColumnCollections, model.ColumnList}},
		{"bound narrow folds to the focused column", true, false, 100, "project", []model.Column{model.ColumnList}},
		{"unbound narrow folds to the focused column", false, false, 100, "inbox", []model.Column{model.ColumnList}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := pickerStart(tt.cols, tt.bound)
			if tt.hidden {
				m, _ = model.Update(m, model.KeyMsg{Chord: "shift+l"})
			}
			if m.Collection.View != tt.collection || m.Focus != model.ColumnList {
				t.Fatalf("startup Collection %+v focus %v", m.Collection, m.Focus)
			}
			if got := m.VisibleColumns(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("visible columns = %v, want %v", got, tt.want)
			}
		})
	}
}

func pickerKeys(m model.Model, keys ...string) (model.Model, []model.Cmd) {
	var cmds []model.Cmd
	for _, key := range keys {
		msg := model.KeyMsg{Chord: key}
		if len([]rune(key)) == 1 {
			msg.Text = key
		}
		m, cmds = model.Update(m, msg)
	}
	return m, cmds
}

// menuKeys opens the Menu's top level over the columns, as a click on the top bar does, then types the keys.
func menuKeys(m model.Model, keys ...string) (model.Model, []model.Cmd) {
	m, _ = model.Update(m, model.ActionMsg{Action: model.OpenPicker})
	return pickerKeys(m, keys...)
}

func TestShiftPOpensTheProjectsLevel(t *testing.T) {
	alpha := model.Collection{View: "project", ProjectUID: "alpha-uid", Title: "alpha"}
	for _, tt := range []struct {
		name  string
		start func() model.Model
		keys  []string
		focus model.Column
		popup bool
	}{
		{"bound list", func() model.Model { return pickerStart(120, true) }, nil, model.ColumnList, true},
		{"bound detail", func() model.Model { return pickerStart(120, true) }, []string{"enter"}, model.ColumnDetail, true},
		{"unbound on a Project Collection", func() model.Model {
			m, _ := model.Update(pickerStart(160, false), model.CollectionSelected{Collection: alpha})
			return m
		}, nil, model.ColumnList, true},
		{"unbound sidebar from the list", func() model.Model { return pickerStart(160, false) }, nil, model.ColumnCollections, false},
		{"unbound sidebar from detail", func() model.Model { return pickerStart(160, false) }, []string{"enter"}, model.ColumnCollections, false},
		{"unbound sidebar from the sidebar", func() model.Model { return pickerStart(160, false) }, []string{"escape"}, model.ColumnCollections, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := pickerKeys(tt.start(), tt.keys...)
			opened, cmds := pickerKeys(m, "shift+p")
			if len(cmds) != 0 || opened.Focus != tt.focus || opened.Picker.Open != tt.popup || (opened.Picker.Level == model.MenuProjects) != tt.popup || (opened.SidebarLevel == model.MenuProjects) == tt.popup {
				t.Fatalf("after P: popup=%v level=%v sidebar level=%v focus=%v cmds=%#v", opened.Picker.Open, opened.Picker.Level, opened.SidebarLevel, opened.Focus, cmds)
			}
			if tt.popup && opened.CurrentMode() != model.ModePicker {
				t.Fatalf("mode %s, want the Menu", opened.CurrentMode())
			}
		})
	}
}

func TestPTypesIntoSearchInsteadOfSettingAParent(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, true), "/", "p")
	if m.FieldForm.Open || m.Filters.Search != "p" {
		t.Fatalf("p in search: form=%v search=%q", m.FieldForm.Open, m.Filters.Search)
	}
}

func TestEscapeUnwindsDetailListThenPickerWhenBound(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, true), "enter")
	for i, want := range []struct {
		mode   model.Mode
		focus  model.Column
		picker bool
	}{
		{model.ModeList, model.ColumnList, false},
		{model.ModePicker, model.ColumnList, true},
	} {
		var cmds []model.Cmd
		m, cmds = pickerKeys(m, "escape")
		if m.CurrentMode() != want.mode || m.Focus != want.focus || m.Picker.Open != want.picker || len(cmds) != 0 {
			t.Fatalf("escape %d: mode=%s focus=%v picker=%v cmds=%#v", i+1, m.CurrentMode(), m.Focus, m.Picker.Open, cmds)
		}
	}
}

func TestEscapeFocusesSidebarWhenUnbound(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, false), "escape")
	if m.CurrentMode() != model.ModeCollections || m.Focus != model.ColumnCollections || m.Picker.Open {
		t.Fatalf("unbound escape: mode=%s focus=%v picker=%v", m.CurrentMode(), m.Focus, m.Picker.Open)
	}
}

func pickerTitles(m model.Model) []string {
	titles := []string{}
	for _, collection := range m.PickerEntries() {
		titles = append(titles, collection.Title)
	}
	return titles
}

func TestPickerEntries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		bound  bool
		filter string
		want   []string
	}{
		{"bound Project is pinned first", true, "", []string{"smoke", "Views", "Projects", "Credentials", "Federation", "Daemons", "Restore a deleted Issue", "New Project"}},
		{"unbound lists no Project inline", false, "", []string{"Views", "Projects", "Credentials", "Federation", "Daemons", "Restore a deleted Issue", "New Project"}},
		{"filter finds a fixed Collection from the top", true, "tod", []string{"Today"}},
		{"filter finds the Views entry by title", true, "view", []string{"Views"}},
		{"filter finds the restore entry by title", true, "restore", []string{"Restore a deleted Issue"}},
		{"filter matches any case and keeps the order", true, "AL", []string{"All Open", "alpha", "Credentials"}},
		{"filter finds a Lettered Page by title", true, "cred", []string{"Credentials"}},
		{"filter can drop the pinned Project", true, "alp", []string{"alpha"}},
		{"filter finds the Projects entry and New Project by title", true, "PROJ", []string{"Projects", "New Project"}},
		{"filter without a match lists nothing", true, "zzz", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := pickerStart(120, tt.bound)
			m.Picker = model.PickerState{Open: true, Filter: tt.filter}
			if got := pickerTitles(m); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("entries = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPickerCursorMoves(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		want int
	}{
		{"starts on the first entry", nil, 0},
		{"down", []string{"down"}, 1},
		{"j", []string{"j"}, 1},
		{"up from the top stays", []string{"up"}, 0},
		{"k from the top stays", []string{"k"}, 0},
		{"down then up", []string{"down", "down", "up"}, 1},
		{"end", []string{"end"}, 7},
		{"home", []string{"end", "home"}, 0},
		{"down stops at the last entry", []string{"end", "down", "j"}, 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := menuKeys(pickerStart(120, true), tt.keys...)
			if !m.Picker.Open || m.Picker.Cursor != tt.want || len(cmds) != 0 {
				t.Fatalf("picker cursor = %d open=%v cmds=%#v, want %d", m.Picker.Cursor, m.Picker.Open, cmds, tt.want)
			}
		})
	}
}

func TestPickerEnterSwitchesLikeTheSidebar(t *testing.T) {
	inbox := model.Collection{View: "inbox", Title: "Inbox"}
	for _, tt := range []struct {
		name string
		keys []string
	}{
		{"from the list", nil},
		{"from detail", []string{"enter"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := pickerKeys(pickerStart(120, true), tt.keys...)
			want, wantCmds := model.Update(base, model.CollectionSelected{Collection: inbox})
			want.Focus = model.ColumnList
			got, gotCmds := menuKeys(base, "down", "enter", "down", "enter")
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) || len(gotCmds) != 1 {
				t.Fatalf("picker switch: collection=%+v loading=%v focus=%v picker=%+v cmds=%#v, want %+v %#v", got.Collection, got.Loading, got.Focus, got.Picker, gotCmds, want.Collection, wantCmds)
			}
		})
	}
}

func TestPickerReturnsToBoundProject(t *testing.T) {
	m, _ := menuKeys(pickerStart(120, true), "down", "enter", "down", "enter")
	if m.Collection.View != "inbox" {
		t.Fatalf("first switch: %+v", m.Collection)
	}
	m, cmds := menuKeys(m, "up", "enter")
	want := model.Collection{View: "project", ProjectUID: "smoke-uid", Title: "smoke"}
	if m.Collection != want || !readsCollection(cmds, m, "") {
		t.Fatalf("pinned entry did not return to the bound Project: %+v %#v", m.Collection, cmds)
	}
}

func TestPickerEscapeChangesNothing(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
	}{
		{"from the list", nil},
		{"from detail", []string{"enter"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := pickerKeys(pickerStart(120, true), tt.keys...)
			got, cmds := menuKeys(base, "down", "escape")
			if !reflect.DeepEqual(got, base) || len(cmds) != 0 {
				t.Fatalf("escape changed state: picker=%+v focus=%v collection=%+v cmds=%#v", got.Picker, got.Focus, got.Collection, cmds)
			}
		})
	}
}

func TestPickerTypingFiltersEntries(t *testing.T) {
	all := []string{"smoke", "Views", "Projects", "Credentials", "Federation", "Daemons", "Restore a deleted Issue", "New Project"}
	for _, tt := range []struct {
		name   string
		keys   []string
		filter string
		titles []string
		cursor int
	}{
		{"typing narrows", []string{"a", "l"}, "al", []string{"All Open", "alpha", "Credentials"}, 0},
		{"typing returns the cursor to the top", []string{"down", "down", "a"}, "a", []string{"Today", "Deadlines", "All Open", "alpha", "Credentials", "Federation", "Daemons", "Restore a deleted Issue"}, 0},
		{"backspace widens", []string{"a", "l", "backspace"}, "a", []string{"Today", "Deadlines", "All Open", "alpha", "Credentials", "Federation", "Daemons", "Restore a deleted Issue"}, 0},
		{"ctrl+u clears", []string{"a", "l", "ctrl+u"}, "", all, 0},
		{"space is typed", []string{"a", "l", "l", "space", "o"}, "all o", []string{"All Open"}, 0},
		{"j and k move while the filter is empty", []string{"j", "j", "k"}, "", all, 1},
		{"j and k are typed once filtering", []string{"a", "l", "k"}, "alk", []string{}, 0},
		{"global keys are typed", []string{"q", "?", "p"}, "q?p", []string{}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := menuKeys(pickerStart(120, true), tt.keys...)
			if m.Picker.Filter != tt.filter || m.Picker.Cursor != tt.cursor || !reflect.DeepEqual(pickerTitles(m), tt.titles) {
				t.Fatalf("filter=%q cursor=%d entries=%q, want %q %d %q", m.Picker.Filter, m.Picker.Cursor, pickerTitles(m), tt.filter, tt.cursor, tt.titles)
			}
			if !m.Picker.Open || m.Quitting || m.HelpOpen || len(cmds) != 0 {
				t.Fatalf("typing left the picker: open=%v quitting=%v help=%v cmds=%#v", m.Picker.Open, m.Quitting, m.HelpOpen, cmds)
			}
		})
	}
}

func TestPickerEnterOpensTheFilteredEntry(t *testing.T) {
	m, cmds := menuKeys(pickerStart(120, true), "t", "o", "enter")
	want := model.Collection{View: "today", Title: "Today"}
	if m.Picker.Open || m.Collection != want || m.Focus != model.ColumnList || !readsCollection(cmds, m, "") {
		t.Fatalf("filtered pick: open=%v collection=%+v cmds=%#v", m.Picker.Open, m.Collection, cmds)
	}
}

func TestPickerEnterWithoutMatchKeepsPickerOpen(t *testing.T) {
	base := pickerStart(120, true)
	m, cmds := menuKeys(base, "z", "z", "enter")
	if !m.Picker.Open || m.Collection != base.Collection || len(cmds) != 0 {
		t.Fatalf("no-match enter: open=%v collection=%+v cmds=%#v", m.Picker.Open, m.Collection, cmds)
	}
}

func TestPickerInterruptQuits(t *testing.T) {
	m, cmds := menuKeys(pickerStart(120, true), "ctrl+c")
	if !m.Quitting || len(cmds) != 0 {
		t.Fatalf("ctrl+c in picker: quitting=%v cmds=%#v", m.Quitting, cmds)
	}
}

func TestPickerPointerEqualsKeyboard(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
		keys []string
		cmds int
	}{
		{"the Views entry", "views", []string{"down", "enter"}, 0},
		{"the pinned bound Project", "project.smoke-uid", []string{"enter"}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened, _ := menuKeys(pickerStart(120, true))
			want, wantCmds := pickerKeys(opened, tt.keys...)
			got, gotCmds := model.Update(opened, model.PickerSelected{Key: tt.key})
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) || len(gotCmds) != tt.cmds {
				t.Fatalf("click differs from keyboard: %+v %#v vs %+v %#v", got.Collection, gotCmds, want.Collection, wantCmds)
			}
		})
	}
}

func TestPickerIgnoresUnknownAndStaleClicks(t *testing.T) {
	opened, _ := menuKeys(pickerStart(120, true))
	for _, tt := range []struct {
		name  string
		start model.Model
		key   string
	}{
		{"unknown entry", opened, "project.missing"},
		{"closed picker", pickerStart(120, true), "inbox"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, cmds := model.Update(tt.start, model.PickerSelected{Key: tt.key})
			if !reflect.DeepEqual(got, tt.start) || len(cmds) != 0 {
				t.Fatalf("click changed state: %+v %#v", got.Collection, cmds)
			}
		})
	}
}

func TestPickerKeysAreAdvertisedFromTheKeymap(t *testing.T) {
	advertised := func(mode model.Mode) map[string]model.Action {
		bindings := map[string]model.Action{}
		for binding := range model.BindingsForMode(mode, true) {
			if mode == model.ModePicker && binding.Mode != model.ModePicker {
				t.Fatalf("picker advertises an inactive global binding: %+v", binding)
			}
			bindings[binding.Chord] = binding.Action
		}
		return bindings
	}
	for _, binding := range model.Keymap {
		if binding.Action == model.OpenPicker {
			t.Fatalf("OpenPicker is reached by a click, not by %q in mode %q", binding.Chord, binding.Mode)
		}
	}
	picker := advertised(model.ModePicker)
	for chord, action := range map[string]model.Action{"down": model.MoveDown, "up": model.MoveUp, "enter": model.PickCollection, "escape": model.ClosePicker} {
		if picker[chord] != action {
			t.Fatalf("picker hints: %s = %q, want %q", chord, picker[chord], action)
		}
	}
}

func TestListHeadingClickOpensThePickerFromTheSidebar(t *testing.T) {
	m := pickerStart(120, false)
	m, _ = pickerKeys(m, "escape")
	if m.Focus != model.ColumnCollections {
		t.Fatalf("fixture: focus %v", m.Focus)
	}
	m, _ = model.Update(m, model.ActionMsg{Action: model.OpenPicker})
	if !m.Picker.Open || m.Picker.Level != model.MenuTop {
		t.Fatal("clicking the top bar title did not open the Menu's top level")
	}
}

func projectsLevel(t *testing.T, bound bool) model.Model {
	t.Helper()
	row := 1
	if bound {
		row = 2
	}
	keys := append(slices.Repeat([]string{"down"}, row), "enter")
	m, cmds := menuKeys(pickerStart(120, bound), keys...)
	if !m.Picker.Open || m.Picker.Level != model.MenuProjects || len(cmds) != 0 {
		t.Fatalf("Projects entry: open=%v level=%v cmds=%#v", m.Picker.Open, m.Picker.Level, cmds)
	}
	return m
}

func TestPickerProjectsEntryListsEveryProject(t *testing.T) {
	m := projectsLevel(t, true)
	if got, want := pickerTitles(m), []string{"Back", "alpha", "smoke"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Projects level = %q, want %q", got, want)
	}
}

func TestPickerPicksAProjectFromTheProjectsLevel(t *testing.T) {
	alpha := model.Collection{View: "project", ProjectUID: "alpha-uid", Title: "alpha"}
	m, cmds := pickerKeys(projectsLevel(t, true), "up", "enter")
	if m.Picker.Open || m.Collection != alpha || m.Focus != model.ColumnList || !readsCollection(cmds, m, "") {
		t.Fatalf("pick: open=%v collection=%+v cmds=%#v", m.Picker.Open, m.Collection, cmds)
	}
	if reopened, _ := menuKeys(m); reopened.Picker.Level != model.MenuTop {
		t.Fatal("the Menu reopened on the Projects level")
	}
}

func TestPickerEscapeLeavesTheProjectsLevelBeforeClosing(t *testing.T) {
	base := projectsLevel(t, true)
	m, cmds := pickerKeys(base, "escape")
	if !m.Picker.Open || m.Picker.Level != model.MenuTop || len(cmds) != 0 || pickerTitles(m)[2] != "Projects" || m.Picker.Cursor != 2 {
		t.Fatalf("esc: open=%v level=%v cursor=%d entries=%q", m.Picker.Open, m.Picker.Level, m.Picker.Cursor, pickerTitles(m))
	}
	if m, _ = pickerKeys(m, "escape"); m.Picker.Open {
		t.Fatal("second esc did not close the Menu")
	}
}

func TestPickerBackEntryAndClicksSwitchLevels(t *testing.T) {
	base := projectsLevel(t, true)
	viaEnter, _ := pickerKeys(base, "home", "enter")
	viaEsc, _ := pickerKeys(base, "escape")
	viaClick, _ := model.Update(base, model.PickerSelected{Key: "back"})
	if viaEnter.Picker.Level != model.MenuTop || !reflect.DeepEqual(viaEnter, viaEsc) || !reflect.DeepEqual(viaClick, viaEsc) {
		t.Fatalf("back: enter %+v, esc %+v, click %+v", viaEnter.Picker, viaEsc.Picker, viaClick.Picker)
	}
	top, _ := menuKeys(pickerStart(120, true))
	into, _ := model.Update(top, model.PickerSelected{Key: "projects"})
	if want := projectsLevel(t, true); !reflect.DeepEqual(into, want) {
		t.Fatalf("click on Projects: %+v, want %+v", into.Picker, want.Picker)
	}
	m, cmds := model.Update(into, model.PickerSelected{Key: "project.alpha-uid"})
	if m.Picker.Open || m.Collection.ProjectUID != "alpha-uid" || len(cmds) != 2 {
		t.Fatalf("click on a Project: open=%v collection=%+v", m.Picker.Open, m.Collection)
	}
}

func TestPickerFilterMatchesAcrossLevels(t *testing.T) {
	for _, m := range []model.Model{
		func() model.Model { m, _ := menuKeys(pickerStart(120, true)); return m }(),
		projectsLevel(t, true),
	} {
		m, _ = pickerKeys(m, "a", "l")
		if got, want := pickerTitles(m), []string{"All Open", "alpha", "Credentials"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("filter from level %v: %q, want %q", m.Picker.Level, got, want)
		}
	}
	m, _ := pickerKeys(projectsLevel(t, true), "s", "m", "o")
	if got := pickerTitles(m); !reflect.DeepEqual(got, []string{"smoke"}) {
		t.Fatalf("pinned-less filter = %q", got)
	}
	if m, _ = pickerKeys(m, "escape"); m.Picker.Open {
		t.Fatal("esc with a filter should close the Menu")
	}
}

func TestSidebarStartsOnTheViewsLevel(t *testing.T) {
	m := pickerStart(120, false)
	got := []string{}
	for _, entry := range m.SidebarEntries() {
		got = append(got, entry.Title)
	}
	if want := []string{"Back", "Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook"}; m.SidebarLevel != model.MenuViews || !reflect.DeepEqual(got, want) {
		t.Fatalf("unbound sidebar: level %v entries %q, want the Views level %q", m.SidebarLevel, got, want)
	}
}

func TestSidebarProjectsLevel(t *testing.T) {
	alpha := model.Collection{View: "project", ProjectUID: "alpha-uid", Title: "alpha"}
	titles := func(m model.Model) []string {
		out := []string{}
		for _, entry := range m.SidebarEntries() {
			out = append(out, entry.Title)
		}
		return out
	}
	top, _ := pickerKeys(pickerStart(120, false), "escape", "escape")
	if got, want := titles(top), []string{"Views", "Projects", "Credentials", "Federation", "Daemons", "Restore a deleted Issue", "New Project"}; top.SidebarLevel != model.MenuTop || !reflect.DeepEqual(got, want) {
		t.Fatalf("top level = %q, want %q", got, want)
	}
	if top.CollectionCursor != 0 || titles(top)[top.CollectionCursor] != "Views" {
		t.Fatalf("leaving the Views level put the cursor on row %d, want Views", top.CollectionCursor)
	}
	m, cmds := pickerKeys(top, "down", "enter")
	if m.SidebarLevel != model.MenuProjects || m.Focus != model.ColumnCollections || len(cmds) != 0 || !reflect.DeepEqual(titles(m), []string{"Back", "alpha", "smoke"}) {
		t.Fatalf("Projects entry: level=%v focus=%v entries=%q", m.SidebarLevel, m.Focus, titles(m))
	}
	if back, _ := pickerKeys(m, "escape"); back.SidebarLevel != model.MenuTop || back.Focus != model.ColumnCollections || !reflect.DeepEqual(titles(back), titles(top)) || titles(back)[back.CollectionCursor] != "Projects" {
		t.Fatalf("esc: level=%v focus=%v", back.SidebarLevel, back.Focus)
	}
	if back, _ := pickerKeys(m, "enter"); back.SidebarLevel != model.MenuTop {
		t.Fatal("enter on Back stayed on the Projects level")
	}
	m, cmds = pickerKeys(m, "down", "enter")
	if m.SidebarLevel != model.MenuViews || m.Collection != alpha || m.Focus != model.ColumnList || !readsCollection(cmds, m, "") {
		t.Fatalf("pick: level=%v collection=%+v cmds=%#v", m.SidebarLevel, m.Collection, cmds)
	}
	clicked, _ := model.Update(top, model.CollectionPointerSelected{Key: "projects"})
	if clicked.SidebarLevel != model.MenuProjects || clicked.Focus != model.ColumnCollections {
		t.Fatalf("click on Projects: level=%v focus=%v", clicked.SidebarLevel, clicked.Focus)
	}
	clicked, _ = model.Update(clicked, model.CollectionPointerSelected{Key: "project.alpha-uid"})
	if clicked.Collection != alpha || clicked.Focus != model.ColumnList {
		t.Fatalf("click on a Project: %+v focus=%v", clicked.Collection, clicked.Focus)
	}
}

func TestSidebarViewsLevelBacksOutToTheTop(t *testing.T) {
	base, _ := pickerKeys(pickerStart(120, false), "escape")
	viaEsc, _ := pickerKeys(base, "escape")
	viaBack, _ := pickerKeys(base, "home", "enter")
	viaClick, _ := model.Update(base, model.CollectionPointerSelected{Key: "back"})
	if viaEsc.SidebarLevel != model.MenuTop || !reflect.DeepEqual(viaBack, viaEsc) || !reflect.DeepEqual(viaClick, viaEsc) {
		t.Fatalf("leaving Views: esc %v, enter on Back %v, click on Back %v", viaEsc.SidebarLevel, viaBack.SidebarLevel, viaClick.SidebarLevel)
	}
	into, _ := pickerKeys(viaEsc, "enter")
	if into.SidebarLevel != model.MenuViews || into.Focus != model.ColumnCollections {
		t.Fatalf("enter on Views: level=%v focus=%v", into.SidebarLevel, into.Focus)
	}
}

func TestPopupPickResetsTheSidebarLevel(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, false), "escape", "escape", "down", "enter", "down", "enter")
	m, _ = menuKeys(m, "down", "enter")
	if m.SidebarLevel != model.MenuViews {
		t.Fatalf("sidebar level %v after a pick in the popup, want the Views level", m.SidebarLevel)
	}
}

// viewsLevel opens the Menu's Views level in the popup of a bound Pane.
func viewsLevel(t *testing.T) model.Model {
	t.Helper()
	m, cmds := menuKeys(pickerStart(120, true), "down", "enter")
	if !m.Picker.Open || m.Picker.Level != model.MenuViews || len(cmds) != 0 {
		t.Fatalf("Views entry: open=%v level=%v cmds=%#v", m.Picker.Open, m.Picker.Level, cmds)
	}
	return m
}

func TestPickerViewsLevelListsBackAndTheFixedCollections(t *testing.T) {
	m := viewsLevel(t)
	want := []string{"Back", "Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook"}
	if got := pickerTitles(m); !reflect.DeepEqual(got, want) || m.Picker.Cursor != 0 {
		t.Fatalf("Views level = %q cursor %d, want %q on Back", got, m.Picker.Cursor, want)
	}
	for i, title := range want[1:] {
		opened, cmds := pickerKeys(m, slices.Repeat([]string{"down"}, i+1)...)
		opened, cmds = pickerKeys(opened, "enter")
		if opened.Picker.Open || opened.Collection.Title != title || opened.Collection.IsProject() || opened.Focus != model.ColumnList || !readsCollection(cmds, opened, "") {
			t.Errorf("%s: open=%v collection=%+v focus=%v cmds=%#v", title, opened.Picker.Open, opened.Collection, opened.Focus, cmds)
		}
	}
}

func TestPickerLeavesTheViewsLevelOnViews(t *testing.T) {
	base := viewsLevel(t)
	viaEsc, _ := pickerKeys(base, "escape")
	viaBackspace, _ := pickerKeys(base, "backspace")
	viaBack, _ := pickerKeys(base, "enter")
	viaClick, _ := model.Update(base, model.PickerSelected{Key: "back"})
	if !viaEsc.Picker.Open || viaEsc.Picker.Level != model.MenuTop || pickerTitles(viaEsc)[viaEsc.Picker.Cursor] != "Views" {
		t.Fatalf("esc: open=%v level=%v entries=%q cursor=%d", viaEsc.Picker.Open, viaEsc.Picker.Level, pickerTitles(viaEsc), viaEsc.Picker.Cursor)
	}
	for name, got := range map[string]model.Model{"backspace": viaBackspace, "enter on Back": viaBack, "click on Back": viaClick} {
		if !reflect.DeepEqual(got, viaEsc) {
			t.Errorf("%s left %+v, esc left %+v", name, got.Picker, viaEsc.Picker)
		}
	}
	if closed, _ := pickerKeys(viaEsc, "escape"); closed.Picker.Open {
		t.Fatal("esc on the top level did not close the Menu")
	}
}

func TestPickerViewsClickAndPAndI(t *testing.T) {
	top, _ := menuKeys(pickerStart(120, true))
	if into, _ := model.Update(top, model.PickerSelected{Key: "views"}); !reflect.DeepEqual(into, viewsLevel(t)) {
		t.Fatalf("click on Views: %+v, want %+v", into.Picker, viewsLevel(t).Picker)
	}
	if p, _ := pickerKeys(viewsLevel(t), "shift+p"); p.Picker.Level != model.MenuViews {
		t.Fatalf("shift+p typed into the Views level's filter must not move the level, got %v", p.Picker.Level)
	}
	p, _ := pickerKeys(pickerStart(120, true), "shift+p")
	if !p.Picker.Open || p.Picker.Level != model.MenuProjects {
		t.Fatalf("P: open=%v level=%v, want the Projects level", p.Picker.Open, p.Picker.Level)
	}
	i, cmds := pickerKeys(pickerStart(120, true), "shift+i")
	if i.Collection.View != "inbox" || !readsCollection(cmds, i, "") {
		t.Fatalf("I: collection %+v cmds %#v", i.Collection, cmds)
	}
}

func TestPickerOpensOnViewsWhileAFixedCollectionIsCurrent(t *testing.T) {
	bound, _ := menuKeys(pickerStart(120, true))
	if bound.Picker.Cursor != 0 {
		t.Fatalf("a Project is current: cursor %d, want the pinned Project", bound.Picker.Cursor)
	}
	inbox, _ := menuKeys(pickerStart(120, true), "down", "enter", "down", "enter")
	reopened, _ := menuKeys(inbox)
	if got := pickerTitles(reopened)[reopened.Picker.Cursor]; got != "Views" || reopened.Picker.Level != model.MenuTop {
		t.Fatalf("with Inbox current the Menu opened on %q at level %v, want Views at the top", got, reopened.Picker.Level)
	}
	unbound, _ := model.Update(pickerStart(120, false), model.CollectionSelected{Collection: model.Collection{View: "project", ProjectUID: "alpha-uid", Title: "alpha"}})
	if opened, _ := menuKeys(unbound); pickerTitles(opened)[opened.Picker.Cursor] != "Views" {
		t.Fatalf("unbound top level cursor on %q", pickerTitles(opened)[opened.Picker.Cursor])
	}
}

func TestPickerViewsLevelCursorStartsOnTheCurrentCollection(t *testing.T) {
	m, _ := menuKeys(pickerStart(120, true), "down", "enter", "down", "down", "enter")
	if m.Collection.View != "today" {
		t.Fatalf("fixture: collection %+v", m.Collection)
	}
	reopened, _ := menuKeys(m, "enter")
	if got := pickerTitles(reopened)[reopened.Picker.Cursor]; reopened.Picker.Level != model.MenuViews || got != "Today" {
		t.Fatalf("Views level opened with the cursor on %q (level %v), want Today", got, reopened.Picker.Level)
	}
}

func TestPickerFilterFindsEveryCollectionFromEachLevel(t *testing.T) {
	for name, m := range map[string]model.Model{"top": func() model.Model { m, _ := menuKeys(pickerStart(120, true)); return m }(), "views": viewsLevel(t), "projects": projectsLevel(t, true)} {
		for filter, want := range map[string]string{"tod": "Today", "inb": "Inbox", "upc": "Upcoming", "dead": "Deadlines", "all o": "All Open", "logb": "Logbook", "alp": "alpha"} {
			typed := m
			for _, r := range filter {
				typed, _ = pickerKeys(typed, string(r))
			}
			if got := pickerTitles(typed); !slices.Equal(got, []string{want}) {
				t.Errorf("from %s, %q found %q, want %q", name, filter, got, want)
			}
		}
	}
}

func TestViewsRowNamesTheCurrentFixedCollection(t *testing.T) {
	if got := pickerStart(120, true).CurrentView(); got != "" {
		t.Fatalf("with a Project current the Views row names %q", got)
	}
	m, _ := menuKeys(pickerStart(120, true), "down", "enter", "down", "down", "enter")
	if got := m.CurrentView(); got != "Today" {
		t.Fatalf("CurrentView = %q, want Today", got)
	}
}
