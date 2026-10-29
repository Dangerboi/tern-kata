package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxViewNameLen is the longest a saved view's name may be, in characters.
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

// ViewRef is what a name given to `views show` or `views rm` resolves to: a fixed Collection or a saved view.
type ViewRef struct {
	Collection *Collection
	View       *SavedView
}

// FixedCollectionNamed is the fixed Collection whose title or key equals name, ignoring case.
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
	name = strings.TrimSpace(name)
	for i := range views {
		if strings.EqualFold(views[i].Name, name) {
			return ViewRef{View: &views[i]}, true
		}
	}
	return ViewRef{}, false
}

// CheckViewName trims name and says why it cannot name a saved view. others are the saved views already kept; self is the name
// of the view being edited (empty for a new one), which may keep its own name. Messages start with the field, "Name".
func CheckViewName(name string, others []SavedView, self string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", errors.New("Name is required")
	case utf8.RuneCountInString(name) > MaxViewNameLen:
		return "", fmt.Errorf("Name is longer than %d characters", MaxViewNameLen)
	}
	if c, ok := FixedCollectionNamed(name); ok {
		return "", fmt.Errorf("Name %q is the built-in Collection %s", name, c.Title)
	}
	for _, other := range others {
		if self != "" && strings.EqualFold(other.Name, self) {
			continue
		}
		if strings.EqualFold(other.Name, name) {
			return "", fmt.Errorf("Name %q is already used by another view", name)
		}
	}
	return name, nil
}

// CheckViewFilters trims the text fields and says which filter value the Filter card would not accept.
func CheckViewFilters(f Filters) (Filters, error) {
	f.ProjectUID = strings.TrimSpace(f.ProjectUID)
	f.Owner = strings.TrimSpace(f.Owner)
	f.Label = strings.TrimSpace(f.Label)
	f.Search = strings.TrimSpace(f.Search)
	if !hasChoice(FilterStatuses, f.Status) {
		return f, fmt.Errorf("Status %q must be open or closed, or empty for all", f.Status)
	}
	if !hasChoice(FilterRelationships, f.Relationship) {
		return f, fmt.Errorf("Relationship %q must be one of parent, child, blocks, blocked_by or related", f.Relationship)
	}
	return f, nil
}

func hasChoice(choices []FilterChoice, value string) bool {
	for _, c := range choices {
		if c.Value == value {
			return true
		}
	}
	return false
}

// CheckViewBase says why c cannot be a saved view's base: a fixed Collection by key, or a Project with a UID.
func CheckViewBase(c Collection) (Collection, error) {
	if c.IsProject() {
		c.ProjectUID = strings.TrimSpace(c.ProjectUID)
		if c.ProjectUID == "" {
			return c, errors.New("Base needs a Project")
		}
		return c, nil
	}
	fixed, ok := FixedCollectionNamed(c.View)
	if !ok || !strings.EqualFold(fixed.View, c.View) {
		return c, fmt.Errorf("Base %q is not a Collection or a Project", c.View)
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

// FiltersFor is the view's filters with an owner of @me replaced by actor, which is when the view opens.
func (v SavedView) FiltersFor(actor string) Filters {
	f := v.Filters
	if strings.TrimSpace(f.Owner) == OwnerMe {
		f.Owner = actor
	}
	return f
}
