package model_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// cardState is one state of a card that owns the keyboard, with the mode it puts the keymap in.
type cardState struct {
	name string
	open func() model.Model
	mode model.Mode
}

// cardBody describes the multi-line Draft body of Create and the Draft editor.
type cardBody struct {
	// focus focuses the body of an opened card; ready makes the Draft saveable once it has text.
	focus, ready func(model.Model) model.Model
	// text is what the Draft holds; present says the card is on screen.
	text    func(model.Model) string
	present func(model.Model) bool
	// err is the card's save error.
	err func(model.Model) error
	// target names the card's body editor for native edits.
	target model.EditTarget
	// save submits the card; fail answers the save with an error.
	save string
	fail func(model.Model, error) model.Msg
}

// popupCard describes one card (or Search, which is not one) for the shared behaviour tests: how
// to open it, which field a click focuses, and how to read the text and the focused field back.
type popupCard struct {
	name string
	card model.Card // NoCard for Search
	// states are the keymap modes the card can be in; the first is the plain opened card.
	states []cardState
	// quits says ctrl+c quits from the card (the Menu, Close and New Project cards).
	quits bool
	// graft puts the card's state on another model, to open several at once.
	graft func(dst *model.Model, src model.Model)
	// multiline cards edit a body, not a single-line text field.
	multiline bool
	textField model.Field   // a text field of the card
	tabOrder  []model.Field // the card's fields in tab order, written out; nil for one field
	field     func(model.Model) model.Field
	text      func(model.Model, model.Field) string
	// save submits the card; result answers it with an error. Nil when the card does not save.
	save   []string
	result func(model.Model, error) model.Msg
	body   *cardBody
}

func (c popupCard) open() model.Model { return c.states[0].open() }

func opened(keys ...string) func() model.Model {
	return func() model.Model {
		m, _ := clickPress(clickAt(model.ColumnList), keys...)
		return m
	}
}

// openedMenu opens the Menu as a click on the top bar does.
func openedMenu() model.Model {
	m, _ := model.Update(clickAt(model.ColumnList), model.ActionMsg{Action: model.OpenPicker})
	return m
}

// openedCreate opens Create on a bound Project, so a card with a title can be saved.
func openedCreate() model.Model {
	m, _ := model.Update(model.Model{Project: model.Project{ID: 7}}, model.KeyMsg{Chord: "n"})
	return m
}

func openedDraft() model.Model {
	m, _ := model.Update(editorModel(), model.KeyMsg{Chord: "c"})
	return m
}

// typed enters a body, as Tern sends a key whose text is its character.
func typed(m model.Model, text string) model.Model {
	m, _ = model.Update(m, model.KeyMsg{Chord: text, Text: text})
	return m
}

// asking opens a card that holds a Draft and presses esc with text typed, which asks to discard.
func asking(c popupCard) func() model.Model {
	return func() model.Model {
		m, _ := clickPress(typed(c.body.focus(c.open()), "x"), "escape")
		return m
	}
}

