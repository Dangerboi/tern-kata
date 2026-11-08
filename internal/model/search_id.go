package model

import "strings"

type IDSearch struct{ ShortID, UID string }

type LookupSearchID struct {
	Workspace string
	ProjectID int64
	ShortID   string
	Request   int64
}

func (LookupSearchID) cmd() {}

type SearchIDLooked struct {
	Request      int64
	ShortID, UID string
}

func (SearchIDLooked) msg() {}

// crockford is the alphabet of kata's lowercase short ids.
const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

// searchShortID reads the query as a kata short id: 4 to 26 characters of the
// short id alphabet, with or without a leading #, in any case.
func searchShortID(query string) (string, bool) {
	id := strings.ToLower(strings.TrimPrefix(query, "#"))
	return id, len(id) >= 4 && len(id) <= 26 && strings.Trim(id, crockford) == ""
}

func (m Model) idMatch() string {
	if id, ok := searchShortID(m.Filters.Search); ok && id == m.IDSearch.ShortID {
		return m.IDSearch.UID
	}
	return ""
}

// idLookup is the lookup the committed search still needs: it is a short id the
// lookup has not answered, in a Project the catalog knows: the Collection's, else
// the filter's, else the bound one.
func (m Model) idLookup() (LookupSearchID, bool) {
	id, ok := searchShortID(m.Filters.Search)
	if !ok || id == m.IDSearch.ShortID {
		return LookupSearchID{}, false
	}
	for _, uid := range []string{m.Collection.ProjectUID, m.Filters.ProjectUID, m.Project.UID} {
		if uid == "" {
			continue
		}
		if entry, ok := CatalogEntry(m.Snapshot, uid); ok {
			return LookupSearchID{Workspace: m.Workspace, ProjectID: entry.Project.ID, ShortID: id}, true
		}
	}
	return LookupSearchID{}, false
}
