package driver_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/driver"
	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
	g "go.kenn.io/kata/pkg/client/generated"
)

// chord is the key as Tern reports it for a bare chord, without text.
func chord(c string) tern.Key {
	var k tern.Key
	for {
		switch {
		case strings.HasPrefix(c, "ctrl+"):
			k.Ctrl, c = true, c[len("ctrl+"):]
		case strings.HasPrefix(c, "shift+"):
			k.Shift, c = true, c[len("shift+"):]
		default:
			k.Name = c
			return k
		}
	}
}

// path is how a keyboard user reaches the state a click reaches: the state and the daemon commands.
type path func(driver.State) (driver.State, []model.Cmd)

func inputs(ins ...any) path {
	return func(s driver.State) (driver.State, []model.Cmd) {
		var cmds []model.Cmd
		for _, in := range ins {
			out := driver.Step(s, in, now)
			s, cmds = out.State, append(cmds, out.Cmds...)
		}
		return s, cmds
	}
}

func keys(chords ...string) path {
	ins := make([]any, len(chords))
	for i, c := range chords {
		ins[i] = chord(c)
	}
	return inputs(ins...)
}

func pressed(s driver.State, chords ...string) driver.State {
	s, _ = keys(chords...)(s)
	return s
}

// differ names the Model fields two states disagree on.
func differ(a, b model.Model) string {
	var out string
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := range av.NumField() {
		if !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			out += fmt.Sprintf("\n  %s: %+v != %+v", av.Type().Field(i).Name, av.Field(i).Interface(), bv.Field(i).Interface())
		}
	}
	return out
}

// sameAs fails when the click and the keys end in different states or run different commands.
func sameAs(t *testing.T, what string, clickState driver.State, clickCmds []model.Cmd, keyState driver.State, keyCmds []model.Cmd) {
	t.Helper()
	if !reflect.DeepEqual(clickState.Model, keyState.Model) || !reflect.DeepEqual(clickCmds, keyCmds) {
		t.Fatalf("%s (click != keys):%s\ncommands: %#v != %#v", what, differ(clickState.Model, keyState.Model), clickCmds, keyCmds)
	}
}

// clicked is the state and commands after one input.
func clicked(s driver.State, in any) (driver.State, []model.Cmd) { return inputs(in)(s) }

// clickBase is an unbound workspace in the Issue list. Rows sort by update time, newest first:
// a parent (collapsed, with one child), two leaves, and a detail whose Links name "other".
func clickBase() driver.State {
	hours := func(n int) time.Time {
		return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC).Add(-time.Duration(n) * time.Hour)
	}
	m := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{
		Collection: []model.Issue{
			{UID: "parent", ShortID: "par1", Title: "A parent", Status: "open", UpdatedAt: hours(1)},
			{UID: "child", ShortID: "chi1", Title: "B child", Status: "open", UpdatedAt: hours(2)},
			{UID: "leaf", ShortID: "lea1", Title: "C leaf", Status: "open", UpdatedAt: hours(3)},
			{UID: "other", ShortID: "oth1", Title: "D other", Status: "open", UpdatedAt: hours(4)},
		},
		CollectionLinks: []g.UILink{{Type: "parent", FromIssueUID: "child", ToIssueUID: "parent"}},
	}})
	m, _ = model.Update(m, model.DetailLoaded{UID: "parent", Selected: &g.UISelectedAuthority{
		Issue: &model.Issue{UID: "parent"},
		Links: []g.UILink{{ID: 1, Type: "related", FromIssueUID: "parent", ToIssueUID: "other", ToQualifiedID: "work#oth1"}},
	}})
	return driver.State{Model: m}
}

// clickAt applies listKeys in the Issue list, then moves the keyboard to focus.
func clickAt(focus model.Column, listKeys ...string) driver.State {
	s := pressed(clickBase(), listKeys...)
	switch focus {
	case model.ColumnCollections:
		s = pressed(s, "escape")
	case model.ColumnDetail:
		s = pressed(s, "enter")
	}
	return s
}

// tabTo cycles Activity tabs with tab, so tests follow the on-screen order.
func tabTo(tab model.ActivityTab) path {
	return func(s driver.State) (driver.State, []model.Cmd) {
		var cmds []model.Cmd
		for range model.TabCount {
			if s.Model.ActiveTab() == tab {
				break
			}
			s, cmds = keys("tab")(s)
		}
		return s, cmds
	}
}