func popupCards() []popupCard {
	create := popupCard{
		name: "Create", card: model.CardCreate, textField: model.CreateTitle,
		tabOrder: []model.Field{model.CreateTitle, model.CreateBody, model.CreateLabels, model.CreateOwner, model.CreatePriority},
		states:   []cardState{{"", openedCreate, model.ModeCreate}},
		graft:    func(dst *model.Model, src model.Model) { dst.Create = src.Create },
		field:    func(m model.Model) model.Field { return m.Create.Field },
		text:     func(m model.Model, _ model.Field) string { return m.Create.Title },
		save:     []string{"ctrl+o"},
		result:   func(_ model.Model, err error) model.Msg { return model.IssueCreated{Err: err} },
		body: &cardBody{
			target: model.EditCreateBody, save: "ctrl+o",
			focus: func(m model.Model) model.Model {
				m, _ = model.Update(m, model.FocusFieldMsg{Field: model.CreateBody})
				return m
			},
			ready: func(m model.Model) model.Model {
				m, _ = model.Update(m, model.FocusFieldMsg{Field: model.CreateTitle})
				return typed(m, "t")
			},
			text:    func(m model.Model) string { return m.Create.Editor.Text },
			present: func(m model.Model) bool { return m.Create.Open },
			err:     func(m model.Model) error { return m.Create.Err },
			fail:    func(_ model.Model, err error) model.Msg { return model.IssueCreated{Err: err} },
		},
	}
	create.states = append(create.states, cardState{"asking to discard", asking(create), model.ModeDiscard})
	draft := popupCard{
		name: "Draft editor", card: model.CardDraft, multiline: true,
		states: []cardState{{"", openedDraft, model.ModeEditor}},
		graft:  func(dst *model.Model, src model.Model) { dst.Drafts, dst.EditorUID = src.Drafts, src.EditorUID },
		field:  func(model.Model) model.Field { return "" },
		body: &cardBody{
			target: model.EditDraftBody, save: "ctrl+o",
			focus:   func(m model.Model) model.Model { return m },
			ready:   func(m model.Model) model.Model { return m },
			text:    func(m model.Model) string { d, _ := m.ActiveDraft(); return d.Editor.Text },
			present: func(m model.Model) bool { _, ok := m.ActiveDraft(); return ok },
			err:     func(m model.Model) error { d, _ := m.ActiveDraft(); return d.Err },
			fail:    func(m model.Model, err error) model.Msg { return model.DraftFailed{UID: m.EditorUID, Err: err} },
		},
	}
	draft.states = append(draft.states,
		cardState{"asking to discard", asking(draft), model.ModeDiscard},
		cardState{"with a conflict", func() model.Model {
			m := openedDraft()
			m, _ = model.Update(m, model.DraftConflict{UID: m.EditorUID, Issue: model.Issue{UID: m.EditorUID, Revision: 9}})
			return m
		}, model.ModeConflict})
	return []popupCard{
		create,
		draft,
		{
			name: "single-field form", card: model.CardField,
			states: []cardState{{"", opened("+"), model.ModeFieldForm}},
			graft:  func(dst *model.Model, src model.Model) { dst.FieldForm = src.FieldForm },
			field:  func(model.Model) model.Field { return "" },
			text:   func(m model.Model, _ model.Field) string { return m.FieldForm.Value },
			save:   []string{"enter"},
			result: func(m model.Model, err error) model.Msg { return model.FieldWritten{UID: m.FieldForm.UID, Err: err} },
		},
		{
			name: "Close", card: model.CardClose, quits: true, textField: model.CloseCommit,
			tabOrder: []model.Field{model.CloseReason, model.CloseMessage, model.CloseCommit, model.ClosePR, model.CloseTest, model.CloseReference, model.CloseRationale},
			states:   []cardState{{"", opened("x"), model.ModeCloseForm}},
			graft:    func(dst *model.Model, src model.Model) { dst.CloseForm = src.CloseForm },
			field:    func(m model.Model) model.Field { return m.CloseForm.Field },
			text:     func(m model.Model, f model.Field) string { return m.CloseForm.Values.Value(f) },
			save:     []string{"ctrl+d"},
			result:   func(m model.Model, err error) model.Msg { return model.CloseResultMsg{UID: m.CloseForm.UID, Err: err} },
		},
		{
			name: "Dates", card: model.CardDates, textField: model.DatesDeadline,
			tabOrder: []model.Field{model.DatesScheduled, model.DatesDeadline},
			states:   []cardState{{"", opened("d"), model.ModeDatesForm}},
			graft:    func(dst *model.Model, src model.Model) { dst.DatesForm = src.DatesForm },
			field:    func(m model.Model) model.Field { return m.DatesForm.Field },
			text:     func(m model.Model, f model.Field) string { return m.DatesForm.Value(f) },
		},
		{
			name: "Recurrence", card: model.CardRecurrence, quits: true, textField: model.RecurrenceStart,
			tabOrder: []model.Field{model.RecurrenceFrequency, model.RecurrenceEvery, model.RecurrenceWeekdays, model.RecurrenceStart, model.RecurrenceEnds, model.RecurrenceZone},
			states:   []cardState{{"", opened("shift+o"), model.ModeRecurrenceForm}},
			graft:    func(dst *model.Model, src model.Model) { dst.RecurrenceForm = src.RecurrenceForm },
			field:    func(m model.Model) model.Field { return m.RecurrenceForm.Field },
			text:     func(m model.Model, _ model.Field) string { return m.RecurrenceForm.Start },
		},
		{
			name: "New Project", card: model.CardProject, quits: true,
			states: []cardState{{"", func() model.Model {
				m, _ := model.Update(clickAt(model.ColumnList), model.CollectionPointerSelected{Key: model.NewProjectKey})
				m, _ = clickPress(m, "ctrl+u")
				return m
			}, model.ModeProjectForm}},
			graft:  func(dst *model.Model, src model.Model) { dst.ProjectForm = src.ProjectForm },
			field:  func(model.Model) model.Field { return "" },
			text:   func(m model.Model, _ model.Field) string { return m.ProjectForm.Name },
			save:   []string{"enter"},
			result: func(_ model.Model, err error) model.Msg { return model.ProjectSaved{Err: err} },
		},
		{
			name: "Lanes", card: model.CardLanes,
			states: []cardState{{"", func() model.Model {
				m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, model.SnapshotLoaded{})
				m, _ = clickPress(m, "b")
				m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p"})
				m, _ = clickPress(m, "shift+b", "ctrl+u")
				return m
			}, model.ModeLanesForm}},
			graft: func(dst *model.Model, src model.Model) { dst.LanesForm = src.LanesForm },
			field: func(model.Model) model.Field { return "" },
			text:  func(m model.Model, _ model.Field) string { return m.LanesForm.Text },
		},
		{
			name: "Filter form", card: model.CardFilter, textField: model.FilterOwner,
			tabOrder: []model.Field{model.FilterProject, model.FilterStatus, model.FilterOwner, model.FilterLabel, model.FilterRelationship},
			states:   []cardState{{"", opened("f"), model.ModeFilterForm}},
			graft:    func(dst *model.Model, src model.Model) { dst.FilterForm = src.FilterForm },
			field:    func(m model.Model) model.Field { return m.FilterForm.Field },
			text: func(m model.Model, f model.Field) string {
				if f == model.FilterLabel {
					return m.FilterForm.Values.Label
				}
				return m.FilterForm.Values.Owner
			},
		},
		{
			name: "Menu", card: model.CardPicker, quits: true,
			states: []cardState{{"", openedMenu, model.ModePicker}},
			graft:  func(dst *model.Model, src model.Model) { dst.Picker = src.Picker },
			field:  func(model.Model) model.Field { return "" },
			text:   func(m model.Model, _ model.Field) string { return m.Picker.Filter },
		},
		{
			name: "Search", card: model.NoCard,
			states: []cardState{{"", opened("/"), model.ModeSearch}},
			field:  func(model.Model) model.Field { return "" },
			text:   func(m model.Model, _ model.Field) string { return m.Filters.Search },
		},
	}
}

