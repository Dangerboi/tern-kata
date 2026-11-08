package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTextEditorStepsByGrapheme(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		cursor     int
		action     model.Action
		want       string
		wantCursor int
	}{
		{"backspace flag", "a🇯🇵", 5, model.EditorBackspace, "a", 1},
		{"backspace family", "a👨\u200d👩\u200d👧", 9, model.EditorBackspace, "a", 1},
		{"backspace accent", "e\u0301", 2, model.EditorBackspace, "", 0},
		{"delete flag", "🇯🇵a", 0, model.EditorDelete, "a", 0},
		{"left over accent", "e\u0301", 2, model.EditorLeft, "e\u0301", 0},
		{"right over accent", "e\u0301x", 0, model.EditorRight, "e\u0301x", 2},
		{"left over flag", "a🇯🇵", 5, model.EditorLeft, "a🇯🇵", 1},
	} {
		e := model.NewTextEditor(tt.text)
		e.Cursor, e.Anchor = tt.cursor, tt.cursor
		e = e.Key(tt.action, false)
		if e.Text != tt.want || e.Cursor != tt.wantCursor {
			t.Errorf("%s: text %q cursor %d, want %q %d", tt.name, e.Text, e.Cursor, tt.want, tt.wantCursor)
		}
	}
}
