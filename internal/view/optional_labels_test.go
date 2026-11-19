package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func labelTexts(t *testing.T, m model.Model, layer string, keys []string) []any {
	t.Helper()
	nodes := pickerNodes(t, m)
	var got []any
	for _, key := range keys {
		node, ok := nodes[layer+"."+key+"-label.name"]
		if !ok {
			t.Fatalf("missing %s.%s-label.name", layer, key)
		}
		got = append(got, text(node))
	}
	return got
}

func TestRenderCreateLabelsAreNounsWithoutOptionalMarks(t *testing.T) {
	want := []any{"Title", "Body", "Labels", "Owner", "Scheduled", "Deadline"}
	keys := []string{"title", "body", "labels", "owner", "scheduled", "deadline"}
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
	want := []any{"Message", "Commit", "Pull request", "Test command", "Reviewed paths", "External account"}
	m, _ := model.Update(selectedFirst(), model.KeyMsg{Chord: "x", Text: "x"})
	got := labelTexts(t, m, "layer.close", []string{"message", "commit", "pr", "test", "reviewed", "external"})
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRenderCloseShowsTheReferenceAndRationaleOnlyForTheirReasons(t *testing.T) {
	m, _ := model.Update(selectedFirst(), model.KeyMsg{Chord: "x", Text: "x"})
	if _, ok := pickerNodes(t, m)["layer.close.reference-label"]; ok {
		t.Error("a done close shows the reference field")
	}
	m.CloseForm.Values.Reason = string(model.ReasonSuperseded)
	if got := text(pickerNodes(t, m)["layer.close.reference-label.name"]); got != "Replaced by" {
		t.Errorf("superseded reference label = %v", got)
	}
	m.CloseForm.Values.Reason = string(model.ReasonAuditNoChange)
	if got := text(pickerNodes(t, m)["layer.close.rationale-label.name"]); got != "Why nothing changed" {
		t.Errorf("rationale label = %v", got)
	}
}

func selectedFirst() model.Model {
	m := pickerLoaded(true)
	m.SelectedUID = "a"
	return m
}