func typeKeys(m model.Model, keys ...model.KeyMsg) model.Model {
	for _, key := range keys {
		m, _ = model.Update(m, key)
	}
	return m
}

func TestPopupCardSharedBehaviour(t *testing.T) {
	for _, c := range popupCards() {
		focused := func() model.Model {
			m := c.open()
			if m.CurrentMode() == model.ModeList {
				t.Fatalf("%s did not open", c.name)
			}
			m, _ = model.Update(m, model.FocusFieldMsg{Field: c.textField})
			return m
		}
		if !c.multiline {
			t.Run(c.name+"/typing", func(t *testing.T) {
				// q is a global binding (quit) and space a binding in some modes: both are text here.
				m := typeKeys(focused(), model.KeyMsg{Chord: "q"}, model.KeyMsg{Chord: "space"}, model.KeyMsg{Chord: "x", Text: "x"})
				if got := c.text(m, c.textField); got != "q x" || m.Quitting {
					t.Fatalf("typed %q quitting=%v", got, m.Quitting)
				}
			})
			t.Run(c.name+"/backspace by rune", func(t *testing.T) {
				for _, tt := range []struct{ typed, want string }{{"aé", "a"}, {"a😀", "a"}} {
					m := typeKeys(focused(), model.KeyMsg{Text: tt.typed}, model.KeyMsg{Chord: "backspace"})
					if got := c.text(m, c.textField); got != tt.want {
						t.Fatalf("%q then backspace = %q, want %q", tt.typed, got, tt.want)
					}
				}
			})
			t.Run(c.name+"/clear", func(t *testing.T) {
				m := typeKeys(focused(), model.KeyMsg{Text: "abc"}, model.KeyMsg{Chord: "ctrl+u"})
				if got := c.text(m, c.textField); got != "" {
					t.Fatalf("after ctrl+u: %q", got)
				}
			})
			t.Run(c.name+"/paste is flattened", func(t *testing.T) {
				m := typeKeys(focused(), model.KeyMsg{Text: "one\ntwo\r\nthree"})
				if got := c.text(m, c.textField); got != "one two three" {
					t.Fatalf("pasted: %q", got)
				}
			})
		}
		t.Run(c.name+"/esc after a click", func(t *testing.T) {
			m := focused()
			card := m.CurrentMode()
			if m, _ = clickPress(m, "escape"); m.CurrentMode() == card {
				t.Fatalf("esc left the card open in mode %s", card)
			}
		})
		t.Run(c.name+"/click focuses the field", func(t *testing.T) {
			if got := c.field(focused()); got != c.textField {
				t.Fatalf("focused field %q, want %q", got, c.textField)
			}
			open := c.open()
			m, _ := model.Update(open, model.FocusFieldMsg{Field: "no-such-field"})
			if got := c.field(m); got != c.field(open) {
				t.Fatalf("click on an unknown field focused %q", got)
			}
		})
		if fields := c.tabOrder; len(fields) > 0 {
			t.Run(c.name+"/field cycling wraps", func(t *testing.T) {
				m := c.open()
				m, _ = model.Update(m, model.FocusFieldMsg{Field: fields[0]})
				for i := 1; i <= len(fields); i++ {
					m, _ = clickPress(m, "tab")
					if got, want := c.field(m), fields[i%len(fields)]; got != want {
						t.Fatalf("tab %d reached %q, want %q", i, got, want)
					}
				}
				for i := len(fields) - 1; i >= 0; i-- {
					m, _ = clickPress(m, "shift+tab")
					if got, want := c.field(m), fields[i]; got != want {
						t.Fatalf("shift+tab to index %d reached %q, want %q", i, got, want)
					}
				}
			})
		}
		if c.save != nil {
			t.Run(c.name+"/save error keeps the input", func(t *testing.T) {
				m := typeKeys(focused(), model.KeyMsg{Text: "keep"})
				mode := m.CurrentMode()
				m, cmds := clickPress(m, c.save...)
				if len(cmds) == 0 {
					t.Fatalf("saving asked the daemon for nothing: %+v", m)
				}
				m, _ = model.Update(m, c.result(m, errors.New("daemon said no")))
				if m.CurrentMode() != mode || c.text(m, c.textField) != "keep" {
					t.Fatalf("after a save error: mode %s text %q", m.CurrentMode(), c.text(m, c.textField))
				}
			})
		}
	}
}

