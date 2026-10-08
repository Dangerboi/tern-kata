package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// The bound Project sorts last in the catalog so that pinning it first is visible.
func pickerCatalog() []g.UIProject {
	return []g.UIProject{
		{Project: model.Project{UID: "alpha-uid", Name: "alpha"}},
		{Project: model.Project{UID: "smoke-uid", Name: "smoke"}},
	}
}

func pickerStart(cols int, bound bool) model.Model {
	loaded := model.ProjectLoaded{Snapshot: g.UISnapshotResponseBody{
		Catalog:    pickerCatalog(),
		Collection: []model.Issue{{UID: "a", ShortID: "aaaa", Title: "First"}, {UID: "b", ShortID: "bbbb", Title: "Second"}},
	}}
	if bound {
		loaded.Project = model.Project{UID: "smoke-uid", Name: "smoke"}
	}
	m, _ := model.Update(model.Model{Cols: cols}, loaded)
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
		{"bound opens the Project without the sidebar", true, false, 120, "project", []model.Column{model.ColumnList, model.ColumnDetail}},
		{"bound with detail hidden", true, true, 120, "project", []model.Column{model.ColumnList}},
		{"unbound keeps three columns on Inbox", false, false, 120, "inbox", []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}},
		{"unbound with detail hidden", false, true, 120, "inbox", []model.Column{model.ColumnCollections, model.ColumnList}},
		{"bound narrow folds to the focused column", true, false, 80, "project", []model.Column{model.ColumnList}},
		{"unbound narrow folds to the focused column", false, false, 80, "inbox", []model.Column{model.ColumnList}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := pickerStart(tt.cols, tt.bound)
			m.DetailHidden = tt.hidden
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

func TestPickerOpensWithPFromListAndDetail(t *testing.T) {
	for _, tt := range []struct {
		name  string
		keys  []string
		focus model.Column
	}{
		{"list", nil, model.ColumnList},
		{"detail", []string{"enter"}, model.ColumnDetail},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := pickerKeys(pickerStart(120, true), tt.keys...)
			opened, cmds := pickerKeys(m, "p")
			if !opened.Picker.Open || opened.CurrentMode() != model.ModePicker || opened.Focus != tt.focus || len(cmds) != 0 {
				t.Fatalf("after p: picker open=%v mode=%s focus=%v cmds=%#v", opened.Picker.Open, opened.CurrentMode(), opened.Focus, cmds)
			}
		})
	}
}

func TestPTypesIntoSearchInsteadOfOpeningPicker(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, true), "/", "p")
	if m.Picker.Open || m.Filters.Search != "p" {
		t.Fatalf("p in search: picker=%v search=%q", m.Picker.Open, m.Filters.Search)
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
		{"bound Project is pinned first", true, "", []string{"smoke", "Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook", "alpha"}},
		{"unbound keeps the sidebar order", false, "", []string{"Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook", "alpha", "smoke"}},
		{"filter matches any case and keeps the order", true, "AL", []string{"All Open", "alpha"}},
		{"filter can drop the pinned Project", true, "alp", []string{"alpha"}},
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
			m, cmds := pickerKeys(pickerStart(120, true), append([]string{"p"}, tt.keys...)...)
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
			got, gotCmds := pickerKeys(base, "p", "down", "enter")
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) || len(gotCmds) != 1 {
				t.Fatalf("picker switch: collection=%+v loading=%v focus=%v picker=%+v cmds=%#v, want %+v %#v", got.Collection, got.Loading, got.Focus, got.Picker, gotCmds, want.Collection, wantCmds)
			}
		})
	}
}

func TestPickerReturnsToBoundProject(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, true), "p", "down", "enter")
	if m.Collection.View != "inbox" {
		t.Fatalf("first switch: %+v", m.Collection)
	}
	m, cmds := pickerKeys(m, "p", "enter")
	want := model.Collection{View: "project", ProjectUID: "smoke-uid", Title: "smoke"}
	if m.Collection != want || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadCollection{Collection: want}}) {
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
			got, cmds := pickerKeys(base, "p", "down", "escape")
			if !reflect.DeepEqual(got, base) || len(cmds) != 0 {
				t.Fatalf("escape changed state: picker=%+v focus=%v collection=%+v cmds=%#v", got.Picker, got.Focus, got.Collection, cmds)
			}
		})
	}
}

