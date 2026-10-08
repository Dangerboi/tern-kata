package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
	g "go.kenn.io/kata/pkg/client/generated"
)

// pickerNodes renders m and indexes every node by the id Tern gives it.
func pickerNodes(t *testing.T, m model.Model) map[string]tern.WireNode {
	t.Helper()
	ops, err := tern.NewReconciler("picker").Diff(view.Render(m, testNow))
	if err != nil {
		t.Fatal(err)
	}
	nodes := map[string]tern.WireNode{}
	var walk func(tern.WireNode)
	walk = func(node tern.WireNode) {
		nodes[node.ID] = node
		for _, child := range node.C {
			walk(child)
		}
	}
	for _, op := range ops {
		if op[0] == "add" {
			walk(op[4].(tern.WireNode))
		}
	}
	return nodes
}

func pickerChildIDs(node tern.WireNode) []string {
	ids := make([]string, 0, len(node.C))
	for _, child := range node.C {
		ids = append(ids, child.ID)
	}
	return ids
}

func pickerLoaded(bound bool) model.Model {
	loaded := model.ProjectLoaded{Snapshot: g.UISnapshotResponseBody{
		Catalog: []g.UIProject{
			{Project: model.Project{UID: "alpha-uid", Name: "alpha"}},
			{Project: model.Project{UID: "smoke-uid", Name: "smoke"}},
		},
		Collection: []model.Issue{{UID: "a", ShortID: "aaaa", Title: "First"}},
	}}
	if bound {
		loaded.Project = model.Project{UID: "smoke-uid", Name: "smoke"}
	}
	m, _ := model.Update(model.Model{Cols: 160}, loaded)
	return m
}

func TestRenderCollectionsColumnOnlyWhenUnbound(t *testing.T) {
	for _, tt := range []struct {
		name    string
		bound   bool
		sidebar bool
	}{
		{"bound opens without the sidebar", true, false},
		{"unbound keeps the sidebar", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nodes := pickerNodes(t, pickerLoaded(tt.bound))
			if _, ok := nodes[view.CollectionsID]; ok != tt.sidebar {
				t.Fatalf("sidebar present = %v, want %v", ok, tt.sidebar)
			}
			for _, id := range []string{"main.columns.list", "main.columns.detail"} {
				if _, ok := nodes[id]; !ok {
					t.Fatalf("missing column %s", id)
				}
			}
		})
	}
}

func TestRenderPickerListsPinnedProjectFirstAndMarksCurrent(t *testing.T) {
	m := pickerLoaded(true)
	m, _ = model.Update(m, model.KeyMsg{Chord: "p", Text: "p"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "down"})
	nodes := pickerNodes(t, m)
	list, ok := nodes["layer.collections.list"]
	if !ok {
		t.Fatal("picker list not rendered")
	}
	prefix := "layer.collections.list."
	want := []string{"project.smoke-uid", "inbox", "today", "upcoming", "deadlines", "all-open", "logbook", "project.alpha-uid", "new-project"}
	var got []string
	for _, id := range pickerChildIDs(list) {
		got = append(got, strings.TrimPrefix(id, prefix))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("picker rows = %v, want %v", got, want)
	}
	if list.P["selected"] != prefix+"inbox" {
		t.Fatalf("selected row = %v, want the cursor entry", list.P["selected"])
	}
	for _, id := range pickerChildIDs(list) {
		current := nodes[id].P["value"] == "current"
		if current != (id == prefix+"project.smoke-uid") {
			t.Fatalf("%s current mark = %v", id, current)
		}
	}
}

func TestRenderPickerFollowsFilterAndShowsEmptyState(t *testing.T) {
	m := pickerLoaded(true)
	m, _ = model.Update(m, model.KeyMsg{Chord: "p", Text: "p"})
	for _, key := range []string{"a", "l"} {
		m, _ = model.Update(m, model.KeyMsg{Chord: key, Text: key})
	}
	nodes := pickerNodes(t, m)
	if got := pickerChildIDs(nodes["layer.collections.list"]); strings.Join(got, ",") != "layer.collections.list.all-open,layer.collections.list.project.alpha-uid" {
		t.Fatalf("filtered rows = %v", got)
	}
	if nodes["layer.collections.filter"].P["text"] != "al" {
		t.Fatalf("filter box = %v", nodes["layer.collections.filter"].P)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "z", Text: "z"})
	nodes = pickerNodes(t, m)
	if len(nodes["layer.collections.list"].C) != 0 || nodes["layer.collections.list"].P["empty"] == nil {
		t.Fatalf("empty picker: %v", nodes["layer.collections.list"])
	}
}

func TestRenderPickerOnlyWhenOpenAndFocusesItsFilter(t *testing.T) {
	m := pickerLoaded(true)
	if _, ok := pickerNodes(t, m)["layer.collections"]; ok || view.InputFocusID(m) != "" {
		t.Fatal("closed picker rendered or focused")
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "p", Text: "p"})
	if _, ok := pickerNodes(t, m)["layer.collections"]; !ok || view.InputFocusID(m) != view.PickerFilterID || view.PickerFilterID != "layer.collections.filter" {
		t.Fatalf("open picker: focus=%q", view.InputFocusID(m))
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	if _, ok := pickerNodes(t, m)["layer.collections"]; ok {
		t.Fatal("closed picker still rendered")
	}
}

func TestRenderTopBarTitleOpensPicker(t *testing.T) {
	heading := pickerNodes(t, pickerLoaded(true))["main.topbar.heading"]
	actions, _ := heading.P["actions"].(map[string]any)
	if actions["click"] != "key=p" {
		t.Fatalf("heading actions = %v", heading.P["actions"])
	}
}