// detailPath is the keyboard route to an Activity tab: enter the detail when the keys are elsewhere,
// tab to the tab, then press after.
func detailPath(tab model.ActivityTab, after ...string) path {
	return func(s driver.State) (driver.State, []model.Cmd) {
		var cmds []model.Cmd
		if s.Model.Focus != model.ColumnDetail {
			s, cmds = keys("enter")(s)
		}
		s, switched := tabTo(tab)(s)
		if len(after) == 0 {
			cmds = append(cmds, switched...)
		}
		s, more := keys(after...)(s)
		return s, append(cmds, more...)
	}
}

// jumpPath is the keys to a Link row's reference. The click needs the Links tab on screen first, so the
// pointer side starts there too and the tab is put back for the comparison.
func jumpPath(s driver.State) (driver.State, []model.Cmd) {
	s, cmds := detailPath(model.TabLinks, "enter")(s)
	s, _ = clicked(s, click("", view.ActivityTabClick(model.TabComments)))
	return s, cmds
}

func TestEveryClickTargetEndsWhereItsKeysEnd(t *testing.T) {
	today := "today"
	toggle := click(view.IssueID("parent"), view.ToggleClick("parent"))
	for _, tt := range []struct {
		name     string
		from     model.Column
		listKeys []string
		click    any
		// The keyboard user starts here, and then presses keyboard.
		keyFrom  model.Column
		keyboard path
	}{
		{"leaf row from list", model.ColumnList, nil, &tern.Select{ID: view.IssueListID, Item: view.IssueID("leaf")}, model.ColumnList, keys("j")},
		{"leaf row from detail", model.ColumnDetail, nil, &tern.Select{ID: view.IssueListID, Item: view.IssueID("leaf")}, model.ColumnDetail, keys("escape", "j")},
		{"leaf row from Collections", model.ColumnCollections, nil, &tern.Select{ID: view.IssueListID, Item: view.IssueID("leaf")}, model.ColumnList, keys("j")},
		{"nested child row", model.ColumnList, []string{"space"}, &tern.Select{ID: view.IssueListID, Item: view.IssueID("child")}, model.ColumnList, keys("j")},
		{"parent row from list", model.ColumnList, []string{"shift+g"}, toggle, model.ColumnList, keys("g", "space")},
		{"parent row from detail", model.ColumnDetail, nil, toggle, model.ColumnDetail, keys("escape", "space")},
		{"parent row from Collections", model.ColumnCollections, nil, toggle, model.ColumnList, keys("space")},
		{"parent row collapses an expanded parent", model.ColumnCollections, []string{"space"}, toggle, model.ColumnList, keys("space")},
		{"top bar title from list", model.ColumnList, nil, click("", view.KeyClick("p")), model.ColumnList, keys("p")},
		{"top bar title from detail", model.ColumnDetail, nil, click("", view.KeyClick("p")), model.ColumnDetail, keys("p")},
		{"detail title from list", model.ColumnList, nil, click("", view.FocusDetailClick), model.ColumnList, keys("enter")},
		{"Activity tab", model.ColumnList, nil, click("", view.ActivityTabClick(model.TabLinks)), model.ColumnList, detailPath(model.TabLinks)},
		{"detail row", model.ColumnDetail, nil, click("", view.DetailItemClick(model.TabLinks, "1")), model.ColumnDetail, detailPath(model.TabLinks)},
		{"referenced Issue from detail", model.ColumnDetail, nil, click("", view.JumpClick("other")), model.ColumnDetail, jumpPath},
		{"referenced Issue from list", model.ColumnList, nil, click("", view.JumpClick("other")), model.ColumnList, jumpPath},
		{"Collection from Collections", model.ColumnCollections, nil, &tern.Select{ID: view.CollectionsID, Item: view.CollectionItemID(today)}, model.ColumnCollections, keys("j", "enter")},
		{"Collection from list", model.ColumnList, nil, &tern.Select{ID: view.CollectionsID, Item: view.CollectionItemID(today)}, model.ColumnList, keys("escape", "j", "enter")},
		{"Collection from detail", model.ColumnDetail, nil, &tern.Select{ID: view.CollectionsID, Item: view.CollectionItemID(today)}, model.ColumnDetail, keys("escape", "escape", "j", "enter")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pointer, pointerCmds := clicked(clickAt(tt.from, tt.listKeys...), tt.click)
			keyboard, keyboardCmds := tt.keyboard(clickAt(tt.keyFrom, tt.listKeys...))
			sameAs(t, "click and keys diverge", pointer, pointerCmds, keyboard, keyboardCmds)
			afterClick, clickCmds := keys("escape")(pointer)
			afterKeys, keyCmds := keys("escape")(keyboard)
			sameAs(t, "esc differs after click", afterClick, clickCmds, afterKeys, keyCmds)
		})
	}
}

