package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// click is the Action input Tern sends for a click on node id that runs
// action: Tern splits `name=value` at the first `=`.
func click(id, action string) *tern.Action {
	act, value, _ := strings.Cut(action, "=")
	return &tern.Action{ID: id, Act: act, Value: value}
}

// wireNodes is every node Tern would hold after v is rendered.
func wireNodes(t *testing.T, v tern.View) []tern.WireNode {
	t.Helper()
	ops, err := tern.NewReconciler("test").Diff(v)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var nodes []tern.WireNode
	var add func(tern.WireNode)
	add = func(n tern.WireNode) {
		nodes = append(nodes, n)
		for _, child := range n.C {
			add(child)
		}
	}
	for _, op := range ops {
		if op[0] == "add" {
			add(op[4].(tern.WireNode))
		}
	}
	return nodes
}

// activity is alpha, a parent of kid, with comments, events and a Link to beta, on the given Activity tab.
func activity(t *testing.T, tab model.ActivityTab) driver.State {
	t.Helper()
	kid := model.Issue{UID: "kid", ShortID: "kidd", Title: "Kid", Status: "open", ProjectName: "work"}
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	selected := &model.IssueDetailData{Issue: &alpha,
		Comments: []model.Comment{{UID: "c1", Author: "dana", CreatedAt: at, Body: "first"}},
		History:  []model.Event{{UID: "e1", Actor: "dana", Type: "issue.reopened", Payload: `{}`, CreatedAt: at}},
		Links: []model.Link{
			{ID: 1, FromIssueUID: "alpha", ToIssueUID: "beta", ToQualifiedID: "work#bbbb", Type: "related"},
			{ID: 2, FromIssueUID: "kid", FromQualifiedID: "work#kidd", ToIssueUID: "alpha", ToQualifiedID: "work#aaaa", Type: "parent"},
		},
	}
	return loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"}, model.Snapshot{
		Catalog: []model.CatalogProject{{Project: model.Project{UID: "p", Name: "work"}}},
		Issues:  []model.Issue{alpha, beta, kid},
		Links:   []model.Link{{Type: "parent", FromIssueUID: "kid", ToIssueUID: "alpha"}},
	}, model.DetailLoaded{UID: "alpha", Selected: selected}, model.ActivityTabMsg{Tab: tab})
}

// Every click action the view draws is one the driver can decode: a node the view makes clickable
// never goes dead (a renamed action, a payload the decoder does not read).
func TestEveryClickTheViewDrawsDecodesToAMessage(t *testing.T) {
	states := map[string]driver.State{
		"first load":          feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State,
		"error":               feed(t, browsing(t, 160), model.Failed{Err: errors.New("kata server not responding")}).State,
		"list":                browsing(t, 160),
		"board":               boardPane(t, alpha, beta, closedAt("shut", time.October, 7)),
		"board relationships": selectedBoard(t, "alpha", []model.Link{childOf(1, "alpha", "open", "beta"), relatedTo(2, "alpha", "far")}, alpha, beta),
		"board help":          feed(t, boardPane(t, alpha), key("?")).State,
		"lane view":           feed(t, boardPane(t, alpha, beta), key("enter")).State,
		"lane narrow":         feed(t, projectPane(t, 120, alpha, beta), key("b")).State,
		"narrow":              browsing(t, 100),
		"help":                feed(t, browsing(t, 160), key("?")).State,
		"menu":                feed(t, browsing(t, 160), openMenu).State,
		"new project":         feed(t, browsing(t, 160), openMenu, key("end"), key("enter")).State,
		"create":              feed(t, browsing(t, 160), key("n")).State,
		"close":               feed(t, browsing(t, 160), key("x")).State,
		"field form":          feed(t, browsing(t, 160), key("+")).State,
		"filters":             feed(t, browsing(t, 160), key("f")).State,
		"lanes card":          feed(t, boardPane(t, alpha), key("shift+b")).State,
		"recurrence":          feed(t, browsing(t, 160), key("shift+o")).State,
		"draft":               feed(t, browsing(t, 160), key("enter"), key("e")).State,
	}
	for tab := model.TabComments; tab < model.TabCount; tab++ {
		states["detail "+tab.String()] = activity(t, tab)
		states["detail "+tab.String()+" focused"] = feed(t, activity(t, tab), key("enter")).State
	}
	for name, s := range states {
		t.Run(name, func(t *testing.T) {
			clicks := 0
			for _, node := range wireNodes(t, view.Render(s.Model, now)) {
				actions, _ := node.P["actions"].(map[string]any)
				for _, kind := range []string{"click", "dblclick"} {
					action, ok := actions[kind].(string)
					if !ok {
						continue
					}
					clicks++
					out := driver.Step(s, click(node.ID, action), now)
					if out.View == nil && !out.Quit {
						t.Errorf("%s on %s: %q decodes to no message", kind, node.ID, action)
					}
				}
			}
			if clicks == 0 {
				t.Error("the view draws no click action: the property checked nothing")
			}
		})
	}
}

