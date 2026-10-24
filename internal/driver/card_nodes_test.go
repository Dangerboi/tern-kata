package driver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// The finders below read the card on top the way a person does: a field by the label or placeholder it draws, a button by its
// text, a card by its title. They return ids of the rendered nodes, which the test then sends Tern's input to or reads text
// from, so a test never builds a node id from a view helper.

// cardLayerID is the id of the card on top: the last modal layer the view draws.
func cardLayerID(t *testing.T, m model.Model) string {
	t.Helper()
	var id string
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if modal, _ := n.P["modal"].(bool); modal && strings.HasPrefix(n.ID, "layer.") && strings.Count(n.ID, ".") == 1 {
			id = n.ID
		}
	}
	if id == "" {
		t.Fatal("no card is drawn")
	}
	return id
}

// layerTitled is the id of the card layer whose heading reads head.
func layerTitled(t *testing.T, m model.Model, head string) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if h, _ := n.P["head"].(string); h == head && strings.HasPrefix(n.ID, "layer.") && strings.Count(n.ID, ".") == 1 {
			return n.ID
		}
	}
	t.Fatalf("no card titled %q is drawn", head)
	return ""
}

func hasCursor(n tern.WireNode) bool { _, ok := n.P["cursor"]; return ok }

// cardFields are the nodes of the card on top that draw a field: a text input, a multi-line body or a group of choices, each
// a direct child of the layer, in the order they are drawn.
func cardFields(t *testing.T, m model.Model) []tern.WireNode {
	t.Helper()
	layer := cardLayerID(t, m)
	var fields []tern.WireNode
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if !strings.HasPrefix(n.ID, layer+".") || strings.Count(n.ID, ".") != 2 {
			continue
		}
		if aria, _ := n.P["aria"].(string); aria != "" || hasCursor(n) {
			fields = append(fields, n)
		}
	}
	return fields
}

// fieldLabeled is the id of the field of the card on top that draws label, as its accessible name, as the text just before it, or
// as its placeholder; it fails unless exactly one does.
func fieldLabeled(t *testing.T, m model.Model, label string) string {
	t.Helper()
	var found []string
	for _, n := range cardFields(t, m) {
		if aria, _ := n.P["aria"].(string); aria == label {
			found = append(found, n.ID)
		}
	}
	if len(found) == 0 { // a field with no accessible name follows the text that labels it
		layer := cardLayerID(t, m)
		labelled := false
		for _, n := range wireNodes(t, view.Render(m, now)) {
			if !strings.HasPrefix(n.ID, layer+".") {
				continue
			}
			if text, _ := n.P["text"].(string); text == label && n.P["tag"] == nil {
				labelled = true
				continue
			}
			if labelled && hasCursor(n) {
				found = append(found, n.ID)
				break
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("fields of the card on top labelled %q = %v, want exactly one", label, found)
	}
	return found[0]
}

// cardInput is the id of the one text input of the card on top.
func cardInput(t *testing.T, m model.Model) string {
	t.Helper()
	var inputs []string
	for _, n := range cardFields(t, m) {
		if hasCursor(n) {
			inputs = append(inputs, n.ID)
		}
	}
	if len(inputs) != 1 {
		t.Fatalf("inputs of the card on top = %v, want exactly one", inputs)
	}
	return inputs[0]
}

// focusedField is the node of the card on top that is drawn as the focused field (the accent tone; the body of a card with other fields is accent always, and counts only when it is the one).
func focusedField(t *testing.T, m model.Model) tern.WireNode {
	t.Helper()
	var found []tern.WireNode
	for _, n := range cardFields(t, m) {
		if n.P["tone"] == "accent" {
			found = append(found, n)
		}
	}
	if len(found) > 1 { // a multi-line body is always drawn in the accent tone
		found = slices.DeleteFunc(found, func(n tern.WireNode) bool { return n.P["maxLines"] != nil })
	}
	if len(found) != 1 {
		t.Fatalf("fields drawn as focused = %d, want exactly one", len(found))
	}
	return found[0]
}

// caretNode is the node Tern is told to focus while the card has a field focused: the field's own when it takes text, none
// for a choice field.
func caretNode(t *testing.T, m model.Model) string {
	t.Helper()
	if n := focusedField(t, m); hasCursor(n) {
		return n.ID
	}
	return ""
}

// buttonOnCard is the id of the button of the card on top whose text starts with text.
func buttonOnCard(t *testing.T, m model.Model, text string) string {
	t.Helper()
	layer := cardLayerID(t, m)
	var found []string
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if label, _ := n.P["text"].(string); strings.HasPrefix(n.ID, layer+".") && n.P["tag"] == "button" && strings.HasPrefix(label, text) {
			found = append(found, n.ID)
		}
	}
	if len(found) != 1 {
		t.Fatalf("buttons of the card on top starting %q = %v, want exactly one", text, found)
	}
	return found[0]
}

// textOnCard is the text of the node of the card on top that draws a text starting with prefix, and whether one does.
func textOnCard(t *testing.T, m model.Model, prefix string) (string, bool) {
	t.Helper()
	layer := cardLayerID(t, m)
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if text, _ := n.P["text"].(string); strings.HasPrefix(n.ID, layer+".") && n.P["tag"] == nil && !hasCursor(n) && strings.HasPrefix(text, prefix) {
			return text, true
		}
	}
	return "", false
}

