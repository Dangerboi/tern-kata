package model

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	// Deleting is the name of the view the question "Delete view <name>?" is about, "" when none is asked.
	Deleting string
}

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

type DeleteView struct {
	Name  string
	Token any
}

func (DeleteView) cmd() {}

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

const (
	ProjectGone     = "Project gone"
	ProjectArchived = "archived"
)

// errViewsChanged is why a save or delete of a saved view is refused after another program wrote the file.
var errViewsChanged = errors.New("views.toml changed since it was read")

func viewsLoaded(m Model, msg ViewsLoaded) Model {
	m.Views = ViewsState{List: msg.Views, Token: msg.Token, Err: msg.Err, Problems: msg.Problems, Deleting: m.Views.Deleting}
	if _, ok := m.savedViewNamed(m.Views.Deleting); m.Views.Deleting != "" && !ok {
		m.Views.Deleting = ""
	}
	return m.keepActiveView()
}

// problemName is the quoted name in a read problem's "[[view]] 2 ("name")" marker, "" when the text has none.
var problemName = regexp.MustCompile(`\[\[view\]\] \d+ \(("(?:[^"\\]|\\.)*")\)`)

// ViewsNote is the one line the Menu shows about views.toml: a failed read in full, else how many saved views the read skipped
// and, for one, which and why. The path and the file's detail are on the Config Page. text is "" when there is nothing to say.
func (m Model) ViewsNote() (text string, failed bool) {
	if m.Views.Err != nil {
		return Cannot("read", "saved views", m.Views.Err), true
	}
	problems := m.Views.Problems
	if len(problems) == 0 {
		return "", false
	}
	head := Count(len(problems), "saved view", "saved views") + " skipped"
	if len(problems) > 1 {
		return head + ".", false
	}
	var name string
	if found := problemName.FindStringSubmatch(problems[0].Error()); found != nil {
		name, _ = strconv.Unquote(found[1])
	}
	reason := Reason(problems[0])
	var field ViewFieldError
	if errors.As(problems[0], &field) {
		reason = Reason(field.Err)
	}
	if first, size := utf8.DecodeRuneInString(reason); size < len(reason) && unicode.IsUpper(first) && !unicode.IsUpper([]rune(reason)[1]) {
		reason = string(unicode.ToLower(first)) + reason[size:]
	}
	if name != "" {
		reason = name + ": " + reason
	}
	return head + ": " + reason + ".", false
}

// keepActiveView forgets the view this Pane opened once the file no longer has it, or lists another view of that name (a
// duplicate that followed the deleted one) with a different base or filters, so the filter line never names a view that is gone.
func (m Model) keepActiveView() Model {
	if m.ActiveView.Name == "" {
		return m
	}
	v, ok := m.savedViewNamed(m.ActiveView.Name)
	if !ok || !sameBase(v.Base, m.ActiveView.Base) || v.FiltersFor(m.Actor) != m.ActiveView.Filters {
		m.ActiveView = ActiveView{}
	}
	return m
}

// sameBase is whether two bases are one Collection, a Project base by its UID since its name may have changed.
func sameBase(a, b Collection) bool {
	if a.IsProject() || b.IsProject() {
		return a.IsProject() && b.IsProject() && a.ProjectUID == b.ProjectUID
	}
	return a.View == b.View
}

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
		return m.fail(fmt.Sprintf("Cannot open view %s: its Project is gone.", v.Name)), nil
	case ProjectArchived:
		return m.fail(fmt.Sprintf("Cannot open view %s: its Project is archived.", v.Name)), nil
	}
	base := v.Base
	if base.IsProject() {
		entry, _ := CatalogEntry(m.Snapshot, base.ProjectUID)
		base = ProjectCollection(base.ProjectUID, entry.Project.Name)
	}
	if !slices.Contains(m.Collections, base) {
		return m.fail(fmt.Sprintf("Cannot open view %s: its Collection is not loaded yet.", v.Name)), nil
	}
	filters := v.FiltersFor(m.Actor)
	m.Picker, m.Focus = PickerState{}, ColumnList
	m = leaveSidebarLevel(m)
	m.Filters, m.SelectedUID = filters, ""
	m.ActiveView = ActiveView{Name: v.Name, Base: base, Filters: filters}
	m, cmds := updateCollection(m, CollectionSelected{Collection: base})
	return m.cursorOnView(v.Name), cmds
}

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

func editSavedView(m Model, popup bool) Model {
	if v, ok := m.selectedSavedView(popup); ok {
		return openViewForm(m, v.Name, v)
	}
	return m
}

func askDeleteView(m Model, popup bool) Model {
	if v, ok := m.selectedSavedView(popup); ok {
		m.Views.Deleting = v.Name
	}
	return m
}

// confirmDeleteView is y: it dismisses the question at once and sends the delete; the file's answer follows as ViewDeleted.
func confirmDeleteView(m Model) (Model, []Cmd) {
	name := m.Views.Deleting
	if name == "" {
		return m, nil
	}
	m.Views.Deleting = ""
	if m.Views.Token == nil {
		reason := errors.New("views.toml was not read")
		if m.Views.Err != nil {
			reason = m.Views.Err
		}
		return m.fail(Cannot("delete", "saved views", reason)), nil
	}
	return m, []Cmd{DeleteView{Name: name, Token: m.Views.Token}}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func dismissDeleteQuestion(m Model) Model {
	m.Views.Deleting = ""
	return m
}

func viewDeleted(m Model, msg ViewDeleted) (Model, []Cmd) {
	m = viewsLoaded(m, msg.Loaded)
	switch {
	case msg.Changed:
		m = m.fail(Cannot("delete", "view "+msg.Sent.Name, errViewsChanged))
	case msg.Err != nil:
		m = m.fail(Cannot("delete", "view "+msg.Sent.Name, msg.Err))
	default:
		m = m.say("Deleted view " + msg.Sent.Name + ".")
	}
	m.clampMenuCursors()
	return m, nil
}

func (m *Model) clampMenuCursors() {
	m.Picker.Cursor = max(0, min(m.Picker.Cursor, len(m.PickerEntries())-1))
	m.CollectionCursor = max(0, min(m.CollectionCursor, len(m.SidebarEntries())-1))
}

func viewSaved(m Model, msg ViewSaved) (Model, []Cmd) {
	m = viewsLoaded(m, msg.Loaded)
	form := m.ViewForm
	ours := form.Open && form.Saving && form.Sent.Self == msg.Sent.Self && form.Sent.View == msg.Sent.View
	if msg.Err != nil || msg.Changed {
		switch {
		case ours:
			m.ViewForm.Saving = false
			m.ViewForm.Err = msg.Err
			if msg.Changed {
				m.ViewForm.Err = errViewsChanged
			}
		case msg.Changed:
			m = m.fail(Cannot("save", "view "+msg.Sent.View.Name, errViewsChanged))
		default:
			m = m.fail(Cannot("save", "view "+msg.Sent.View.Name, msg.Err))
		}
		return m, nil
	}
	if !ours {
		return m.say("Saved view " + msg.Sent.View.Name + "."), nil
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

func reloadViews() []Cmd { return []Cmd{LoadViews{}} }
