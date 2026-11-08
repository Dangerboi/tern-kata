package model

import (
	"slices"
	"strings"
)

type Control int

const (
	ControlSelect Control = iota
	ControlTabs
	ControlSwitch
)

const (
	ModeDropdown       Mode   = "dropdown"
	DropdownNext       Action = "dropdown-next"
	DropdownPrevious   Action = "dropdown-previous"
	DropdownPick       Action = "dropdown-pick"
	DropdownClose      Action = "dropdown-close"
	DropdownEraseQuery Action = "dropdown-erase-query"
	DropdownClearQuery Action = "dropdown-clear-query"
)

const SearchOver = 12

// ChoiceSet is a choice field as the card shows it: the control, the labels in the order ChooseMsg indexes them and the index of
// the chosen one, -1 when none is. A switch is on at OnIndex.
type ChoiceSet struct {
	Control Control
	Labels  []string
	Chosen  int
	OnIndex int
}

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

func newChoices(control Control, labels []string, chosen int) ChoiceSet {
	return ChoiceSet{Control: control, Labels: labels, Chosen: chosen}
}

func segmentedOrSelect(n int) Control {
	if n >= 1 && n <= 3 {
		return ControlTabs
	}
	return ControlSelect
}

// Dropdown is the open list of a select: the card and field it belongs to, the index of the highlighted choice and the search
// text. The pointer and the keys both drive it; a dropdown closes when its field loses focus or its card goes.
type Dropdown struct {
	Card   Card
	Field  Field
	Open   bool
	Active int
	Query  string
	// activeLabel is the label the highlight is on, so a refresh that reorders the choices keeps it on the same one.
	activeLabel string
}

func openedDropdown(card Card, field Field, set ChoiceSet) Dropdown {
	d := Dropdown{Card: card, Field: field, Open: true, Active: max(set.Chosen, 0)}
	d.hold(set)
	return d
}

func (d *Dropdown) hold(set ChoiceSet) {
	d.activeLabel = ""
	if d.Active >= 0 && d.Active < len(set.Labels) {
		d.activeLabel = set.Labels[d.Active]
	}
}

func (d *Dropdown) follow(set ChoiceSet) {
	if d.Active >= 0 && d.Active < len(set.Labels) && set.Labels[d.Active] == d.activeLabel {
		return
	}
	if at := slices.Index(set.Labels, d.activeLabel); at >= 0 {
		d.Active = at
	} else {
		d.Active = min(max(d.Active, 0), max(len(set.Labels)-1, 0))
		d.hold(set)
	}
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

func (m Model) OpenDropdown(card Card, name Field) Dropdown {
	if open := m.Dropdown; open.Open && open.Card == card && open.Field == name {
		return open
	}
	return Dropdown{Card: card, Field: name}
}

func (m Model) settleDropdown() Model {
	if !m.Dropdown.Open {
		return m
	}
	if entry, ok := m.topEntry(); ok && entry.card == m.Dropdown.Card && entry.choices != nil && entry.focus != nil && !entry.busy(m) {
		if field := *entry.focus(&m); field == m.Dropdown.Field {
			if set, ok := entry.choices(m, field); ok && set.Control == ControlSelect {
				m.Dropdown.follow(set)
				return m
			}
		}
	}
	m.Dropdown = Dropdown{}
	return m
}

// SwitchMsg is the pointer flipping the switch called Field of the open popup card. The field takes focus and holds On.
type SwitchMsg struct {
	Field Field
	On    bool
}

func (SwitchMsg) msg() {}

// DropdownToggleMsg is the pointer opening or closing the dropdown of the select called Field of the open popup card.
type DropdownToggleMsg struct {
	Field Field
	Open  bool
}

func (DropdownToggleMsg) msg() {}

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

func toggleDropdown(m Model, name Field, open bool) Model {
	if !open {
		if m.Dropdown.Field == name {
			m.Dropdown = Dropdown{}
		}
		return m
	}
	m = focusField(m, name)
	entry, ok := m.topEntry()
	if !ok || entry.choices == nil || entry.busy(m) || *entry.focus(&m) != name {
		return m
	}
	if set, ok := entry.choices(m, name); ok && set.Control == ControlSelect {
		m.Dropdown = openedDropdown(entry.card, name, set)
	}
	return m
}

func (m Model) topChoices(name Field) (ChoiceSet, bool) {
	if entry, ok := m.topEntry(); ok && entry.choices != nil {
		return entry.choices(m, name)
	}
	return ChoiceSet{}, false
}

func (e cardEntry) openDropdown(m Model) (Model, bool) {
	if e.choices == nil || e.focus == nil {
		return m, false
	}
	field := *e.focus(&m)
	set, ok := e.choices(m, field)
	if !ok || set.Control != ControlSelect {
		return m, false
	}
	m.Dropdown = openedDropdown(e.card, field, set)
	return m, true
}

// dropdownKey is one key for the open dropdown of the focused select, read through the ModeDropdown bindings: up and down move
// the highlight over the choices the search text leaves, enter picks it, escape closes the dropdown, and a search select takes
// typed text, backspace and ctrl+u. Any other key closes the dropdown and reaches the card as it would with none, which is how
// left, right and tab move on. It reports whether the dropdown took the key.
func (e cardEntry) dropdownKey(m Model, msg KeyMsg) (Model, bool) {
	if !m.Dropdown.Open || e.choices == nil || e.focus == nil || e.busy(m) || m.Dropdown.Card != e.card {
		return m, false
	}
	field := *e.focus(&m)
	set, ok := e.choices(m, field)
	if !ok || set.Control != ControlSelect || field != m.Dropdown.Field {
		m.Dropdown = Dropdown{}
		return m, false
	}
	dropdown := &m.Dropdown
	dropdown.follow(set)
	matches := set.Matches(dropdown.Query)
	at := slices.Index(matches, dropdown.Active)
	binding, bound := LookupBindingForMode(msg.Chord, ModeDropdown)
	switch {
	case bound && (binding.Action == DropdownNext || binding.Action == DropdownPrevious):
		if len(matches) > 0 {
			step := 1
			if binding.Action == DropdownPrevious {
				step = -1
			}
			dropdown.Active = matches[((max(at, 0)+step)%len(matches)+len(matches))%len(matches)]
			dropdown.hold(set)
		}
	case bound && binding.Action == DropdownPick:
		if at >= 0 && e.choose != nil {
			m = e.choose(m, dropdown.Active)
			if e.clearErr != nil {
				m = e.clearErr(m)
			}
		}
		m.Dropdown = Dropdown{}
	case bound && binding.Action == DropdownClose:
		m.Dropdown = Dropdown{}
	case bound && binding.Action == DropdownEraseQuery && set.Search():
		runes := []rune(dropdown.Query)
		dropdown.Query = string(runes[:max(len(runes)-1, 0)])
		dropdown.refilter(set)
	case bound && binding.Action == DropdownClearQuery && set.Search():
		dropdown.Query = ""
		dropdown.refilter(set)
	case !bound && set.Search() && typedText(msg) != "":
		dropdown.Query += typedText(msg)
		dropdown.refilter(set)
	default:
		m.Dropdown = Dropdown{}
		return m, false
	}
	return m, true
}

func (d *Dropdown) refilter(set ChoiceSet) {
	if matches := set.Matches(d.Query); len(matches) > 0 {
		d.Active = matches[0]
		d.hold(set)
	}
}
