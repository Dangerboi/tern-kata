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

type MenuLevel int

const (
	MenuTop MenuLevel = iota
	MenuViews
	MenuProjects
)

type PickerState struct {
	Open   bool
	Filter string
	Caret  Caret
	Cursor int
	Level  MenuLevel
}

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

type EntryKind int

const (
	EntryCollection EntryKind = iota
	EntryNewProject
	EntryProjects
	EntryViews
	EntryBack
	EntryLettered // a Lettered Page, named by PickerEntry.Page
	EntryRestore
	EntryNewView
	EntrySavedView // a saved view: Title is its name and Index its place in SavedViews
)

type PickerEntry struct {
	Collection
	Kind  EntryKind
	Index int
	Page  Page
}

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

func (e PickerEntry) IsCollection() bool { return e.Kind == EntryCollection }

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
	if match(RestoreTitle) && !m.IssueScoped {
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

func (m Model) savedViewEntries(match func(string) bool) []PickerEntry {
	var entries []PickerEntry
	for i, v := range m.SavedViews() {
		if match(v.Name) {
			entries = append(entries, PickerEntry{Collection: Collection{Title: v.Name}, Kind: EntrySavedView, Index: i})
		}
	}
	return entries
}

func (m Model) PickerEntries() []PickerEntry {
	return m.menuEntries(m.Picker.Level, m.Picker.Filter)
}

func (m Model) SidebarEntries() []PickerEntry { return m.menuEntries(m.SidebarLevel, "") }

func (m Model) CurrentView() string {
	if m.Collection.IsProject() {
		return ""
	}
	return m.Collection.Title
}

func entryIndex(entries []PickerEntry, key string) int {
	return max(0, slices.IndexFunc(entries, func(e PickerEntry) bool { return e.Key() == key }))
}

func (m Model) syncSidebarCursor() Model {
	for i, entry := range m.SidebarEntries() {
		if entry.IsCollection() && entry.Collection == m.Collection {
			m.CollectionCursor = i
		}
	}
	return m
}

func openPicker(m Model) Model {
	if slices.Contains(pageModes, m.CurrentMode()) {
		m.Picker = PickerState{Open: true}
		if m.CurrentView() != "" {
			m.Picker.Cursor = entryIndex(m.PickerEntries(), ViewsKey)
		}
	}
	return m
}

func openProjects(m Model) (Model, []Cmd) { return openMenuLevel(m, MenuProjects) }

func openMenuLevel(m Model, level MenuLevel) (Model, []Cmd) {
	if m.SidebarShown() {
		m.Picker, m.Focus = PickerState{}, ColumnCollections
		return setMenuLevel(m, false, level), reloadViews()
	}
	if m = openPicker(m); m.Picker.Open {
		return setMenuLevel(m, true, level), reloadViews()
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

// menuUp is every way of leaving a Views or Projects level (esc, Backspace, Back, a click on Back):
// the popup (when popup) or the sidebar returns to the top level with the cursor on the row that led
// there. It reports false, changing nothing, at the top level.
func menuUp(m Model, popup bool) (Model, bool) {
	level := m.SidebarLevel
	if popup {
		level = m.Picker.Level
	}
	if level == MenuTop {
		return m, false
	}
	return setMenuLevel(m, popup, MenuTop), true
}

func leaveSidebarLevel(m Model) Model {
	if m.SidebarLevel != MenuViews {
		m = setMenuLevel(m, false, MenuViews)
	}
	return m
}

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
		m, _ = menuUp(m, popup)
		return m, nil
	}
	m.Picker, m.Focus = PickerState{}, ColumnList
	m = leaveSidebarLevel(m)
	return updateCollection(m, CollectionSelected{Collection: entry.Collection})
}

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
	if m.Views.Deleting != "" {
		if binding, ok := LookupBindingForMode(msg.Chord, ModeDeleteView); ok && binding.entry != nil {
			return binding.run(m)
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
		if up, ok := menuUp(m, true); ok && m.Picker.Filter == "" {
			return up, nil
		}
		m.Picker = PickerState{}
	case PickCollection:
		return pickCollection(m)
	case EditViewAction:
		return editSavedView(m, true), nil
	case AskDeleteView:
		return askDeleteView(m, true), nil
	case MoveDown, MoveUp, MoveFirst, MoveLast:
		moveCursor(&m.Picker.Cursor, last+1, binding.Action)
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