// errorOnCard is the problem the card on top draws, and whether it draws one.
func errorOnCard(t *testing.T, m model.Model) (string, bool) {
	t.Helper()
	layer := cardLayerID(t, m)
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if text, _ := n.P["text"].(string); strings.HasPrefix(n.ID, layer+".") && n.P["tone"] == "error" {
			return text, true
		}
	}
	return "", false
}

// pageBarEntryAt is the id of the Page bar's entry that draws chord as its key, wherever the bar sits, and whether one is drawn.
func pageBarEntryAt(t *testing.T, m model.Model, chord string) (string, bool) {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(m, now)) {
		for _, bar := range []string{".pages.", ".pagebar."} {
			if i := strings.Index(n.ID, bar); i >= 0 && !strings.Contains(n.ID[i+len(bar):], ".") && slices.Contains(drawnPieces(n), chord) {
				return n.ID, true
			}
		}
	}
	return "", false
}

// pageBarEntry is pageBarEntryAt that fails the test when the Page bar draws no such entry.
func pageBarEntry(t *testing.T, m model.Model, chord string) string {
	t.Helper()
	id, ok := pageBarEntryAt(t, m, chord)
	if !ok {
		t.Fatalf("the Page bar draws no entry for key %q", chord)
	}
	return id
}

// choiceShowing is the id of the choice that the field labelled label draws as choice, whether or not it is the chosen one
// (the chosen one is drawn in brackets).
func choiceShowing(t *testing.T, m model.Model, label, choice string) string {
	t.Helper()
	field := fieldLabeled(t, m, label)
	var found []string
	for _, n := range wireNodes(t, view.Render(m, now)) {
		rest, ok := strings.CutPrefix(n.ID, field+".choices.")
		if !ok || strings.Contains(rest, ".") {
			continue
		}
		for _, piece := range drawnPieces(n) {
			if strings.Trim(piece, "[] ") == choice {
				found = append(found, n.ID)
				break
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("choices of %q drawing %q = %v, want exactly one", label, choice, found)
	}
	return found[0]
}

// fieldDrawn says whether the card on top draws a field labelled label.
func fieldDrawn(t *testing.T, m model.Model, label string) bool {
	t.Helper()
	return slices.ContainsFunc(cardFields(t, m), func(n tern.WireNode) bool { aria, _ := n.P["aria"].(string); return aria == label })
}

// textAfterField is the text the card on top draws right under its field labelled label, "" when none follows it before the
// next field.
func textAfterField(t *testing.T, m model.Model, label string) string {
	t.Helper()
	field := fieldLabeled(t, m, label)
	after := false
	for _, n := range wireNodes(t, view.Render(m, now)) {
		switch {
		case n.ID == field:
			after = true
		case after && strings.HasPrefix(n.ID, field[:strings.LastIndex(field, ".")+1]):
			if hasCursor(n) || n.P["aria"] != nil || n.P["tag"] != nil {
				return ""
			}
			text, _ := n.P["text"].(string)
			return text
		}
	}
	return ""
}

// pageBarDrawn says whether the Page bar draws an entry for key chord.
func pageBarDrawn(t *testing.T, m model.Model, chord string) bool {
	t.Helper()
	_, ok := pageBarEntryAt(t, m, chord)
	return ok
}
