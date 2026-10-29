package model

import (
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	ModePicker     Mode   = "picker"
	OpenPicker     Action = "open-picker"
	OpenProjects   Action = "open-projects"
	OpenInbox      Action = "open-inbox"
	ClosePicker    Action = "close-picker"
	PickCollection Action = "pick-collection"
)

// MenuLevel is which list of the Menu shows: the top level, the Views level or the Projects level.
type MenuLevel int

const (
	MenuTop MenuLevel = iota
	MenuViews
	MenuProjects
)

// PickerState is the Menu popup, a layer over the main columns. Cursor indexes
// PickerEntries; Level says which level the popup shows.
type PickerState struct {
	Open   bool
	Filter string
	Caret  Caret
	Cursor int
	Level  MenuLevel
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
	ViewsKey      = "views"
	ViewsTitle    = "Views"
	BackKey       = "back"
	BackTitle     = "Back"
)

// PickerEntry is a row of the Menu: a Collection, the Views or Projects entry that opens
// that level, New Project, a Lettered Page (Page names it), or Back, which leaves a level.
type PickerEntry struct {
	Collection
	NewProject, Projects, Views, Back, Lettered, Restore bool
	Page                                                 Page
}

// Key names the row, as Collection.Key names a Collection's.
func (e PickerEntry) Key() string {
	switch {
	case e.Lettered:
		return "page." + pageDefs[e.Page].id
	case e.Restore:
		return RestoreKey
	case e.NewProject:
		return NewProjectKey
	case e.Projects:
		return ProjectsKey
	case e.Views:
		return ViewsKey
	case e.Back:
		return BackKey
	}
	return e.Collection.Key()
}

// IsCollection reports whether the row is a Collection, not a level, a Page or an action.
func (e PickerEntry) IsCollection() bool {
	return !e.NewProject && !e.Projects && !e.Views && !e.Back && !e.Lettered && !e.Restore
}

// menuEntries are the Menu's rows. The top level is the bound Project pinned first,
// then Views, Projects, the Lettered Pages, Restore and New Project; the Views level
// is Back and the fixed Collections; the Projects level is Back and every Project.
// A filter ignores the level and lists every matching title flat (any case).
func (m Model) menuEntries(level MenuLevel, filter string) []PickerEntry {
	if len(m.Collections) == 0 {
		return nil
	}
	filter = strings.ToLower(filter)
	match := func(title string) bool { return strings.Contains(strings.ToLower(title), filter) }
	entries := make([]PickerEntry, 0, len(m.Collections)+8)
	if level != MenuTop && filter == "" {
		entries = append(entries, PickerEntry{Collection: Collection{Title: BackTitle}, Back: true})
		for _, c := range m.Collections {
			if c.IsProject() == (level == MenuProjects) {
				entries = append(entries, PickerEntry{Collection: c})
			}
		}
		return entries
	}
	pinned := func(c Collection) bool { return m.Bound && c.IsProject() && c.ProjectUID == m.Project.UID }
	// At the top only the bound Project is a row of its own; a filter finds every Collection.
	for _, wantPinned := range []bool{true, false} {
		for _, c := range m.Collections {
			if pinned(c) == wantPinned && (wantPinned || filter != "") && match(c.Title) {
				entries = append(entries, PickerEntry{Collection: c})
			}
		}
	}
	if match(ViewsTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: ViewsTitle}, Views: true})
	}
	if match(ProjectsTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: ProjectsTitle}, Projects: true})
	}
	for _, def := range pageDefs {
		if def.lettered && match(def.label) {
			entries = append(entries, PickerEntry{Collection: Collection{Title: def.label}, Lettered: true, Page: def.page})
		}
	}
	if match(RestoreTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: RestoreTitle}, Restore: true})
	}
	if match(NewProjectTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: NewProjectTitle}, NewProject: true})
	}
	return entries
}

// PickerEntries are the popup's rows.
func (m Model) PickerEntries() []PickerEntry {
	return m.menuEntries(m.Picker.Level, m.Picker.Filter)
}

// SidebarEntries are the sidebar's rows: the Menu's current level, unfiltered.
func (m Model) SidebarEntries() []PickerEntry { return m.menuEntries(m.SidebarLevel, "") }

