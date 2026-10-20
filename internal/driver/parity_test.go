package driver_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
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
		if !av.Type().Field(i).IsExported() {
			continue
		}
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
	m := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{
		Issues: []model.Issue{
			{UID: "parent", ShortID: "par1", Title: "A parent", Status: "open", UpdatedAt: hours(1)},
			{UID: "child", ShortID: "chi1", Title: "B child", Status: "open", UpdatedAt: hours(2)},
			{UID: "leaf", ShortID: "lea1", Title: "C leaf", Status: "open", UpdatedAt: hours(3)},
			{UID: "other", ShortID: "oth1", Title: "D other", Status: "open", UpdatedAt: hours(4)},
		},
		Links: []model.Link{{Type: "parent", FromIssueUID: "child", ToIssueUID: "parent"}},
	}})
	m, _ = model.Update(m, model.DetailLoaded{UID: "parent", Selected: &model.IssueDetailData{
		Issue: &model.Issue{UID: "parent"},
		Links: []model.Link{{ID: 1, Type: "related", FromIssueUID: "parent", ToIssueUID: "other", ToQualifiedID: "work#oth1"}},
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
// pointer side clicks that tab first too (linksTab); both record the same jump history entry.
var jumpPath = detailPath(model.TabLinks, "enter")

var linksTab = click("", view.ActivityTabClick(model.TabLinks))

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
		// before is clicked ahead of click on the pointer side.
		before any
	}{
		{"leaf row from list", model.ColumnList, nil, &tern.Select{ID: view.IssueListID, Item: view.IssueID("leaf")}, model.ColumnList, keys("j"), nil},
		{"leaf row from detail", model.ColumnDetail, nil, &tern.Select{ID: view.IssueListID, Item: view.IssueID("leaf")}, model.ColumnDetail, keys("escape", "j"), nil},
		{"leaf row from Collections", model.ColumnCollections, nil, &tern.Select{ID: view.IssueListID, Item: view.IssueID("leaf")}, model.ColumnList, keys("j"), nil},
		{"nested child row", model.ColumnList, []string{"space"}, &tern.Select{ID: view.IssueListID, Item: view.IssueID("child")}, model.ColumnList, keys("j"), nil},
		{"parent row from list", model.ColumnList, []string{"shift+g"}, toggle, model.ColumnList, keys("g", "space"), nil},
		{"parent row from detail", model.ColumnDetail, nil, toggle, model.ColumnDetail, keys("escape", "space"), nil},
		{"parent row from Collections", model.ColumnCollections, nil, toggle, model.ColumnList, keys("space"), nil},
		{"parent row collapses an expanded parent", model.ColumnCollections, []string{"space"}, toggle, model.ColumnList, keys("space"), nil},
		{"top bar title from list", model.ColumnList, nil, click("", view.MenuClick), model.ColumnList, inputs(openMenu), nil},
		{"top bar title from detail", model.ColumnDetail, nil, click("", view.MenuClick), model.ColumnDetail, inputs(openMenu), nil},
		{"detail title from list", model.ColumnList, nil, click("", view.FocusDetailClick), model.ColumnList, keys("enter"), nil},
		{"Activity tab", model.ColumnList, nil, click("", view.ActivityTabClick(model.TabLinks)), model.ColumnList, detailPath(model.TabLinks), nil},
		{"detail row", model.ColumnDetail, nil, click("", view.DetailItemClick(model.TabLinks, "1")), model.ColumnDetail, detailPath(model.TabLinks), nil},
		{"referenced Issue from detail", model.ColumnDetail, nil, click("", view.JumpClick("other")), model.ColumnDetail, jumpPath, linksTab},
		{"referenced Issue from list", model.ColumnList, nil, click("", view.JumpClick("other")), model.ColumnList, jumpPath, linksTab},
		{"Collection from Collections", model.ColumnCollections, nil, &tern.Select{ID: view.CollectionsID, Item: view.CollectionItemID(today)}, model.ColumnCollections, keys("j", "enter"), nil},
		{"Collection from list", model.ColumnList, nil, &tern.Select{ID: view.CollectionsID, Item: view.CollectionItemID(today)}, model.ColumnList, keys("escape", "j", "enter"), nil},
		{"Collection from detail", model.ColumnDetail, nil, &tern.Select{ID: view.CollectionsID, Item: view.CollectionItemID(today)}, model.ColumnDetail, keys("escape", "escape", "j", "enter"), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pointer := clickAt(tt.from, tt.listKeys...)
			if tt.before != nil {
				pointer, _ = clicked(pointer, tt.before)
			}
			pointer, pointerCmds := clicked(pointer, tt.click)
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
	for _, c := range []string{"n", "x", "/", "?", "page_down"} {
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
		{"filters field", "f", &tern.Focus{ID: view.FilterFieldID(model.FilterOwner)}, []string{"tab", "tab"}},
		{"create field", "n", &tern.Focus{ID: view.CreateFieldID(model.CreateOwner)}, []string{"tab", "tab", "tab"}},
		{"close field", "x", &tern.Focus{ID: view.CloseFieldID(model.ClosePR)}, []string{"tab", "tab", "tab"}},
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

func pickerCatalog() []model.CatalogProject {
	return []model.CatalogProject{{Project: model.Project{UID: "alpha-uid", Name: "alpha"}}, {Project: model.Project{UID: "smoke-uid", Name: "smoke"}}}
}

// newProject is an unbound directory with the New Project card open from the sidebar.
func newProject() driver.State {
	m := loadFirst(model.Model{Cols: 160, Workspace: "/home/dev/my-repo", Actor: "alice"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{
		Catalog: pickerCatalog(),
		Issues:  []model.Issue{{UID: "a", ShortID: "aaaa", Title: "First"}},
	}})
	return pressed(driver.State{Model: m}, "escape", "end", "enter")
}

// cardButtonsOf are the buttons the card on top of s draws, by node id: every key click under a layer or the Menu.
func cardButtonsOf(t *testing.T, s driver.State) (map[string]string, map[string]string) {
	t.Helper()
	out := step(t, s, resize(160))
	found, labels := map[string]string{}, map[string]string{}
	for _, node := range wireNodes(t, *out.View) {
		actions, _ := node.P["actions"].(map[string]any)
		clickAct, _ := actions["click"].(string)
		if strings.HasPrefix(clickAct, view.KeyClick("")) && (strings.HasPrefix(node.ID, "layer.") || strings.HasPrefix(node.ID, view.PickerID)) {
			found[node.ID] = clickAct
			labels[node.ID], _ = node.P["text"].(string)
		}
	}
	return found, labels
}

// Every button of every Popup card state that draws buttons ends where the key it names ends: the chord it sends is bound
// in the card's mode, and its label names the card's keys for that action.
func TestCardButtonsClickLikeTheirKeys(t *testing.T) {
	issue := driver.State{Model: model.Model{Actor: "reviewer", SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Labels: []string{"bug"}}}}}
	creating := driver.State{Model: model.Model{Project: model.Project{ID: 7}}}
	titled, _ := inputs(chord("n"), tern.Key{Text: "title"})(creating)
	asking := pressed(titled, "escape")
	draftModel := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a"}, &model.IssueDetailData{Issue: &model.Issue{UID: "a", ProjectID: 2, Body: "a😀b", Revision: 7}})
	draft := pressed(driver.State{Model: draftModel}, "enter", "e")
	typed, _ := inputs(tern.Key{Name: "paste", Text: "more"})(draft)
	asked := pressed(typed, "escape")
	conflict := driver.State{Model: draft.Model}
	conflict.Model, _ = model.Update(conflict.Model, model.DraftConflict{UID: "a", Issue: model.Issue{UID: "a", ProjectID: 2, Body: "Remote", Revision: 8}})
	replyModel := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a"}, &model.IssueDetailData{Issue: &model.Issue{UID: "a", ProjectID: 2, Revision: 1}, Comments: []model.Comment{{UID: "c1", Author: "dana", Body: "claim"}}})
	reply := pressed(driver.State{Model: replyModel}, "enter", "shift+r")
	duplicate := driver.State{Model: reply.Model}
	duplicate.Model, _ = model.Update(duplicate.Model, model.DraftFailed{UID: "a", Err: model.DuplicateReply{Message: "a reply of this kind already exists"}})
	recurrence := int64(4)
	recurring := func(rec *model.Recurrence) driver.State {
		m := withDetail(model.Model{Workspace: "/repo", SelectedUID: "a", Issues: []model.Issue{{UID: "a", ProjectID: 2, RecurrenceID: &recurrence}}},
			&model.IssueDetailData{Issue: &model.Issue{UID: "a", ProjectID: 2, RecurrenceID: &recurrence}, Recurrence: rec})
		return pressed(driver.State{Model: m}, "shift+o")
	}
	change := recurring(&model.Recurrence{UID: "rec", RecurrenceTerms: model.RecurrenceTerms{Rule: "FREQ=DAILY;INTERVAL=1", Start: "2026-10-19", TimeZone: "UTC"}, Revision: 3})
	bound := browsing(t, 160)
	for _, tt := range []struct {
		name  string
		start driver.State
		mode  model.Mode
	}{
		{"Create", titled, model.ModeCreate},
		{"Create discard prompt", asking, model.ModeDiscard},
		{"Draft editor", draft, model.ModeEditor},
		{"reply Draft", reply, model.ModeEditor},
		{"duplicate reply Draft", duplicate, model.ModeEditor},
		{"Draft discard prompt", asked, model.ModeDiscard},
		{"Draft conflict", conflict, model.ModeConflict},
		{"Close", pressed(issue, "x"), model.ModeCloseForm},
		{"Recurrence make", pressed(issue, "shift+o"), model.ModeRecurrenceForm},
		{"Recurrence change", change, model.ModeRecurrenceForm},
		{"Recurrence stop", pressed(change, "ctrl+x"), model.ModeRecurrenceStop},
		{"Recurrence error", recurring(nil), model.ModeRecurrenceForm},
		{"Lanes", func() driver.State {
			s, _ := inputs(chord("b"), model.LanesLoaded{ProjectUID: "p"}, chord("shift+b"))(bound)
			return s
		}(), model.ModeLanesForm},
		{"Filter", pressed(issue, "f"), model.ModeFilterForm},
		{"single field", pressed(issue, "+"), model.ModeFieldForm},
		{"New Project", newProject(), model.ModeProjectForm},
		{"Menu", step(t, bound, openMenu).State, model.ModePicker},
		{"Menu Projects", pressed(bound, "shift+p"), model.ModePicker},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.start.Model.CurrentMode(); got != tt.mode {
				t.Fatalf("mode %s, want %s", got, tt.mode)
			}
			buttons, labels := cardButtonsOf(t, tt.start)
			if len(buttons) == 0 {
				t.Fatal("the card draws no buttons")
			}
			for id, act := range buttons {
				chord := strings.TrimPrefix(act, view.KeyClick(""))
				binding, ok := model.LookupBindingForMode(chord, tt.mode)
				if !ok || binding.Mode != tt.mode {
					t.Errorf("%s sends %q, which %s does not bind", id, chord, tt.mode)
					continue
				}
				keyboard, keyCmds := keys(chord)(tt.start)
				pointer, clickCmds := clicked(tt.start, click(id, act))
				sameAs(t, id+" differs from its key", pointer, clickCmds, keyboard, keyCmds)
				var labelKeys []string
				for _, b := range model.CardKeys(tt.mode, binding.Action) {
					labelKeys = append(labelKeys, b.Key)
				}
				if want := " (" + strings.Join(labelKeys, " / ") + ")"; !strings.HasSuffix(labels[id], want) || labels[id] == want {
					t.Errorf("%s label %q is not words followed by %q", id, labels[id], want)
				}
			}
		})
	}
}

// withDetail is m with the detail of its selected Issue loaded.
func withDetail(m model.Model, authority *model.IssueDetailData) model.Model {
	m, _ = model.Update(m, model.ResizeMsg{Cols: m.Cols})
	m, _ = model.Update(m, model.DetailLoaded{UID: m.SelectedUID, Selected: authority})
	return m
}