func TestEveryFooterHintClicksLikeItsKey(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnCollections, model.ColumnDetail} {
		s := clickAt(focus)
		for binding := range model.BindingsForMode(s.Model.CurrentMode(), true) {
			t.Run(string(s.Model.CurrentMode())+"/"+binding.Chord, func(t *testing.T) {
				keyboard, keyCmds := keys(binding.Chord)(s)
				pointer, clickCmds := clicked(s, click("", view.KeyClick(binding.Chord)))
				sameAs(t, "hint differs from key", pointer, clickCmds, keyboard, keyCmds)
			})
		}
	}
}

func TestPointerHintsUseKeyboardDispatch(t *testing.T) {
	for _, c := range []string{"n", "x", "/", "?", "page_down", "p"} {
		t.Run(c, func(t *testing.T) {
			s := driver.State{Model: withDetail(model.Model{Project: model.Project{ID: 7}, SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", Status: "open", ProjectID: 7}}}, nil)}
			keyboard, keyCmds := keys(c)(s)
			pointer, clickCmds := clicked(s, click("", view.KeyClick(c)))
			sameAs(t, "pointer differs from keyboard", pointer, clickCmds, keyboard, keyCmds)
		})
	}
}

func TestLayerClicksEndWhereTheirKeysEnd(t *testing.T) {
	for _, tt := range []struct {
		name  string
		open  string
		click *tern.Focus
		keys  []string
	}{
		{"filters field", "f", &tern.Focus{ID: view.FilterFieldID(2)}, []string{"tab", "tab"}},
		{"create field", "n", &tern.Focus{ID: view.CreateFieldID(3)}, []string{"tab", "tab", "tab"}},
		{"close field", "x", &tern.Focus{ID: view.CloseFieldID(3)}, []string{"tab", "tab", "tab"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			open := pressed(clickAt(model.ColumnList), tt.open)
			pointer, clickCmds := clicked(open, tt.click)
			keyboard, keyCmds := keys(tt.keys...)(open)
			sameAs(t, "click and keys diverge", pointer, clickCmds, keyboard, keyCmds)
		})
	}
}

func TestRetryButtonRetriesLikeItsKey(t *testing.T) {
	s := driver.State{Model: model.Model{Workspace: "/repo", Err: errors.New("kata server not responding")}}
	pointer, clickCmds := clicked(s, click(view.RetryID, view.RetryClick))
	keyboard, keyCmds := keys("ctrl+r")(s)
	if len(keyCmds) != 1 {
		t.Fatalf("fixture: ctrl+r commands = %#v", keyCmds)
	}
	sameAs(t, "retry click and key diverge", pointer, clickCmds, keyboard, keyCmds)
}

func pickerCatalog() []g.UIProject {
	return []g.UIProject{{Project: model.Project{UID: "alpha-uid", Name: "alpha"}}, {Project: model.Project{UID: "smoke-uid", Name: "smoke"}}}
}

// newProject is an unbound directory with the New Project card open from the sidebar.
func newProject() driver.State {
	m := loadFirst(model.Model{Cols: 160, Workspace: "/home/dev/my-repo", Actor: "alice"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{
		Catalog:    pickerCatalog(),
		Collection: []model.Issue{{UID: "a", ShortID: "aaaa", Title: "First"}},
	}})
	return pressed(driver.State{Model: m}, "escape", "end", "enter")
}

// A card's button clicks its key, so the click ends where the key ends.
func TestCardButtonsClickLikeTheirKeys(t *testing.T) {
	issue := driver.State{Model: model.Model{Actor: "reviewer", SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Labels: []string{"bug"}}}}}
	creating := driver.State{Model: model.Model{Project: model.Project{ID: 7}}}
	titled, _ := inputs(chord("n"), tern.Key{Text: "title"})(creating)
	asking := pressed(titled, "escape")
	draftModel := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a"}, &g.UISelectedAuthority{Issue: &model.Issue{UID: "a", ProjectID: 2, Body: "a😀b", Revision: 7}})
	draft := pressed(driver.State{Model: draftModel}, "enter", "e")
	typed, _ := inputs(tern.Key{Name: "paste", Text: "more"})(draft)
	asked := pressed(typed, "escape")
	for _, tt := range []struct {
		name   string
		start  driver.State
		chords []string
		check  func(model.Model) bool
	}{
		{"Create Issue with no title", pressed(creating, "n"), []string{"ctrl+o"}, func(m model.Model) bool { return m.Create.Err != nil && m.Create.Open }},
		{"Create Issue", titled, []string{"ctrl+o"}, func(m model.Model) bool { return m.Create.Saving }},
		{"Create Cancel", titled, []string{"escape"}, func(m model.Model) bool { return m.Create.DiscardConfirm }},
		{"Create discard prompt", asking, []string{"y", "n"}, func(m model.Model) bool { return !m.Create.Open || !m.Create.DiscardConfirm }},
		{"field form Save without a value", pressed(issue, "+"), []string{"enter"}, func(m model.Model) bool { return m.FieldForm.Err != nil }},
		{"field form Cancel", pressed(issue, "+"), []string{"escape"}, func(m model.Model) bool { return !m.FieldForm.Open }},
		{"New Project with no name", pressed(newProject(), "ctrl+u"), []string{"enter"}, func(m model.Model) bool { return m.ProjectForm.Err != nil }},
		{"New Project Cancel", newProject(), []string{"escape"}, func(m model.Model) bool { return !m.ProjectForm.Open }},
		{"Draft Save", draft, []string{"ctrl+o"}, func(m model.Model) bool { return m.Drafts["a"].Saving }},
		{"Draft $EDITOR", draft, []string{"ctrl+e"}, func(model.Model) bool { return true }},
		{"Draft Discard", typed, []string{"escape"}, func(m model.Model) bool { return m.Drafts["a"].DiscardConfirm }},
		{"Draft discard prompt", asked, []string{"y", "n"}, func(model.Model) bool { return true }},
		{"Close Issue with no message", pressed(issue, "x"), []string{"ctrl+o"}, func(m model.Model) bool { return m.CloseForm.Err != nil }},
		{"Close Quick done", pressed(issue, "x"), []string{"ctrl+d"}, func(m model.Model) bool { return m.CloseForm.Saving }},
		{"Close Cancel", pressed(issue, "x"), []string{"escape"}, func(m model.Model) bool { return !m.CloseForm.Open }},
		{"Filters Apply", pressed(issue, "f"), []string{"ctrl+o"}, func(m model.Model) bool { return !m.FilterForm.Open }},
		{"Filters Cancel", pressed(issue, "f"), []string{"escape"}, func(m model.Model) bool { return !m.FilterForm.Open }},
	} {
		for _, c := range tt.chords {
			t.Run(tt.name+"/"+c, func(t *testing.T) {
				keyboard, keyCmds := keys(c)(tt.start)
				pointer, clickCmds := clicked(tt.start, click("", view.KeyClick(c)))
				sameAs(t, "click and key diverge", pointer, clickCmds, keyboard, keyCmds)
				if len(tt.chords) == 1 && !tt.check(pointer.Model) {
					t.Fatalf("the click did not do what the button says: %+v", pointer.Model)
				}
			})
		}
	}
}

// withDetail is m with the detail of its selected Issue loaded.
func withDetail(m model.Model, authority *g.UISelectedAuthority) model.Model {
	m, _ = model.Update(m, model.ResizeMsg{Cols: m.Cols})
	m, _ = model.Update(m, model.DetailLoaded{UID: m.SelectedUID, Selected: authority})
	return m
}
