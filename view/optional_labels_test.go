package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func labelTexts(t *testing.T, m model.Model, layer string, keys []string) []any {
	t.Helper()
	nodes := pickerNodes(t, m)
	var got []any
	for _, key := range keys {
		node, ok := nodes[layer+"."+key+"-label"]
		if !ok {
			t.Fatalf("missing %s.%s-label", layer, key)
		}
		got = append(got, node.P["text"])
	}
	return got
}

func TestRenderCreateLabelsMarkOptionalFields(t *testing.T) {
	want := []any{"Title", "Body (optional)", "Labels (optional, comma-separated)", "Owner (optional)", "Priority (optional, 0-4)"}
	keys := []string{"title", "body", "labels", "owner", "priority"}
	for _, chord := range []string{"n", "shift+n"} {
		m, _ := model.Update(selectedFirst(), model.KeyMsg{Chord: chord, Text: chord})
		got := labelTexts(t, m, "layer.create", keys)
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: label %d = %v, want %v", chord, i, got[i], want[i])
			}
		}
	}
}

func TestRenderCloseLabelsAboveEveryInput(t *testing.T) {
	want := []any{
		"Reason (done / wontfix / duplicate / superseded / audit-no-change)",
		"Message",
		"Commit SHA (optional)",
		"PR URL (optional)",
		"Test command (optional)",
		"Issue reference (duplicate / superseded)",
		"Rationale (audit-no-change)",
	}
	m, _ := model.Update(selectedFirst(), model.KeyMsg{Chord: "x", Text: "x"})
	got := labelTexts(t, m, "layer.close", []string{"0", "1", "2", "3", "4", "5", "6"})
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func selectedFirst() model.Model {
	m := pickerLoaded(true)
	m.SelectedUID = "a"
	return m
}
