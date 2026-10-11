package model_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// draftCard describes a card that holds a multi-line Draft: Create and the Draft editor.
type draftCard struct {
	name string
	// open opens the card with an empty Draft; openText opens it so that save is allowed.
	open func() model.Model
	// body focuses the multi-line body of an opened card.
	body func(model.Model) model.Model
	// text is what the Draft holds; present says the card is on screen.
	text    func(model.Model) string
	present func(model.Model) bool
	// target names the card's body editor for native edits.
	target model.EditTarget
	// save submits the card; fail answers the save with an error.
	save string
	fail func(model.Model, error) model.Msg
}

func draftCards() []draftCard {
	return []draftCard{
		{
			name: "Create", open: openedCreate, target: model.EditCreateBody, save: "ctrl+o",
			body:    func(m model.Model) model.Model { m, _ = model.Update(m, model.FocusFieldMsg{Field: 1}); return m },
			text:    func(m model.Model) string { return m.Create.Editor.Text },
			present: func(m model.Model) bool { return m.Create.Open },
			fail:    func(_ model.Model, err error) model.Msg { return model.IssueCreated{Err: err} },
		},
		{
			name: "Draft editor", target: model.EditDraftBody, save: "ctrl+o",
			open: func() model.Model {
				m, _ := model.Update(editorModel(), model.KeyMsg{Chord: "c"})
				return m
			},
			body:    func(m model.Model) model.Model { return m },
			text:    func(m model.Model) string { d, _ := m.ActiveDraft(); return d.Editor.Text },
			present: func(m model.Model) bool { _, ok := m.ActiveDraft(); return ok },
			fail:    func(m model.Model, err error) model.Msg { return model.DraftFailed{UID: m.EditorUID, Err: err} },
		},
	}
}

// typed enters a body, as Tern sends a key whose text is its character.
func typed(m model.Model, text string) model.Model {
	m, _ = model.Update(m, model.KeyMsg{Chord: text, Text: text})
	return m
}

func TestDraftCardsTypePrintableKeysIntoTheBody(t *testing.T) {
	for _, c := range draftCards() {
		t.Run(c.name, func(t *testing.T) {
			m := c.body(c.open())
			for _, key := range []string{"q", "?", "p", "j"} {
				m = typed(m, key)
			}
			if got := c.text(m); got != "q?pj" || m.Quitting || m.HelpOpen {
				t.Fatalf("body %q quitting=%v help=%v", got, m.Quitting, m.HelpOpen)
			}
		})
	}
}

func TestDraftCardsShareDiscardConfirmation(t *testing.T) {
	for _, c := range draftCards() {
		t.Run(c.name+"/leaving with nothing typed does not ask", func(t *testing.T) {
			m, _ := clickPress(c.open(), "escape")
			if c.present(m) {
				t.Fatalf("esc on an empty Draft left mode %s", m.CurrentMode())
			}
		})
		asking := func() model.Model {
			m, _ := clickPress(typed(c.body(c.open()), "x"), "escape")
			if m.CurrentMode() != model.ModeDiscard {
				t.Fatalf("esc with text left mode %s, want a discard question", m.CurrentMode())
			}
			return m
		}
		t.Run(c.name+"/asking swallows other keys and edits", func(t *testing.T) {
			m := asking()
			m, _ = clickPress(typed(m, "z"), "ctrl+c", "tab")
			m, _ = model.Update(m, model.EditMsg{Target: c.target, From: 0, To: 0, Text: "e", Cursor: 1, Len: 1})
			if got := c.text(m); got != "x" || m.Quitting || m.CurrentMode() != model.ModeDiscard {
				t.Fatalf("while asking: text %q quitting=%v mode %s", got, m.Quitting, m.CurrentMode())
			}
		})
		for _, keep := range []string{"n", "escape"} {
			t.Run(c.name+"/"+keep+" keeps the text", func(t *testing.T) {
				m, _ := clickPress(asking(), keep)
				if !c.present(m) || m.CurrentMode() == model.ModeDiscard || c.text(m) != "x" {
					t.Fatalf("after %s: present=%v mode %s text %q", keep, c.present(m), m.CurrentMode(), c.text(m))
				}
			})
		}
		t.Run(c.name+"/y discards", func(t *testing.T) {
			m, _ := clickPress(asking(), "y")
			if c.present(m) {
				t.Fatalf("y left mode %s", m.CurrentMode())
			}
		})
	}
}

func TestDraftCardsSaveErrorKeepsTheDraft(t *testing.T) {
	for _, c := range draftCards() {
		t.Run(c.name, func(t *testing.T) {
			m := c.body(c.open())
			m = typed(typed(m, "k"), "p")
			if c.name == "Create" {
				m, _ = model.Update(m, model.FocusFieldMsg{Field: 0})
				m = typed(m, "t")
			}
			m, cmds := clickPress(m, c.save)
			if len(cmds) == 0 {
				t.Fatalf("save sent nothing: mode %s", m.CurrentMode())
			}
			// While saving, nothing edits the Draft.
			before := c.text(m)
			m, _ = model.Update(typed(m, "z"), model.EditMsg{Target: c.target, From: 0, To: 0, Text: "e", Cursor: 1, Len: 1})
			if c.text(m) != before {
				t.Fatalf("saving Draft edited: %q -> %q", before, c.text(m))
			}
			m, _ = model.Update(m, c.fail(m, errors.New("daemon said no")))
			if !c.present(m) || c.text(m) != before {
				t.Fatalf("after a save error: present=%v text %q, want %q", c.present(m), c.text(m), before)
			}
			// The card is editable again.
			if m = typed(c.body(m), "!"); c.text(m) != before+"!" {
				t.Fatalf("Draft not editable after the error: %q", c.text(m))
			}
		})
	}
}

func TestDraftCardsTakeNativeEditsAndUndo(t *testing.T) {
	for _, c := range draftCards() {
		t.Run(c.name, func(t *testing.T) {
			m, _ := model.Update(c.body(c.open()), model.EditMsg{Target: c.target, Text: "hi", Cursor: 2})
			if got := c.text(m); got != "hi" {
				t.Fatalf("after a native edit: %q", got)
			}
			if m, _ = model.Update(m, model.UndoMsg{Target: c.target}); c.text(m) != "" {
				t.Fatalf("after undo: %q", c.text(m))
			}
			// An edit for the other card's body does not reach this one.
			other := model.EditCreateBody
			if c.target == other {
				other = model.EditDraftBody
			}
			if m, _ = model.Update(m, model.EditMsg{Target: other, Text: "no", Cursor: 2}); c.text(m) != "" {
				t.Fatalf("the other card's edit landed: %q", c.text(m))
			}
		})
	}
}
