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

const searchOver = 12

// ChoiceSet is a choice field as the card shows it: the control, the labels in the order ChooseMsg indexes them and the index of
// the chosen one, -1 when none is. A switch's labels are No, then Yes.
type ChoiceSet struct {
	Control Control
	Labels  []string
	Chosen  int
}

func (s ChoiceSet) Search() bool { return s.Control == ControlSelect && len(s.Labels) > searchOver }

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

// On is whether a switch is on.
func (s ChoiceSet) On() bool { return s.Chosen == boolIndex(true) }

// SwitchLabels is the label of a switch turned off and turned on.
func (s ChoiceSet) SwitchLabels() [2]string { return [2]string{s.Labels[0], s.Labels[1]} }

// boolIndex is the index of a choice of "no" (0) and "yes" (1).
func boolIndex(yes bool) int {
	if yes {
		return 1
	}
	return 0
}

// indexBool is the "yes" a choice index of a "no" or "yes" choice stands for, false when it is neither.
func indexBool(index int) (yes, ok bool) {
	return index == boolIndex(true), index == boolIndex(false) || index == boolIndex(true)
}

// yesNoSwitch is a switch of "No" and "Yes" that is on at "Yes".
func yesNoSwitch(yes bool) ChoiceSet {
	return ChoiceSet{Control: ControlSwitch, Labels: []string{"No", "Yes"}, Chosen: boolIndex(yes)}
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
	// Keyed is whether a key has moved the highlight or the search text since the dropdown opened: until then the Pane leaves
	// the highlight of a menu the pointer opened to Tern (the view draws Active only for a drawn-open or keyed menu).
	Keyed bool
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
	if entry, ok := m.topEntry(); ok && entry.card == m.Dropdown.Card && entry.focus != nil && !entry.busy(m) {
		if field := *entry.focus(&m); field == m.Dropdown.Field {
			if set, ok := entry.selectSet(m, field); ok {
				m.Dropdown.follow(set)
				return m
			}
		}
	}
	m.Dropdown = Dropdown{}
	return m
}

// selectRef names one select: the card and the field it is the control of.
type selectRef struct {
	Card  Card
	Field Field
}

// SelectShown is whether the Pane draws the select of field name in card open.
func (m Model) SelectShown(card Card, name Field) bool {
	return m.shownSelect == selectRef{card, name}
}

// SelectDrawn is whether the select of field name in card is drawn: card is the card on top, which does not hide name, and
// name is a select of it (CardChoices).
func (m Model) SelectDrawn(card Card, name Field) bool {
	entry, ok := m.topEntry()
	if !ok || entry.card != card || (entry.hidden != nil && entry.hidden(m, name)) {
		return false
	}
	_, ok = entry.selectSet(m, name)
	return ok
}

// CloseSelectMenu closes the menu of the select of Field in Card in Tern, where the pointer opened it and the Pane closed it
// (showSelect). The Pane never drew that menu open, so drawing it closed is no change Tern would apply; the driver sends the
// change as ops on the select instead.
type CloseSelectMenu selectRef

func (CloseSelectMenu) cmd() {}

// showSelect decides which select the Pane draws open after a message. pointer is whether the message was a selectPointerMsg and
// prev is the model before it. Tern opens and closes a select's menu at the pointer by itself and lets a change of the drawn
// open state win over it, so a drawing that answers an earlier click and
// reaches Tern after a later one would undo that click: open the menu again after a pick, or close it under the next click.
// The Pane therefore draws open only a menu the keys opened. A menu the pointer opened stays drawn closed while it is open (Tern
// shows it), and so does one the pointer closed or picked from. When the Pane itself closes a menu the pointer opened (a key
// picks, escape, tab, a field the keys leave), it asks for CloseSelectMenu.
func (m Model) showSelect(prev Model, pointer bool, cmds []Cmd) (Model, []Cmd) {
	open, was := m.openSelect(), prev.openSelect()
	switch {
	case open != (selectRef{}) && open == was:
	case open != (selectRef{}) && !pointer:
		m.shownSelect = open
	case open != (selectRef{}):
		m.shownSelect = selectRef{}
	case m.shownSelect != (selectRef{}):
		m.shownSelect = selectRef{}
	case was != (selectRef{}) && !pointer:
		cmds = append(cmds, CloseSelectMenu(was))
	}
	return m, cmds
}

