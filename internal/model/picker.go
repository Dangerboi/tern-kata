package model

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	ModePicker Mode   = "picker"
	OpenPicker Action = "open-picker"
	// ShowMenu is the key that opens the Menu from the list; a click on the top bar's Menu button is OpenPicker.
	ShowMenu       Action = "show-menu"
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

// EntryKind is what a Menu row is.
type EntryKind int

const (
	EntryCollection EntryKind = iota // a fixed Collection or a Project
	EntryNewProject
	EntryProjects // opens the Projects level
	EntryViews    // opens the Views level
	EntryBack     // leaves a level
	EntryLettered // a Lettered Page, named by PickerEntry.Page
	EntryRestore
	EntryNewView   // opens the View card
	EntrySavedView // a saved view: Title is its name and Index its place in SavedViews
)

// PickerEntry is a row of the Menu: a Collection, a saved view, the Views or Projects entry that opens
// that level, New Project, New view, a Lettered Page (Page names it), or Back, which leaves a level.
type PickerEntry struct {
	Collection
	Kind  EntryKind
	Index int
	Page  Page
}

// Key names the row, as Collection.Key names a Collection's.
func (e PickerEntry) Key() string {
	switch e.Kind {
	case EntryLettered:
		return "page." + pageDefs[e.Page].id
	case EntryRestore:
		return RestoreKey
	case EntryNewView:
		return NewViewKey
	case EntrySavedView:
		return viewKeyPrefix + strconv.Itoa(e.Index)
	case EntryNewProject:
		return NewProjectKey
	case EntryProjects:
		return ProjectsKey
	case EntryViews:
		return ViewsKey
	case EntryBack:
		return BackKey
	}
	return e.Collection.Key()
}

// IsCollection reports whether the row is a Collection, not a level, a saved view, a Page or an action.
func (e PickerEntry) IsCollection() bool { return e.Kind == EntryCollection }

// menuEntries are the Menu's rows. The top level is the bound Project pinned first,
// then Views, Projects, the Lettered Pages, Restore and New Project; the Views level
// is Back, the fixed Collections, the saved views by name and New view; the Projects level is Back and every Project.
// A filter ignores the level and lists every matching title flat (any case).
func (m Model) menuEntries(level MenuLevel, filter string) []PickerEntry {
	if len(m.Collections) == 0 {
		return nil
	}
	filter = strings.ToLower(filter)
	match := func(title string) bool { return strings.Contains(strings.ToLower(title), filter) }
	entries := make([]PickerEntry, 0, len(m.Collections)+8)
	if level != MenuTop && filter == "" {
		entries = append(entries, PickerEntry{Collection: Collection{Title: BackTitle}, Kind: EntryBack})
		for _, c := range m.Collections {
			if c.IsProject() == (level == MenuProjects) {
				entries = append(entries, PickerEntry{Collection: c})
			}
		}
		if level == MenuViews {
			entries = append(entries, m.savedViewEntries(func(string) bool { return true })...)
			entries = append(entries, PickerEntry{Collection: Collection{Title: NewViewTitle}, Kind: EntryNewView})
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
	if filter != "" {
		entries = append(entries, m.savedViewEntries(match)...)
	}
	if match(ViewsTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: ViewsTitle}, Kind: EntryViews})
	}
	if match(ProjectsTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: ProjectsTitle}, Kind: EntryProjects})
	}
	for _, def := range pageDefs {
		if def.lettered && match(def.label) {
			entries = append(entries, PickerEntry{Collection: Collection{Title: def.label}, Kind: EntryLettered, Page: def.page})
		}
	}
	if match(RestoreTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: RestoreTitle}, Kind: EntryRestore})
	}
	if match(NewProjectTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: NewProjectTitle}, Kind: EntryNewProject})
	}
	if filter != "" && match(NewViewTitle) {
		entries = append(entries, PickerEntry{Collection: Collection{Title: NewViewTitle}, Kind: EntryNewView})
	}
	return entries
}

// savedViewEntries are the rows of the saved views whose name match accepts, by name.
func (m Model) savedViewEntries(match func(string) bool) []PickerEntry {
	var entries []PickerEntry
	for i, v := range m.SavedViews() {
		if match(v.Name) {
			entries = append(entries, PickerEntry{Collection: Collection{Title: v.Name}, Kind: EntrySavedView, Index: i})
		}
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
func openProjects(m Model) (Model, []Cmd) {
	if m.SidebarShown() {
		m.Picker, m.Focus = PickerState{}, ColumnCollections
		return setMenuLevel(m, false, MenuProjects), reloadViews()
	}
	if m = openPicker(m); m.Picker.Open {
		return setMenuLevel(m, true, MenuProjects), reloadViews()
	}
	return m, nil
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
	switch entry.Kind {
	case EntryLettered:
		m.Picker, m.Focus = PickerState{}, ColumnList
		m = leaveSidebarLevel(m)
		return showPage(entry.Page)(m)
	case EntryRestore:
		m.Picker, m.Focus = PickerState{}, ColumnList
		m = leaveSidebarLevel(m)
		return openRestore(m), nil
	case EntryNewProject:
		return openProjectForm(m), nil
	case EntryNewView:
		return openViewForm(m, "", SavedView{Name: "", Base: m.Collection, Filters: m.Filters}), nil
	case EntrySavedView:
		v, ok := m.savedViewNamed(entry.Title)
		if !ok {
			return m, nil
		}
		return openSavedView(m, v)
	case EntryViews:
		return setMenuLevel(m, popup, MenuViews), nil
	case EntryProjects:
		return setMenuLevel(m, popup, MenuProjects), nil
	case EntryBack:
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
	if m.Views.Deleting != "" { // the question takes only its answers
		if binding, ok := LookupBindingForMode(msg.Chord, ModeDeleteView); ok && binding.entry != nil {
			return binding.entry.handle(m)
		}
		return m, nil
	}
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
	case EditViewAction:
		return editSavedView(m, true), nil
	case AskDeleteView:
		return askDeleteView(m, true), nil
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

// MenuKeys are the bindings that open the Menu from where the Pane is now, which the top bar's Menu button shows as keycaps: esc, only
// where esc opens the Menu rather than going back or to the sidebar. None elsewhere.
func (m Model) MenuKeys() []Binding {
	if m.Focus != ColumnList || m.SidebarShown() || m.EscGoesBack() {
		return nil
	}
	return CardKeys(m.CurrentMode(), ShowMenu)
}
