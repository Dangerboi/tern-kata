package model

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ViewsState is what the Pane knows of views.toml: the saved views it read at start and each time the Menu opened, in file
// order, and what to hand back to the next write.
type ViewsState struct {
	List []SavedView
	// Token is what the last read returned, opaque to the model; a write hands it back and the file refuses it when it changed
	// since. Nil when the file was not read, which no write can follow.
	Token any
	// Err is why the file could not be read (it is left as it is), Problems the entries the read left out.
	Err      error
	Problems []error
	// Deleting is the name of the view the question "Delete view <name>? y/n" is about, "" when none is asked.
	Deleting string
}

// LoadViews reads views.toml.
type LoadViews struct{}

func (LoadViews) cmd() {}

// ViewsLoaded answers LoadViews, and rides every ViewSaved and ViewDeleted as the read that follows a write.
type ViewsLoaded struct {
	Views    []SavedView
	Problems []error
	Token    any
	Err      error
}

func (ViewsLoaded) msg() {}

// SaveView writes View into views.toml: as a new view when Self is empty, else in place of the view called Self (an edit or a
// rename). Token is the read the write rests on.
type SaveView struct {
	Self  string
	View  SavedView
	Token any
}

func (SaveView) cmd() {}

// ViewSaved answers SaveView. Changed says the file was no longer as the Token read it; nothing was written. Loaded is the read
// that follows, so the Pane holds the file as it is now.
type ViewSaved struct {
	Sent    SaveView
	Changed bool
	Err     error
	Loaded  ViewsLoaded
}

func (ViewSaved) msg() {}

// DeleteView removes the view called Name from views.toml.
type DeleteView struct {
	Name  string
	Token any
}

func (DeleteView) cmd() {}

// ViewDeleted answers DeleteView, as ViewSaved answers SaveView.
type ViewDeleted struct {
	Sent    DeleteView
	Changed bool
	Err     error
	Loaded  ViewsLoaded
}

func (ViewDeleted) msg() {}

const (
	NewViewKey   = "new-view"
	NewViewTitle = "New view"
	// viewKeyPrefix starts the key of a saved view's row; an index follows, so a name with spaces never reaches a node id.
	viewKeyPrefix = "view."
)

// The reasons a saved view's base cannot be opened.
const (
	ProjectGone     = "Project gone"
	ProjectArchived = "archived"
)

func viewsLoaded(m Model, msg ViewsLoaded) Model {
	m.Views = ViewsState{List: msg.Views, Token: msg.Token, Err: msg.Err, Problems: msg.Problems, Deleting: m.Views.Deleting}
	if _, ok := m.savedViewNamed(m.Views.Deleting); m.Views.Deleting != "" && !ok {
		m.Views.Deleting = ""
	}
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Saved views not read: %v", msg.Err)
	} else if len(msg.Problems) > 0 {
		m.Notice = fmt.Sprintf("Saved views left out: %v", msg.Problems[0])
	}
	return m.keepActiveView()
}

// keepActiveView forgets the view this Pane opened once the file no longer has it, so the filter line never names a view that is gone.
func (m Model) keepActiveView() Model {
	if _, ok := m.savedViewNamed(m.ActiveView.Name); m.ActiveView.Name != "" && !ok {
		m.ActiveView = ActiveView{}
	}
	return m
}

// SavedViews are the saved views by name, ignoring case: the order the Menu lists them in.
func (m Model) SavedViews() []SavedView {
	views := slices.Clone(m.Views.List)
	slices.SortStableFunc(views, func(a, b SavedView) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Name, b.Name))
	})
	return views
}

func (m Model) savedViewNamed(name string) (SavedView, bool) {
	return SavedViewNamed(m.Views.List, name)
}

// ViewBaseNote says why the saved view cannot be opened: its base Project is gone from the catalog or archived. "" when the
// base is there, or while the catalog is not known, which says nothing yet.
func (m Model) ViewBaseNote(v SavedView) string {
	if !v.Base.IsProject() || len(m.Snapshot.Catalog) == 0 {
		return ""
	}
	entry, ok := CatalogEntry(m.Snapshot, v.Base.ProjectUID)
	switch {
	case !ok:
		return ProjectGone
	case entry.Project.Archived:
		return ProjectArchived
	}
	return ""
}

// EntryTitle is how a Menu row reads: its title, and for a saved view whose base is missing the reason, in parentheses.
func (m Model) EntryTitle(e PickerEntry) string {
	if e.Kind == EntrySavedView {
		if v, ok := m.savedViewNamed(e.Title); ok {
			if note := m.ViewBaseNote(v); note != "" {
				return e.Title + " (" + note + ")"
			}
		}
	}
	return e.Title
}

// ActiveView is the saved view this Pane opened, its base and the filters it applied (the owner already resolved), so the filter
// line can say while the Collection and the filters are still the view's.
type ActiveView struct {
	Name    string
	Base    Collection
	Filters Filters
}

// CurrentSavedView is the name of the saved view the Pane opened while its base is still the current Collection and the
// filters still equal it, "" otherwise.
func (m Model) CurrentSavedView() string {
	if m.ActiveView.Name != "" && m.Collection == m.ActiveView.Base && m.Filters == m.ActiveView.Filters {
		return m.ActiveView.Name
	}
	return ""
}