// selectPointerMsg is whether msg is the pointer acting on a popup card, so Tern already shows what it did: opening or closing
// a select's menu, picking from it, flipping a switch, pressing a button or focusing another field (a click elsewhere closes
// Tern's menu itself).
func selectPointerMsg(msg Msg) bool {
	switch msg.(type) {
	case DropdownToggleMsg, ChooseMsg, SwitchMsg, FocusFieldMsg, ButtonPressMsg:
		return true
	}
	return false
}

func (m Model) openSelect() selectRef {
	if !m.Dropdown.Open {
		return selectRef{}
	}
	return selectRef{m.Dropdown.Card, m.Dropdown.Field}
}

// SwitchMsg is the pointer flipping the switch called Field of the open popup card. A card with a flip hook asks for its write;
// on any other the field takes focus and holds On.
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

// switchField flips the switch called name of the open popup card. A card whose switch asks for a write (flip) takes the
// flip itself; any other card holds the index like a choice.
func switchField(m Model, name Field, on bool) (Model, []Cmd) {
	entry, ok := m.topEntry()
	if !ok || entry.choices == nil {
		return m, nil
	}
	set, ok := entry.choices(m, name)
	if !ok || set.Control != ControlSwitch {
		return m, nil
	}
	if entry.flip != nil {
		return entry.flip(m, on)
	}
	return chooseField(m, name, boolIndex(on)), nil
}

func toggleDropdown(m Model, name Field, open bool) Model {
	if !open {
		if m.Dropdown.Field == name {
			m.Dropdown = Dropdown{}
		}
		return m
	}
	m = focusField(m, name)
	if entry, ok := m.topEntry(); ok && entry.focus != nil && !entry.busy(m) && *entry.focus(&m) == name {
		m, _ = entry.openDropdown(m, name)
	}
	return m
}

// selectSet is the choice set of the field called name when the card draws it as a select.
func (e cardEntry) selectSet(m Model, name Field) (ChoiceSet, bool) {
	if e.choices == nil {
		return ChoiceSet{}, false
	}
	set, ok := e.choices(m, name)
	return set, ok && set.Control == ControlSelect
}

// openDropdown opens the dropdown of the select called name. One already open on that field stays as it is, so the typed
// search text and the highlight survive a second open.
func (e cardEntry) openDropdown(m Model, name Field) (Model, bool) {
	set, ok := e.selectSet(m, name)
	if !ok {
		return m, false
	}
	if open := m.Dropdown; !open.Open || open.Card != e.card || open.Field != name {
		m.Dropdown = openedDropdown(e.card, name, set)
	}
	return m, true
}

// pick gives the focused field its choice at index, closes the dropdown and dismisses the card's error, whether the pointer or
// a key picked.
func (e cardEntry) pick(m Model, index int) Model {
	m.Dropdown = Dropdown{}
	if e.choose != nil {
		m = e.choose(m, index)
	}
	if e.clearErr != nil {
		m = e.clearErr(m)
	}
	return m
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
	set, ok := e.selectSet(m, field)
	if !ok || field != m.Dropdown.Field {
		m.Dropdown = Dropdown{}
		return m, false
	}
	dropdown := &m.Dropdown
	dropdown.follow(set)
	matches := set.Matches(dropdown.Query)
	at := slices.Index(matches, dropdown.Active)
	binding, bound := LookupBindingForMode(msg.Chord, ModeDropdown)
	dropdown.Keyed = true // a key that closes the dropdown resets it below
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
		if at >= 0 {
			m = e.pick(m, dropdown.Active)
		} else {
			m.Dropdown = Dropdown{}
		}
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