// Every card in the order has a row in the table and declares its fields, so a card added to the
// order without either fails here.
func TestEveryPopupCardIsDescribed(t *testing.T) {
	table := popupCards()
	for _, card := range model.Cards() {
		if !slices.ContainsFunc(table, func(c popupCard) bool { return c.card == card }) {
			t.Errorf("card %d has no row in the shared card table", card)
		}
		if _, declared := model.CardFields(card); !declared {
			t.Errorf("card %d declares no field list; a card without fields declares an empty one", card)
		}
	}
	for _, c := range table {
		if c.card != model.NoCard && !slices.Contains(model.Cards(), c.card) {
			t.Errorf("%s is not in the card order", c.name)
		}
		if specs, _ := model.CardFields(c.card); c.textField != "" && !slices.ContainsFunc(specs, func(f model.FieldSpec) bool { return f.Name == c.textField && f.Text }) {
			t.Errorf("%s: %q is not one of its text fields", c.name, c.textField)
		}
	}
}

func TestPopupCardStates(t *testing.T) {
	for _, c := range popupCards() {
		for _, state := range c.states {
			t.Run(c.name+" "+state.name, func(t *testing.T) {
				m := state.open()
				if got := m.CurrentMode(); got != state.mode {
					t.Fatalf("mode %s, want %s", got, state.mode)
				}
				if got := m.TopCard(); got != c.card {
					t.Fatalf("top card %d, want %d", got, c.card)
				}
				if m, _ = clickPress(m, "ctrl+c"); m.Quitting != (c.quits && c.card != model.NoCard) {
					t.Fatalf("ctrl+c: quitting=%v, want %v", m.Quitting, c.quits)
				}
			})
			// A card lists a global key (quit, help) only if pressing it does what it says.
			t.Run(c.name+" "+state.name+"/hints", func(t *testing.T) {
				if c.card == model.NoCard {
					t.Skip("Search is not a card")
				}
				listed := false
				for binding := range model.BindingsForMode(state.mode, false) {
					if binding.Mode != "" {
						continue
					}
					m, _ := clickPress(state.open(), binding.Chord)
					if !m.Quitting && !m.HelpOpen {
						t.Errorf("%s lists %s (%s), which the card swallows", state.mode, binding.Chord, binding.Help)
					}
					listed = listed || binding.Chord == "ctrl+c"
				}
				if listed != c.quits {
					t.Errorf("%s lists ctrl+c = %v, but it quits = %v", state.mode, listed, c.quits)
				}
			})
		}
	}
}