// openSavedView makes the view's base Collection current with its filters applied, @me being this Pane's actor. A base that is
// gone or archived says so and changes nothing.
func openSavedView(m Model, v SavedView) (Model, []Cmd) {
	switch m.ViewBaseNote(v) {
	case ProjectGone:
		m.Notice = fmt.Sprintf("View %s: its Project is gone; nothing changed", v.Name)
		return m, nil
	case ProjectArchived:
		m.Notice = fmt.Sprintf("View %s: its Project is archived; nothing changed", v.Name)
		return m, nil
	}
	base := v.Base
	if base.IsProject() {
		entry, _ := CatalogEntry(m.Snapshot, base.ProjectUID)
		base = ProjectCollection(base.ProjectUID, entry.Project.Name)
	}
	if !slices.Contains(m.Collections, base) {
		m.Notice = fmt.Sprintf("View %s: its Collection is not loaded yet; nothing changed", v.Name)
		return m, nil
	}
	filters := v.FiltersFor(m.Actor)
	m.Picker, m.Focus = PickerState{}, ColumnList
	m = leaveSidebarLevel(m)
	m.Filters, m.SelectedUID = filters, ""
	m.ActiveView = ActiveView{Name: v.Name, Base: base, Filters: filters}
	m, cmds := updateCollection(m, CollectionSelected{Collection: base})
	return m.cursorOnView(v.Name), cmds
}

// cursorOnView puts the sidebar's cursor on the saved view's row when the sidebar shows the Views level.
func (m Model) cursorOnView(name string) Model {
	if m.SidebarLevel != MenuViews {
		return m
	}
	for i, e := range m.SidebarEntries() {
		if e.Kind == EntrySavedView && strings.EqualFold(e.Title, name) {
			m.CollectionCursor = i
		}
	}
	return m
}

// selectedSavedView is the saved view under the cursor of the popup (or, with popup false, the sidebar).
func (m Model) selectedSavedView(popup bool) (SavedView, bool) {
	entries, cursor := m.SidebarEntries(), m.CollectionCursor
	if popup {
		entries, cursor = m.PickerEntries(), m.Picker.Cursor
	}
	if len(entries) == 0 {
		return SavedView{}, false
	}
	entry := entries[min(cursor, len(entries)-1)]
	if entry.Kind != EntrySavedView {
		return SavedView{}, false
	}
	return m.savedViewNamed(entry.Title)
}

// editSavedView is the edit key on a saved view's row: the View card, prefilled.
func editSavedView(m Model, popup bool) Model {
	if v, ok := m.selectedSavedView(popup); ok {
		return openViewForm(m, v.Name, v)
	}
	return m
}

// askDeleteView is the delete key on a saved view's row: it asks before the file is touched.
func askDeleteView(m Model, popup bool) Model {
	if v, ok := m.selectedSavedView(popup); ok {
		m.Views.Deleting = v.Name
	}
	return m
}

// DeleteViewQuestion is the question asked before a saved view is deleted.
func DeleteViewQuestion(name string) string { return "Delete view " + name + "? y/n" }

// confirmDeleteView is y: it dismisses the question at once and sends the delete; the file's answer follows as ViewDeleted.
func confirmDeleteView(m Model) (Model, []Cmd) {
	name := m.Views.Deleting
	if name == "" {
		return m, nil
	}
	m.Views.Deleting = ""
	if m.Views.Token == nil {
		m.Notice = "Saved views not deleted: " + cmp.Or(errText(m.Views.Err), "views.toml was not read")
		return m, nil
	}
	return m, []Cmd{DeleteView{Name: name, Token: m.Views.Token}}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// dismissDeleteQuestion is n or esc: the view stays and the question goes.
func dismissDeleteQuestion(m Model) Model {
	m.Views.Deleting = ""
	return m
}

func viewDeleted(m Model, msg ViewDeleted) (Model, []Cmd) {
	m = viewsLoaded(m, msg.Loaded)
	switch {
	case msg.Changed:
		m.Notice = fmt.Sprintf("views.toml changed since it was read; view %s was not deleted", msg.Sent.Name)
	case msg.Err != nil:
		m.Notice = fmt.Sprintf("View %s not deleted: %v", msg.Sent.Name, msg.Err)
	default:
		m.Notice = "Deleted view " + msg.Sent.Name
		if m.ActiveView.Name != "" && strings.EqualFold(m.ActiveView.Name, msg.Sent.Name) {
			m.ActiveView = ActiveView{}
		}
	}
	m.clampMenuCursors()
	return m, nil
}

// clampMenuCursors keeps both cursors on a row after the list shrank.
func (m *Model) clampMenuCursors() {
	m.Picker.Cursor = max(0, min(m.Picker.Cursor, len(m.PickerEntries())-1))
	m.CollectionCursor = max(0, min(m.CollectionCursor, len(m.SidebarEntries())-1))
}

// viewSaved is the answer to a save: the View card closes and the view opens, or the card stays with its input and says why.
func viewSaved(m Model, msg ViewSaved) (Model, []Cmd) {
	m = viewsLoaded(m, msg.Loaded)
	form := m.ViewForm
	ours := form.Open && form.Saving && form.Sent.Self == msg.Sent.Self && form.Sent.View == msg.Sent.View
	if msg.Err != nil || msg.Changed {
		if ours {
			m.ViewForm.Saving = false
			m.ViewForm.Err = msg.Err
			if msg.Changed {
				m.ViewForm.Err = errors.New("views.toml changed since it was read; the view was not saved. Check the name and save again")
			}
		}
		return m, nil
	}
	if !ours {
		return m, nil
	}
	m.ViewForm = ViewFormState{}
	saved, ok := m.savedViewNamed(msg.Sent.View.Name)
	if !ok {
		return m, nil
	}
	if msg.Sent.Self != "" && m.ActiveView.Name != "" && strings.EqualFold(m.ActiveView.Name, msg.Sent.Self) {
		m.ActiveView = ActiveView{} // an edited view is opened again below, so the line names the new one
	}
	return openSavedView(m, saved)
}

// reloadViews is what opening the Menu asks: the file as it is now.
func reloadViews() []Cmd { return []Cmd{LoadViews{}} }