func TestPickerTypingFiltersEntries(t *testing.T) {
	all := []string{"smoke", "Inbox", "Today", "Upcoming", "Deadlines", "All Open", "Logbook", "alpha"}
	for _, tt := range []struct {
		name   string
		keys   []string
		filter string
		titles []string
		cursor int
	}{
		{"typing narrows", []string{"a", "l"}, "al", []string{"All Open", "alpha"}, 0},
		{"typing returns the cursor to the top", []string{"down", "down", "a"}, "a", []string{"Today", "Deadlines", "All Open", "alpha"}, 0},
		{"backspace widens", []string{"a", "l", "backspace"}, "a", []string{"Today", "Deadlines", "All Open", "alpha"}, 0},
		{"ctrl+u clears", []string{"a", "l", "ctrl+u"}, "", all, 0},
		{"space is typed", []string{"a", "l", "l", "space", "o"}, "all o", []string{"All Open"}, 0},
		{"j and k move while the filter is empty", []string{"j", "j", "k"}, "", all, 1},
		{"j and k are typed once filtering", []string{"a", "l", "k"}, "alk", []string{}, 0},
		{"global keys are typed", []string{"q", "?", "p"}, "q?p", []string{}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := pickerKeys(pickerStart(120, true), append([]string{"p"}, tt.keys...)...)
			if m.Picker.Filter != tt.filter || m.Picker.Cursor != tt.cursor || !reflect.DeepEqual(pickerTitles(m), tt.titles) {
				t.Fatalf("filter=%q cursor=%d entries=%q, want %q %d %q", m.Picker.Filter, m.Picker.Cursor, pickerTitles(m), tt.filter, tt.cursor, tt.titles)
			}
			if !m.Picker.Open || m.Quitting || m.HelpOpen || len(cmds) != 0 {
				t.Fatalf("typing left the picker: open=%v quitting=%v help=%v cmds=%#v", m.Picker.Open, m.Quitting, m.HelpOpen, cmds)
			}
		})
	}
}

func TestPickerTypedTextKeepsCaseAndPaste(t *testing.T) {
	m, _ := pickerKeys(pickerStart(120, true), "p")
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+a", Text: "A"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "ll\nopen"})
	if m.Picker.Filter != "All open" || !reflect.DeepEqual(pickerTitles(m), []string{"All Open"}) {
		t.Fatalf("filter = %q entries=%q", m.Picker.Filter, pickerTitles(m))
	}
}

func TestPickerEnterOpensTheFilteredEntry(t *testing.T) {
	m, cmds := pickerKeys(pickerStart(120, true), "p", "t", "o", "enter")
	want := model.Collection{View: "today", Title: "Today"}
	if m.Picker.Open || m.Collection != want || m.Focus != model.ColumnList || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadCollection{Collection: want}}) {
		t.Fatalf("filtered pick: open=%v collection=%+v cmds=%#v", m.Picker.Open, m.Collection, cmds)
	}
}

func TestPickerEnterWithoutMatchKeepsPickerOpen(t *testing.T) {
	base := pickerStart(120, true)
	m, cmds := pickerKeys(base, "p", "z", "z", "enter")
	if !m.Picker.Open || m.Collection != base.Collection || len(cmds) != 0 {
		t.Fatalf("no-match enter: open=%v collection=%+v cmds=%#v", m.Picker.Open, m.Collection, cmds)
	}
}

func TestPickerInterruptQuits(t *testing.T) {
	m, cmds := pickerKeys(pickerStart(120, true), "p", "ctrl+c")
	if !m.Quitting || !reflect.DeepEqual(cmds, []model.Cmd{model.Quit{}}) {
		t.Fatalf("ctrl+c in picker: quitting=%v cmds=%#v", m.Quitting, cmds)
	}
}

func TestPickerPointerEqualsKeyboard(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
		keys []string
	}{
		{"Inbox", "inbox", []string{"down", "enter"}},
		{"another Project", "project.alpha-uid", []string{"end", "enter"}},
		{"the pinned bound Project", "project.smoke-uid", []string{"enter"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened, _ := pickerKeys(pickerStart(120, true), "p")
			want, wantCmds := pickerKeys(opened, tt.keys...)
			got, gotCmds := model.Update(opened, model.PickerSelected{Key: tt.key})
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) || len(gotCmds) != 1 {
				t.Fatalf("click differs from keyboard: %+v %#v vs %+v %#v", got.Collection, gotCmds, want.Collection, wantCmds)
			}
		})
	}
}

func TestPickerIgnoresUnknownAndStaleClicks(t *testing.T) {
	opened, _ := pickerKeys(pickerStart(120, true), "p")
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

func TestPickerHeadingClickIsThePKey(t *testing.T) {
	base := pickerStart(120, true)
	want, wantCmds := model.Update(base, model.KeyMsg{Chord: "p", Text: "p"})
	got, gotCmds := model.Update(base, model.ActionMsg{Action: "key", Value: "p"})
	if !got.Picker.Open || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCmds, wantCmds) {
		t.Fatalf("heading click: open=%v differs from p", got.Picker.Open)
	}
}
