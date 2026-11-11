package view_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

const phoneCols = 47

func atCols(m model.Model, cols int) model.Model { return apply(m, model.ResizeMsg{Cols: cols}) }

// pageChips are the Page bar's chips by Page id, wherever the bar sits.
func pageChips(t *testing.T, nodes shown, m model.Model) map[string]string {
	t.Helper()
	root := "main.pagebar"
	if _, ok := nodes[root]; !ok {
		root = "main.topbar.left.pages"
	}
	chips := map[string]string{}
	for _, entry := range m.PageBar() {
		chip, ok := nodes[root+"."+entry.ID]
		if !ok {
			t.Fatalf("no chip for %s under %s: %v", entry.ID, root, nodes.under("main.topbar"))
		}
		label := ""
		for _, id := range nodes.under(chip.ID) {
			label += text(nodes[id])
			if nodes[id].K == "icon" {
				label += "icon:" + nodes[id].P["name"].(string)
			}
		}
		chips[entry.ID] = label
	}
	return chips
}

func TestNarrowPageBarNamesEveryPage(t *testing.T) {
	m := atCols(browsing(), phoneCols)
	nodes := render(t, m)
	for id, label := range pageChips(t, nodes, m) {
		if strings.Trim(label, "0123456789 ") == "" {
			t.Errorf("%s chip is %q, only a digit", id, label)
		}
	}
}

func TestNarrowPageBarKeepsEachChipsClickAndTitle(t *testing.T) {
	m := atCols(browsing(), phoneCols)
	nodes := render(t, m)
	root := "main.pagebar"
	if _, ok := nodes[root]; !ok {
		root = "main.topbar.left.pages"
	}
	for _, entry := range m.PageBar() {
		chip := nodes[root+"."+entry.ID]
		if chip.P["title"] != entry.Label+" page" || chip.P["actions"] == nil {
			t.Errorf("%s chip props = %v", entry.ID, chip.P)
		}
	}
}

// The Attention count is the one sign of a waiting agent on a narrow Pane, whose header leaves out the counts.
func TestNarrowPageBarKeepsTheAttentionCountBesideItsIcon(t *testing.T) {
	m := browsing()
	m.Attention.Requests = make([]model.AttentionRequest, 3)
	m = atCols(m, phoneCols)
	if label := pageChips(t, render(t, m), m)["attention"]; !strings.Contains(label, "icon:") || !strings.Contains(label, "3") {
		t.Errorf("attention chip is %q, want its icon and the count 3", label)
	}
}

func TestEightyColumnPageBarKeepsKeycapsAndLabels(t *testing.T) {
	m := atCols(browsing(), model.FoldedLegendCols)
	nodes := render(t, m)
	if len(nodes.under("main.topbar")) == 0 {
		t.Fatal("no top bar")
	}
	for _, id := range nodes.under("main") {
		if nodes[id].K == "icon" && strings.Contains(id, "pages.") {
			t.Errorf("%s is an icon chip at %d columns", id, model.FoldedLegendCols)
		}
	}
}

func detailAt(cols int) model.Model {
	return apply(atCols(browsing(), cols), model.KeyMsg{Chord: "enter"})
}

func tabNodes(nodes shown) []tern.WireNode {
	var tabs []tern.WireNode
	for _, id := range nodes.under("main") {
		if i := strings.Index(id, ".tabs.tab"); i >= 0 && !strings.Contains(id[i+len(".tabs.tab"):], ".") {
			tabs = append(tabs, nodes[id])
		}
	}
	return tabs
}

func TestNarrowActivityTabsAreOneLineOfUnbrokenLabels(t *testing.T) {
	nodes := render(t, detailAt(phoneCols))
	tabs := tabNodes(nodes)
	if len(tabs) != int(model.TabCount) {
		t.Fatalf("tabs = %d, want %d", len(tabs), model.TabCount)
	}
	for _, tab := range tabs {
		if tab.P["wrap"] != "none" || fmt.Sprint(tab.P["shrink"]) != "0" {
			t.Errorf("%s props = %v, want wrap none and no shrink", tab.ID, tab.P)
		}
		if strings.Contains(text(tab), "(") {
			t.Errorf("%s shows a count: %q", tab.ID, text(tab))
		}
	}
}

func TestEightyColumnActivityTabsKeepCounts(t *testing.T) {
	nodes := render(t, detailAt(model.FoldedLegendCols))
	tabs := tabNodes(nodes)
	if len(tabs) != int(model.TabCount) || !strings.Contains(text(tabs[0]), "Comments (0)") {
		t.Fatalf("tabs = %v", tabs)
	}
	if _, ok := tabs[0].P["wrap"]; ok {
		t.Errorf("tab props = %v, want no wrap override", tabs[0].P)
	}
}

func TestNarrowHeaderDropsCountsAndDaemonKeepsMenu(t *testing.T) {
	nodes := render(t, atCols(browsing(), phoneCols))
	for _, id := range nodes.under("main.topbar") {
		if strings.HasSuffix(id, ".counts") || strings.HasSuffix(id, ".daemon") || strings.HasSuffix(id, ".sep") {
			t.Errorf("%s drawn at %d columns", id, phoneCols)
		}
	}
	if _, ok := nodes["main.topbar.right.menu"]; !ok {
		t.Errorf("no menu button: %v", nodes.under("main.topbar"))
	}
}

func TestEightyColumnHeaderKeepsCountsAndDaemon(t *testing.T) {
	nodes := render(t, atCols(browsing(), model.FoldedLegendCols))
	for _, id := range []string{"main.topbar.counts", "main.topbar.right.daemon", "main.topbar.right.menu"} {
		if _, ok := nodes[id]; !ok {
			t.Errorf("%s is gone at %d columns", id, model.FoldedLegendCols)
		}
	}
}

func TestNarrowAgentCardButtonsWrapInOrderWithCancelLast(t *testing.T) {
	m := model.Model{Cols: phoneCols, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	m = press(m, "w")
	nodes := pickerNodes(t, m)
	foot := nodes["layer.agent.foot"]
	var keys []string
	for _, group := range foot.C {
		for _, row := range group.C {
			if row.ID == group.ID+".buttons" {
				for _, b := range row.C {
					keys = append(keys, strings.TrimPrefix(b.ID, row.ID+"."))
				}
			}
		}
	}
	want := []string{"split", "tab", "new-background", "team", "pause", "go", "background", "message", "quick-reply", "interrupt", "stop", "cancel"}
	if strings.Join(keys, " ") != strings.Join(want, " ") {
		t.Errorf("buttons = %v, want %v", keys, want)
	}
	if _, ok := nodes["layer.agent.foot.new.cancel"]; ok {
		t.Error("Cancel is still beside the first group")
	}
}