// Issue rows and the Menu list send Select inputs; Tern puts the item id of the row in them.
func TestSelectInputsDecodeForTheListAndBothMenus(t *testing.T) {
	s := browsing(t, 160)
	if got := driver.Step(s, &tern.Select{ID: view.IssueListID, Item: issueRow(t, s.Model, view.IssueListID, beta)}, now).State.Model.SelectedUID; got != "beta" {
		t.Errorf("Issue row select picked %q", got)
	}
	unbound := loadedWith(t, 160, model.Project{}, model.Snapshot{Issues: []model.Issue{alpha}})
	if out := driver.Step(unbound, &tern.Select{ID: view.CollectionsID, Item: rowShowing(t, unbound.Model, view.CollectionsID, "Today")}, now); len(out.Cmds) != 1 {
		t.Errorf("sidebar select commands = %#v", out.Cmds)
	}
	menu := feed(t, s, openMenu).State
	if out := driver.Step(menu, &tern.Select{ID: view.PickerListID, Item: rowShowing(t, menu.Model, view.PickerListID, "Today")}, now); out.State.Model.Picker.Open {
		t.Error("Menu select left the Menu open")
	}
}

// Tern's focus input on any field of the open card is "focus that field of that card": the Close card included.
func TestClickingAFieldMakesItTheFieldTheKeysEdit(t *testing.T) {
	for _, tt := range []struct {
		name  string
		open  string
		id    func(model.Field) string
		field model.Field // the field three tabs from the first
		keys  []string
		read  func(model.Model) model.Field
	}{
		{"Create", "n", view.CreateFieldID, model.CreateOwner, []string{"tab", "tab", "tab"}, func(m model.Model) model.Field { return m.Create.Field }},
		{"Close", "x", view.CloseFieldID, model.ClosePR, []string{"tab", "tab", "tab"}, func(m model.Model) model.Field { return m.CloseForm.Field }},
		{"Recurrence", "shift+o", view.RecurrenceFieldID, model.RecurrenceStart, []string{"tab", "tab", "tab"}, func(m model.Model) model.Field { return m.RecurrenceForm.Field }},
		{"Filters", "f", view.FilterFieldID, model.FilterLabel, []string{"tab", "tab", "tab"}, func(m model.Model) model.Field { return m.FilterForm.Field }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened := feed(t, browsing(t, 160), key(tt.open)).State
			clicked := step(t, opened, &tern.Focus{ID: tt.id(tt.field)})
			var keys []any
			for _, chord := range tt.keys {
				keys = append(keys, key(chord))
			}
			typed := feed(t, opened, keys...)
			if tt.read(clicked.State.Model) != tt.field || !reflect.DeepEqual(clicked.State.Model, typed.State.Model) {
				t.Fatalf("click on %q left field %q, keys left %q", tt.field, tt.read(clicked.State.Model), tt.read(typed.State.Model))
			}
			wantOps(t, clicked, tern.OpFocus(tt.id(tt.field)))
		})
	}
}

// A click on one choice of a card's choice field is the keys that focus that field and reach that choice: the click Tern
// sends is the one the view draws on it.
func TestClickingAChoiceIsTheKeysThatReachIt(t *testing.T) {
	weekdays := func(s driver.State) []string {
		keys := []string{"tab", "tab"} // Frequency, Every, Weekdays
		for range (4 - s.Model.RecurrenceForm.WeekdayAt + 7) % 7 {
			keys = append(keys, "right")
		}
		return append(keys, "space")
	}
	for _, tt := range []struct {
		name, open, id string
		keys           func(driver.State) []string
	}{
		{"Recurrence frequency", "shift+o", view.RecurrenceFieldID(model.RecurrenceFrequency) + ".choices.2", func(driver.State) []string { return []string{"right"} }}, // Weekly to Monthly
		{"Recurrence weekday", "shift+o", view.RecurrenceFieldID(model.RecurrenceWeekdays) + ".choices.4", weekdays},                                                    // Friday
		{"Filter status", "f", view.FilterFieldID(model.FilterStatus) + ".choices.1", func(driver.State) []string { return []string{"tab", "right"} }},                  // open
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened := feed(t, browsing(t, 160), key(tt.open)).State
			var action string
			for _, node := range wireNodes(t, view.Render(opened.Model, now)) {
				if node.ID == tt.id {
					actions, _ := node.P["actions"].(map[string]any)
					action, _ = actions["click"].(string)
				}
			}
			if action == "" {
				t.Fatalf("no click drawn on %s", tt.id)
			}
			clicked := step(t, opened, click(tt.id, action))
			var keys []any
			for _, chord := range tt.keys(opened) {
				keys = append(keys, key(chord))
			}
			typed := feed(t, opened, keys...)
			if !reflect.DeepEqual(clicked.State.Model, typed.State.Model) {
				t.Fatalf("click on %s left\n%+v\nkeys left\n%+v", tt.id, clicked.State.Model.RecurrenceForm, typed.State.Model.RecurrenceForm)
			}
		})
	}
}

func TestTextTypedAfterClickingACloseFieldGoesToThatField(t *testing.T) {
	s := feed(t, browsing(t, 160), key("x"), &tern.Focus{ID: view.CloseFieldID(model.CloseCommit)}, key("a")).State
	if values := s.Model.CloseForm.Values; values.Commit != "a" || values.Message != "" {
		t.Fatalf("values = %+v", values)
	}
}
