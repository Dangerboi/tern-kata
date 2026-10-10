package view_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func TestHelpIsGroupedKeyActionTables(t *testing.T) {
	m, _ := model.Update(pickerLoaded(true), model.KeyMsg{Chord: "?"})
	nodes := render(t, m)
	help := nodes["layer.help"]
	raw, err := json.Marshal(help)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	tables := 0
	for _, id := range nodes.under("layer.help") {
		if nodes[id].K == "table" {
			tables++
		}
	}
	if tables < 3 {
		t.Errorf("want a table per group, got %d", tables)
	}
	for _, want := range []string{`"Everywhere"`, `"list"`, `"detail"`, `"key"`, `"action"`, `"j/↓"`, `"next Issue"`} {
		if !strings.Contains(body, want) {
			t.Errorf("help lacks %s", want)
		}
	}
	if strings.Contains(body, "Tern headless") {
		t.Error("headless-only chords listed")
	}
	if strings.Count(body, `"next Issue"`) != 1 {
		t.Errorf("next Issue rows not merged: %d", strings.Count(body, `"next Issue"`))
	}
	closeBtn := nodes["layer.help.close"]
	if text(closeBtn) != "Close (esc)" {
		t.Errorf("close button = %q", text(closeBtn))
	}
	if !strings.Contains(string(mustJSON(t, closeBtn)), "key=escape") {
		t.Error("close button does not click through key=escape")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
