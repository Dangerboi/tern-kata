package model

import (
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	ModePicker     Mode   = "picker"
	OpenPicker     Action = "open-picker"
	ClosePicker    Action = "close-picker"
	PickCollection Action = "pick-collection"
)

// PickerState is the Menu popup, a layer over the main columns. Cursor indexes
// PickerEntries; Projects says the popup shows the Projects level.
type PickerState struct {
	Open     bool
	Filter   string
	Cursor   int
	Projects bool
}

// PickerSelected is a click on a picker row; Key is PickerEntry.Key.
type PickerSelected struct{ Key string }

func (PickerSelected) msg() {}

// Key names a Collection in rendered rows, matching Tern's node ids.
func (c Collection) Key() string {
	if c.IsProject() {
		return "project." + c.ProjectUID
	}
	return c.View
}

const (
	ProjectsKey   = "projects"
	ProjectsTitle = "Projects"
	BackKey       = "back"
	BackTitle     = "Back"
)

// PickerEntry is a row of the Menu: a Collection, the Projects entry that opens
// the Projects level, New Project, or Back, which leaves the Projects level.
type PickerEntry struct {
	Collection
	NewProject, Projects, Back bool
}

// Key names the row, as Collection.Key names a Collection's.
func (e PickerEntry) Key() string {
	switch {
	case e.NewProject:
		return NewProjectKey
	case e.Projects:
		return ProjectsKey
	case e.Back:
		return BackKey
	}
	return e.Collection.Key()
}

// menuEntries are the Menu's rows. The top level is the fixed Collections with
// the bound Project pinned first, then Projects and New Project; the Projects
// level is Back and every Project. A filter ignores the level and narrows both
// to titles containing it (any case).
func (m Model) menuEntries(projects bool, filter string) []PickerEntry {
	if len(m.Collections) == 0 {
		return nil
	}
	filter = strings.ToLower(filter)
	match := func(title string) bool { return strings.Contains(strings.ToLower(title), filter) }
	entries := make([]PickerEntry, 0, len(m.Collections)+3)
	if projects && filter == "" {
		entries = append(entries, PickerEntry{Collection: Collection{Title: BackTitle}, Back: true})
		for _, c := range m.Collections {
			if c.IsProject() {
				entries = append(entries, PickerEntry{Collection: c})
			}
		}
		return entries
	}
	pinned := func(c Collection) bool { return m.Bound && c.IsProject() && c.ProjectUID == m.Project.UID }
	shown := func(c Collection) bool { return filter != "" || !c.IsProject() || pinned(c) }
	for _, wantPinned := range []bool{true, false} {
		for _, c := range m.Collections {
			if pinned(c) == wantPinned && shown(c) && match(c.Title) {
				entries = append(entries, PickerEntry{Collection: c})
			}
		}
	}
	if match(ProjectsTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: ProjectsTitle}, Projects: true})
	}
	if match(NewProjectTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: NewProjectTitle}, NewProject: true})
	}
	return entries
}

// PickerEntries are the popup's rows.
func (m Model) PickerEntries() []PickerEntry {
	return m.menuEntries(m.Picker.Projects, m.Picker.Filter)
}

// SidebarEntries are the sidebar's rows: the Menu's current level, unfiltered.
func (m Model) SidebarEntries() []PickerEntry { return m.menuEntries(m.SidebarProjects, "") }

func entryIndex(entries []PickerEntry, key string) int {
	return max(0, slices.IndexFunc(entries, func(e PickerEntry) bool { return e.Key() == key }))
}

// syncSidebarCursor puts the sidebar cursor on the current Collection when the level shows it.
func (m Model) syncSidebarCursor() Model {
	for i, entry := range m.SidebarEntries() {
		if entry.Collection == m.Collection && !entry.Projects && !entry.Back && !entry.NewProject {
			m.CollectionCursor = i
		}
	}
	return m
}

// openPicker opens the picker over the columns, at the top level; other layers keep the keys.
func openPicker(m Model) Model {
	if mode := m.CurrentMode(); mode == ModeList || mode == ModeDetail || mode == ModeCollections {
		m.Picker = PickerState{Open: true}
	}
	return m
}

// setMenuLevel moves the popup (when popup) or the sidebar to the Projects level or back to the top,
// with the cursor on Projects at the top, and in the Projects list on the current Project or else Back.
func setMenuLevel(m Model, popup, projects bool) Model {
	key := ProjectsKey
	if projects {
		key = BackKey
		if m.Collection.IsProject() {
			key = m.Collection.Key()
		}
	}
	if popup {
		m.Picker.Projects, m.Picker.Filter = projects, ""
		m.Picker.Cursor = entryIndex(m.PickerEntries(), key)
	} else {
		m.SidebarProjects = projects
		m.CollectionCursor = entryIndex(m.SidebarEntries(), key)
	}
	return m
}

// openEntry is enter or a click on a Menu row in the popup or the sidebar. A Collection becomes
// current and takes the list's keys; Projects and Back change the level; New Project opens its
// form, leaving the Menu behind it.
func openEntry(m Model, entry PickerEntry, popup bool) (Model, []Cmd) {
	switch {
	case entry.NewProject:
		return openProjectForm(m), nil
	case entry.Projects:
		return setMenuLevel(m, popup, true), nil
	case entry.Back:
		return setMenuLevel(m, popup, false), nil
	}
	m.Picker, m.Focus = PickerState{}, ColumnList
	if !popup && m.SidebarProjects {
		m = setMenuLevel(m, false, false)
	}
	return updateCollection(m, CollectionSelected{Collection: entry.Collection})
}

// pickCollection opens the entry under the popup's cursor.
func pickCollection(m Model) (Model, []Cmd) {
	entries := m.PickerEntries()
	if len(entries) == 0 {
		return m, nil
	}
	return openEntry(m, entries[min(m.Picker.Cursor, len(entries)-1)], true)
}

func pickCollectionKey(m Model, key string) (Model, []Cmd) {
	if m.Picker.Open {
		for i, entry := range m.PickerEntries() {
			if entry.Key() == key {
				m.Picker.Cursor = i
				return pickCollection(m)
			}
		}
	}
	return m, nil
}

func updatePicker(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, ok := LookupBindingForMode(msg.Chord, ModePicker)
	// Once the filter has text, single characters (j, k) belong to it.
	if ok && m.Picker.Filter != "" && utf8.RuneCountInString(binding.Chord) == 1 {
		ok = false
	}
	if !ok {
		return typePickerFilter(m, msg), nil
	}
	last := len(m.PickerEntries()) - 1
	switch binding.Action {
	case ClosePicker:
		if m.Picker.Projects && m.Picker.Filter == "" {
			return setMenuLevel(m, true, false), nil
		}
		m.Picker = PickerState{}
	case PickCollection:
		return pickCollection(m)
	case MoveDown:
		m.Picker.Cursor = max(0, min(m.Picker.Cursor+1, last))
	case MoveUp:
		m.Picker.Cursor = max(0, m.Picker.Cursor-1)
	case MoveFirst:
		m.Picker.Cursor = 0
	case MoveLast:
		m.Picker.Cursor = max(0, last)
	case DeleteInput:
		m.Picker.Filter, m.Picker.Cursor = deleteLastRune(m.Picker.Filter), 0
	case ClearInput:
		m.Picker.Filter, m.Picker.Cursor = "", 0
	case QuitAction:
		return updateNavigation(m, QuitAction)
	}
	return m, nil
}

func typePickerFilter(m Model, msg KeyMsg) Model {
	if text := typedText(msg); text != "" {
		m.Picker.Filter += text
		m.Picker.Cursor = 0
	}
	return m
}