// Of any two cards open at once, the earlier one in the order is on top and owns the mode.
func TestPopupCardOrder(t *testing.T) {
	// The order is spelled out here, not read from model.Cards().
	var cards []popupCard
	for _, name := range []string{"Create", "Draft editor", "single-field form", "Close", "Dates", "Recurrence", "New Project", "Filter form", "Menu"} {
		i := slices.IndexFunc(popupCards(), func(c popupCard) bool { return c.name == name })
		if i < 0 {
			t.Fatalf("card %q has no row", name)
		}
		cards = append(cards, popupCards()[i])
	}
	for i, top := range cards {
		for _, under := range cards[i+1:] {
			t.Run(top.name+" over "+under.name, func(t *testing.T) {
				m := model.Model{}
				under.graft(&m, under.open())
				top.graft(&m, top.open())
				if m.TopCard() != top.card || m.CurrentMode() != top.states[0].mode {
					t.Fatalf("top card %d mode %s, want %d %s", m.TopCard(), m.CurrentMode(), top.card, top.states[0].mode)
				}
			})
		}
	}
}

func TestDraftCardsTypePrintableKeysIntoTheBody(t *testing.T) {
	for _, c := range popupCards() {
		if c.body == nil {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			m := c.body.focus(c.open())
			for _, key := range []string{"q", "?", "p", "j"} {
				m = typed(m, key)
			}
			if got := c.body.text(m); got != "q?pj" || m.Quitting || m.HelpOpen {
				t.Fatalf("body %q quitting=%v help=%v", got, m.Quitting, m.HelpOpen)
			}
		})
	}
}

func TestDraftCardsShareDiscardConfirmation(t *testing.T) {
	for _, c := range popupCards() {
		if c.body == nil {
			continue
		}
		body := c.body
		t.Run(c.name+"/leaving with nothing typed does not ask", func(t *testing.T) {
			m, _ := clickPress(c.open(), "escape")
			if body.present(m) {
				t.Fatalf("esc on an empty Draft left mode %s", m.CurrentMode())
			}
		})
		t.Run(c.name+"/asking swallows other keys and edits", func(t *testing.T) {
			m, _ := clickPress(typed(asking(c)(), "z"), "ctrl+c", "tab")
			m, _ = model.Update(m, model.EditMsg{Target: body.target, From: 0, To: 0, Text: "e", Cursor: 1, Len: 1})
			if got := body.text(m); got != "x" || m.Quitting || m.CurrentMode() != model.ModeDiscard {
				t.Fatalf("while asking: text %q quitting=%v mode %s", got, m.Quitting, m.CurrentMode())
			}
		})
		for _, keep := range []string{"n", "escape"} {
			t.Run(c.name+"/"+keep+" keeps the text", func(t *testing.T) {
				m, _ := clickPress(asking(c)(), keep)
				if !body.present(m) || m.CurrentMode() == model.ModeDiscard || body.text(m) != "x" {
					t.Fatalf("after %s: present=%v mode %s text %q", keep, body.present(m), m.CurrentMode(), body.text(m))
				}
			})
		}
		t.Run(c.name+"/y discards", func(t *testing.T) {
			m, _ := clickPress(asking(c)(), "y")
			if body.present(m) {
				t.Fatalf("y left mode %s", m.CurrentMode())
			}
		})
	}
}

