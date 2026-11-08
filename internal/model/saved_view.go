package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ViewFieldError is a refusal of one field of a saved view, so a caller finds the input at fault without reading the message.
// The message starts with the field's label, as the View card and the CLI show it.
type ViewFieldError struct {
	Field Field
	Err   error
}

func (e ViewFieldError) Error() string { return e.Err.Error() }
func (e ViewFieldError) Unwrap() error { return e.Err }

const MaxViewNameLen = 64

// OwnerMe is the owner filter value that means the Pane's actor; it is saved as typed and resolved when the view opens.
const OwnerMe = "@me"

// SavedView is a named base Collection with the Filter card's fields and search text, kept in views.toml.
//
// Base is a fixed Collection (View is its key, Title its title) or a Project (View is the Project kind, with the Project's
// UID and the name it had when the view was saved). Filters.ProjectUID is the Filter card's Project field, separate from
// the base.
type SavedView struct {
	Name    string
	Base    Collection
	Filters Filters
}

type ViewRef struct {
	Collection *Collection
	View       *SavedView
}

func FixedCollectionNamed(name string) (Collection, bool) {
	name = strings.TrimSpace(name)
	for _, c := range FixedCollections() {
		if strings.EqualFold(c.Title, name) || strings.EqualFold(c.View, name) {
			return c, true
		}
	}
	return Collection{}, false
}

// ResolveViewName finds the fixed Collection or saved view called name, ignoring case. A saved view never shares a fixed
// Collection's name, so the fixed Collections are tried first only as a safeguard against a hand-edited file.
func ResolveViewName(name string, views []SavedView) (ViewRef, bool) {
	if c, ok := FixedCollectionNamed(name); ok {
		return ViewRef{Collection: &c}, true
	}
	if i := viewIndex(views, name); i >= 0 {
		return ViewRef{View: &views[i]}, true
	}
	return ViewRef{}, false
}

func SavedViewNamed(views []SavedView, name string) (SavedView, bool) {
	if i := viewIndex(views, name); i >= 0 {
		return views[i], true
	}
	return SavedView{}, false
}

func viewIndex(views []SavedView, name string) int {
	name = strings.TrimSpace(name)
	return slices.IndexFunc(views, func(v SavedView) bool { return strings.EqualFold(v.Name, name) })
}

// CheckViewName trims name and says why it cannot name a saved view. others are the saved views already kept; self is the name
// of the view being edited (empty for a new one), which may keep its own name. Messages start with the field, "Name".
func CheckViewName(name string, others []SavedView, self string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ViewFieldError{ViewName, errors.New("Name is required")}
	case strings.ContainsFunc(name, unicode.IsControl):
		return "", ViewFieldError{ViewName, errors.New("Name cannot contain control characters such as a newline or tab")}
	case utf8.RuneCountInString(name) > MaxViewNameLen:
		return "", ViewFieldError{ViewName, fmt.Errorf("Name is longer than %d characters", MaxViewNameLen)}
	}
	if c, ok := FixedCollectionNamed(name); ok {
		return "", ViewFieldError{ViewName, fmt.Errorf("Name %q is the fixed Collection %s", name, c.Title)}
	}
	for _, other := range others {
		if self != "" && strings.EqualFold(other.Name, self) {
			continue
		}
		if strings.EqualFold(other.Name, name) {
			return "", ViewFieldError{ViewName, fmt.Errorf("Name %q is already used by another view", name)}
		}
	}
	return name, nil
}

func CheckViewFilters(f Filters) (Filters, error) {
	f.ProjectUID = strings.TrimSpace(f.ProjectUID)
	f.Owner = strings.TrimSpace(f.Owner)
	f.Label = strings.TrimSpace(f.Label)
	f.Search = strings.TrimSpace(f.Search)
	if !hasChoice(FilterStatuses, f.Status) {
		return f, ViewFieldError{FilterStatus, fmt.Errorf("Status %q must be open or closed, or empty for all", f.Status)}
	}
	if !hasChoice(FilterRelationships, f.Relationship) {
		return f, ViewFieldError{FilterRelationship, fmt.Errorf("Relationship %q must be one of parent, child, blocks, blocked_by or related", f.Relationship)}
	}
	return f, nil
}

func hasChoice(choices []FilterChoice, value string) bool {
	return slices.ContainsFunc(choices, func(c FilterChoice) bool { return c.Value == value })
}

func CheckViewBase(c Collection) (Collection, error) {
	if c.IsProject() {
		c.ProjectUID = strings.TrimSpace(c.ProjectUID)
		if c.ProjectUID == "" {
			return c, ViewFieldError{ViewBase, errors.New("Base needs a Project")}
		}
		return c, nil
	}
	fixed, ok := FixedCollectionNamed(c.View)
	if !ok || !strings.EqualFold(fixed.View, c.View) {
		return c, ViewFieldError{ViewBase, fmt.Errorf("Base %q is not a Collection or a Project", c.View)}
	}
	return fixed, nil
}

// CheckSavedView is the View card's and the CLI's one check: it returns v trimmed, or the first refusal, naming its field.
// others and self are as in CheckViewName.
func CheckSavedView(v SavedView, others []SavedView, self string) (SavedView, error) {
	var err error
	if v.Name, err = CheckViewName(v.Name, others, self); err != nil {
		return v, err
	}
	if v.Base, err = CheckViewBase(v.Base); err != nil {
		return v, err
	}
	if v.Filters, err = CheckViewFilters(v.Filters); err != nil {
		return v, err
	}
	return v, nil
}

// CheckSavedViewIn is CheckSavedView for a view being written by hand: it also refuses a Project filter that differs from a
// Project base, which no Issue could match. projectName gives a Project's current name for the message, a UID when it knows
// none; the base then goes by its saved title.
// A file already holding such a view still loads, since CheckSavedView alone does not apply the rule.
func CheckSavedViewIn(v SavedView, others []SavedView, self string, projectName func(uid string) string) (SavedView, error) {
	v, err := CheckSavedView(v, others, self)
	if err != nil {
		return v, err
	}
	if v.Base.IsProject() && v.Filters.ProjectUID != "" && v.Filters.ProjectUID != v.Base.ProjectUID {
		base := projectName(v.Base.ProjectUID)
		if base == v.Base.ProjectUID {
			base = v.Base.Title
		}
		return v, ViewFieldError{FilterProject, fmt.Errorf("Project %q is not the base Project %q, so no Issue would match", projectName(v.Filters.ProjectUID), base)}
	}
	return v, nil
}

func (v SavedView) FiltersFor(actor string) Filters {
	f := v.Filters
	if strings.TrimSpace(f.Owner) == OwnerMe {
		f.Owner = actor
	}
	return f
}
