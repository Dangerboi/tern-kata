package model

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// Control is the kind of control a popup card draws a choice field with.
type Control int

const (
	// ControlSelect is a menu: a field that opens a list of the choices, with search past SearchOver of them.
	ControlSelect Control = iota
	// ControlTabs is a segmented strip of two or three choices.
	ControlTabs
	// ControlSwitch is a yes or no setting.
	ControlSwitch
)

// SearchOver is how many choices a select holds before it takes a search line.
const SearchOver = 12

// ChoiceSet is a choice field as the card shows it: the control, the labels in the order ChooseMsg indexes them and the index of
// the chosen one, -1 when none is. A switch is on at OnIndex.
type ChoiceSet struct {
	Control Control
	Labels  []string
	Chosen  int
	OnIndex int
}

// Search says the select takes a search line.
func (s ChoiceSet) Search() bool { return s.Control == ControlSelect && len(s.Labels) > SearchOver }

// Matches is the indexes of the labels that hold query, in order, whatever the case. Every index matches an empty query.
func (s ChoiceSet) Matches(query string) []int {
	query = strings.ToLower(query)
	var found []int
	for i, label := range s.Labels {
		if strings.Contains(strings.ToLower(label), query) {
			found = append(found, i)
		}
	}
	return found
}

// newChoices is a ChoiceSet drawn by control; chosen is the index of the held value.
func newChoices(control Control, labels []string, chosen int) ChoiceSet {
	return ChoiceSet{Control: control, Labels: labels, Chosen: chosen}
}

// segmentedOrSelect is the control a field the card does not name gets: tabs for up to three choices, a select for more.
func segmentedOrSelect(n int) Control {
	if n >= 1 && n <= 3 {
		return ControlTabs
	}
	return ControlSelect
}

// ChoiceMenu is the open menu of a select: the card and field it belongs to, the index of the highlighted choice and the search
// text. The pointer and the keys both drive it; a menu closes when its field loses focus or its card goes.
type ChoiceMenu struct {
	Card   Card
	Field  Field
	Open   bool
	Active int
	Query  string
}

// CardChoices is the choice set of the field called name of card, false for a field that is not a choice or one the card keeps
// as a row of its own words.
func (m Model) CardChoices(card Card, name Field) (ChoiceSet, bool) {
	for _, entry := range cards {
		if entry.card == card && entry.choices != nil {
			return entry.choices(m, name)
		}
	}
	return ChoiceSet{}, false
}

// OpenMenu is the open menu of the select called name of card; its zero value, closed, for any other field.
func (m Model) OpenMenu(card Card, name Field) ChoiceMenu {
	if menu := m.Menu; menu.Open && menu.Card == card && menu.Field == name {
		return menu
	}
	return ChoiceMenu{Card: card, Field: name}
}

// settleMenu closes a menu whose field is no longer the focused select of the card on top.
func (m Model) settleMenu() Model {
	if !m.Menu.Open {
		return m
	}
	if entry, ok := m.topEntry(); ok && entry.card == m.Menu.Card && entry.choices != nil && entry.focus != nil && !entry.busy(m) {
		if field := *entry.focus(&m); field == m.Menu.Field {
			if set, ok := entry.choices(m, field); ok && set.Control == ControlSelect {
				return m
			}
		}
	}
	m.Menu = ChoiceMenu{}
	return m
}

// SwitchMsg is the pointer flipping the switch called Field of the open popup card. The field takes focus and holds On.
type SwitchMsg struct {
	Field Field
	On    bool
}

func (SwitchMsg) msg() {}

// MenuToggleMsg is the pointer opening or closing the menu of the select called Field of the open popup card.
type MenuToggleMsg struct {
	Field Field
	Open  bool
}

func (MenuToggleMsg) msg() {}

// switchField flips the switch field called name to on.
func switchField(m Model, name Field, on bool) Model {
	set, ok := m.topChoices(name)
	if !ok || set.Control != ControlSwitch {
		return m
	}
	index := set.OnIndex
	if !on {
		index = 1 - set.OnIndex
	}
	return chooseField(m, name, index)
}