func TestDraftCardsSaveErrorKeepsTheDraft(t *testing.T) {
	for _, c := range popupCards() {
		if c.body == nil {
			continue
		}
		body := c.body
		t.Run(c.name, func(t *testing.T) {
			m := body.ready(typed(typed(body.focus(c.open()), "k"), "p"))
			m, cmds := clickPress(m, body.save)
			if len(cmds) == 0 {
				t.Fatalf("save sent nothing: mode %s", m.CurrentMode())
			}
			// While saving, nothing edits the Draft.
			before := body.text(m)
			m, _ = model.Update(typed(m, "z"), model.EditMsg{Target: body.target, From: 0, To: 0, Text: "e", Cursor: 1, Len: 1})
			if body.text(m) != before {
				t.Fatalf("saving Draft edited: %q -> %q", before, body.text(m))
			}
			m, _ = model.Update(m, body.fail(m, errors.New("daemon said no")))
			if !body.present(m) || body.text(m) != before {
				t.Fatalf("after a save error: present=%v text %q, want %q", body.present(m), body.text(m), before)
			}
			if body.err(m) == nil {
				t.Fatal("the save error was not kept")
			}
			// The card is editable again, and the next edit clears the error.
			if m = typed(body.focus(m), "!"); body.text(m) != before+"!" {
				t.Fatalf("Draft not editable after the error: %q", body.text(m))
			}
			if err := body.err(m); err != nil {
				t.Fatalf("the next edit left the error %q", err)
			}
		})
	}
}

func TestDraftCardsIgnoreKeysWhileSaving(t *testing.T) {
	for _, c := range popupCards() {
		if c.body == nil {
			continue
		}
		body := c.body
		t.Run(c.name, func(t *testing.T) {
			m := body.ready(typed(body.focus(c.open()), "k"))
			m, cmds := clickPress(m, body.save)
			if len(cmds) == 0 {
				t.Fatalf("save sent nothing: mode %s", m.CurrentMode())
			}
			for _, chord := range []string{body.save, "escape", "tab", "y", "z"} {
				next, cmds := model.Update(m, model.KeyMsg{Chord: chord, Text: chord})
				if len(cmds) != 0 || !reflect.DeepEqual(next, m) {
					t.Fatalf("%s while saving: cmds %v, state changed", chord, cmds)
				}
			}
		})
	}
}

func TestDraftCardsTakeNativeEditsAndUndo(t *testing.T) {
	for _, c := range popupCards() {
		if c.body == nil {
			continue
		}
		body := c.body
		t.Run(c.name, func(t *testing.T) {
			m, _ := model.Update(body.focus(c.open()), model.EditMsg{Target: body.target, Text: "hi", Cursor: 2})
			if got := body.text(m); got != "hi" {
				t.Fatalf("after a native edit: %q", got)
			}
			if m, _ = model.Update(m, model.UndoMsg{Target: body.target}); body.text(m) != "" {
				t.Fatalf("after undo: %q", body.text(m))
			}
			// An edit for the other card's body does not reach this one.
			other := model.EditCreateBody
			if body.target == other {
				other = model.EditDraftBody
			}
			if m, _ = model.Update(m, model.EditMsg{Target: other, Text: "no", Cursor: 2}); body.text(m) != "" {
				t.Fatalf("the other card's edit landed: %q", body.text(m))
			}
		})
	}
}