// CurrentView is the title of the fixed Collection that is current, or "" while a Project is.
func (m Model) CurrentView() string {
	if m.Collection.IsProject() {
		return ""
	}
	return m.Collection.Title
}

func entryIndex(entries []PickerEntry, key string) int {
	return max(0, slices.IndexFunc(entries, func(e PickerEntry) bool { return e.Key() == key }))
}

// syncSidebarCursor puts the sidebar cursor on the current Collection when the level shows it.
func (m Model) syncSidebarCursor() Model {
	for i, entry := range m.SidebarEntries() {
		if entry.IsCollection() && entry.Collection == m.Collection {
			m.CollectionCursor = i
		}
	}
	return m
}

// openPicker opens the picker over the columns, at the top level with the cursor on Views
// while a fixed Collection is current; other layers keep the keys.
func openPicker(m Model) Model {
	if slices.Contains(pageModes, m.CurrentMode()) {
		m.Picker = PickerState{Open: true}
		if m.CurrentView() != "" {
			m.Picker.Cursor = entryIndex(m.PickerEntries(), ViewsKey)
		}
	}
	return m
}

// openProjects is kata's P: the Menu's Projects level, in the sidebar while it shows, else in the popup.
func openProjects(m Model) Model {
	if m.SidebarShown() {
		m.Picker, m.Focus = PickerState{}, ColumnCollections
		return setMenuLevel(m, false, MenuProjects)
	}
	if m = openPicker(m); m.Picker.Open {
		m = setMenuLevel(m, true, MenuProjects)
	}
	return m
}

// setMenuLevel moves the popup (when popup) or the sidebar to a level. The cursor lands on the
// row that led there when returning to the top (Views or Projects), and in the Views or Projects
// list on the current Collection of that kind or else Back.
func setMenuLevel(m Model, popup bool, level MenuLevel) Model {
	from := m.SidebarLevel
	if popup {
		from = m.Picker.Level
	}
	key := BackKey
	switch {
	case level == MenuTop && from == MenuViews:
		key = ViewsKey
	case level == MenuTop:
		key = ProjectsKey
	case m.Collection.IsProject() == (level == MenuProjects):
		key = m.Collection.Key()
	}
	if popup {
		m.Picker.Level, m.Picker.Filter = level, ""
		m.Picker.Cursor = entryIndex(m.PickerEntries(), key)
	} else {
		m.SidebarLevel = level
		m.CollectionCursor = entryIndex(m.SidebarEntries(), key)
	}
	return m
}

// leaveSidebarLevel returns the sidebar to the Views level, where it rests, once an entry was opened.
func leaveSidebarLevel(m Model) Model {
	if m.SidebarLevel != MenuViews {
		m = setMenuLevel(m, false, MenuViews)
	}
	return m
}

// openEntry is enter or a click on a Menu row in the popup or the sidebar. A Collection becomes
// current and takes the list's keys; Views, Projects and Back change the level; New Project opens its
// form, leaving the Menu behind it.
func openEntry(m Model, entry PickerEntry, popup bool) (Model, []Cmd) {
	switch {
	case entry.Lettered:
		m.Picker, m.Focus = PickerState{}, ColumnList
		m = leaveSidebarLevel(m)
		return showPage(entry.Page)(m)
	case entry.Restore:
		m.Picker, m.Focus = PickerState{}, ColumnList
		m = leaveSidebarLevel(m)
		return openRestore(m), nil
	case entry.NewProject:
		return openProjectForm(m), nil
	case entry.Views:
		return setMenuLevel(m, popup, MenuViews), nil
	case entry.Projects:
		return setMenuLevel(m, popup, MenuProjects), nil
	case entry.Back:
		return setMenuLevel(m, popup, MenuTop), nil
	}
	m.Picker, m.Focus = PickerState{}, ColumnList
	m = leaveSidebarLevel(m)
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
		if text, edited := editText(m.Picker.Filter, &m.Picker.Caret, "", false, msg); edited {
			m.Picker.Filter, m.Picker.Cursor = text, 0
		}
		return m, nil
	}
	last := len(m.PickerEntries()) - 1
	switch binding.Action {
	case ClosePicker:
		if m.Picker.Level != MenuTop && m.Picker.Filter == "" {
			return setMenuLevel(m, true, MenuTop), nil
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
	}
	return m, nil
}