// toggleMenu opens or closes the menu of the select called name; opening focuses the field.
func toggleMenu(m Model, name Field, open bool) Model {
	if !open {
		if m.Menu.Field == name {
			m.Menu = ChoiceMenu{}
		}
		return m
	}
	m = focusField(m, name)
	entry, ok := m.topEntry()
	if !ok || entry.choices == nil || entry.busy(m) || *entry.focus(&m) != name {
		return m
	}
	if set, ok := entry.choices(m, name); ok && set.Control == ControlSelect {
		m.Menu = ChoiceMenu{Card: entry.card, Field: name, Open: true, Active: max(set.Chosen, 0)}
	}
	return m
}

// topChoices is the choice set of the field called name of the card on top.
func (m Model) topChoices(name Field) (ChoiceSet, bool) {
	if entry, ok := m.topEntry(); ok && entry.choices != nil {
		return entry.choices(m, name)
	}
	return ChoiceSet{}, false
}

// openChoiceMenu opens the menu of the focused field when it is a select; it reports whether it did.
func (e cardEntry) openChoiceMenu(m Model) (Model, bool) {
	if e.choices == nil || e.focus == nil {
		return m, false
	}
	field := *e.focus(&m)
	set, ok := e.choices(m, field)
	if !ok || set.Control != ControlSelect {
		return m, false
	}
	m.Menu = ChoiceMenu{Card: e.card, Field: field, Open: true, Active: max(set.Chosen, 0)}
	return m, true
}

// menuKey is one key for the open menu of the focused select: up and down move the highlight over the choices the search
// text leaves, enter picks it, escape closes the menu, and a search select takes typed text and backspace. Any other key
// closes the menu and reaches the card as it would with no menu, which is how left, right and tab move on. It reports whether
// the menu took the key.
func (e cardEntry) menuKey(m Model, msg KeyMsg) (Model, bool) {
	if !m.Menu.Open || e.choices == nil || e.focus == nil || e.busy(m) || m.Menu.Card != e.card {
		return m, false
	}
	field := *e.focus(&m)
	set, ok := e.choices(m, field)
	if !ok || set.Control != ControlSelect || field != m.Menu.Field {
		m.Menu = ChoiceMenu{}
		return m, false
	}
	menu := &m.Menu
	matches := set.Matches(menu.Query)
	at := slices.Index(matches, menu.Active)
	switch {
	case msg.Chord == "down" || msg.Chord == "up":
		if len(matches) > 0 {
			step := 1
			if msg.Chord == "up" {
				step = -1
			}
			menu.Active = matches[((max(at, 0)+step)%len(matches)+len(matches))%len(matches)]
		}
	case msg.Chord == "enter":
		if at >= 0 && e.choose != nil {
			m = e.choose(m, menu.Active)
			if e.clearErr != nil {
				m = e.clearErr(m)
			}
		}
		m.Menu = ChoiceMenu{}
	case msg.Chord == "escape":
		m.Menu = ChoiceMenu{}
	case set.Search() && msg.Chord == "backspace":
		runes := []rune(menu.Query)
		menu.Query = string(runes[:max(len(runes)-1, 0)])
		menu.refilter(set)
	case set.Search() && msg.Chord == "ctrl+u":
		menu.Query = ""
		menu.refilter(set)
	case set.Search() && menuText(msg) != "":
		menu.Query += menuText(msg)
		menu.refilter(set)
	default:
		m.Menu = ChoiceMenu{}
		return m, false
	}
	return m, true
}

// refilter puts the highlight on the first choice the search text leaves.
func (menu *ChoiceMenu) refilter(set ChoiceSet) {
	if matches := set.Matches(menu.Query); len(matches) > 0 {
		menu.Active = matches[0]
	}
}

// menuText is the text a key types into a select's search line: a bare character or a space.
func menuText(msg KeyMsg) string {
	switch {
	case msg.Chord == "space":
		return " "
	case msg.Text != "" && (msg.Chord == "paste" || utf8.RuneCountInString(msg.Chord) == 1):
		return singleLineText(msg.Text)
	case utf8.RuneCountInString(msg.Chord) == 1:
		return msg.Chord
	}
	return ""
}
