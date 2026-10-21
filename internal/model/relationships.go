package model

import (
	"fmt"
	"slices"
	"strings"
)

// RelationshipRef is the other end of a Relationship: the Issue's UID, its qualified id (project#id) and its status.
type RelationshipRef struct {
	UID, QualifiedID, Status string
}

// Open says the Issue on the other end is open.
func (r RelationshipRef) Open() bool { return r.Status == "open" }

// ID is the Issue's id as shown from project: the short id inside it, the qualified id outside.
func (r RelationshipRef) ID(project string) string {
	if short, ok := strings.CutPrefix(r.QualifiedID, project+"#"); ok {
		return short
	}
	return r.QualifiedID
}

// Relationships are one Issue's Relationships, as seen from it: each other Issue appears once per kind, in link order.
type Relationships struct {
	Parent    *RelationshipRef
	Children  []RelationshipRef
	Blocks    []RelationshipRef
	BlockedBy []RelationshipRef
	Related   []RelationshipRef
}

// OpenBlockers are the Issues that block this one and are still open.
func (r Relationships) OpenBlockers() []RelationshipRef { return openRefs(r.BlockedBy) }

// OpenChildren are the children that are still open.
func (r Relationships) OpenChildren() []RelationshipRef { return openRefs(r.Children) }

// OpenBlocked are the Issues this one blocks that are still open.
func (r Relationships) OpenBlocked() []RelationshipRef { return openRefs(r.Blocks) }

func openRefs(refs []RelationshipRef) []RelationshipRef {
	return slices.DeleteFunc(slices.Clone(refs), func(r RelationshipRef) bool { return !r.Open() })
}

// RelationshipIndex is every Issue's Relationships, built once from the Collection snapshot's links.
type RelationshipIndex map[string]*Relationships

// NewRelationshipIndex reads links: a parent link runs from the child to the parent, a blocks link from the blocker to the Issue
// it blocks, a blocked_by link the other way round, and a related link has no direction. Self-links are ignored, an Issue is
// kept once per kind, and an Issue with several parent links has the first by link id.
func NewRelationshipIndex(links []Link) RelationshipIndex {
	ordered := slices.Clone(links)
	slices.SortStableFunc(ordered, func(a, b Link) int { return int(a.ID - b.ID) })
	index := RelationshipIndex{}
	for _, l := range ordered {
		if l.FromIssueUID == l.ToIssueUID {
			continue
		}
		from := RelationshipRef{UID: l.FromIssueUID, QualifiedID: l.FromQualifiedID, Status: l.FromStatus}
		to := RelationshipRef{UID: l.ToIssueUID, QualifiedID: l.ToQualifiedID, Status: l.ToStatus}
		switch l.Type {
		case "parent":
			if r := index.of(from.UID); r.Parent == nil {
				r.Parent = &to
			}
			index.of(to.UID).Children = addRef(index.of(to.UID).Children, from)
		case "blocks":
			index.of(from.UID).Blocks = addRef(index.of(from.UID).Blocks, to)
			index.of(to.UID).BlockedBy = addRef(index.of(to.UID).BlockedBy, from)
		case "blocked_by":
			index.of(from.UID).BlockedBy = addRef(index.of(from.UID).BlockedBy, to)
			index.of(to.UID).Blocks = addRef(index.of(to.UID).Blocks, from)
		case "related":
			index.of(from.UID).Related = addRef(index.of(from.UID).Related, to)
			index.of(to.UID).Related = addRef(index.of(to.UID).Related, from)
		}
	}
	return index
}

func (x RelationshipIndex) of(uid string) *Relationships {
	if x[uid] == nil {
		x[uid] = &Relationships{}
	}
	return x[uid]
}

// Of is uid's Relationships; an Issue with none has the zero value.
func (x RelationshipIndex) Of(uid string) Relationships {
	if r := x[uid]; r != nil {
		return *r
	}
	return Relationships{}
}

func addRef(refs []RelationshipRef, ref RelationshipRef) []RelationshipRef {
	if slices.ContainsFunc(refs, func(r RelationshipRef) bool { return r.UID == ref.UID }) {
		return refs
	}
	return append(refs, ref)
}

// Highlight is how an Issue relates to the selected one, named from the selected Issue's side.
type Highlight string

// Highlights, in ring precedence: the first one an Issue has gives its ring.
const (
	HighlightBlocksThis    Highlight = "blocks this"
	HighlightBlockedByThis Highlight = "blocked by this"
	HighlightParent        Highlight = "parent"
	HighlightChild         Highlight = "child"
	HighlightRelated       Highlight = "related"
)

// HighlightsOf are the Relationships of uid to the Issue these Relationships belong to, in ring precedence, nil when unrelated.
func (r Relationships) HighlightsOf(uid string) []Highlight {
	has := func(refs []RelationshipRef) bool {
		return slices.ContainsFunc(refs, func(x RelationshipRef) bool { return x.UID == uid })
	}
	var out []Highlight
	if has(r.BlockedBy) {
		out = append(out, HighlightBlocksThis)
	}
	if has(r.Blocks) {
		out = append(out, HighlightBlockedByThis)
	}
	if r.Parent != nil && r.Parent.UID == uid {
		out = append(out, HighlightParent)
	}
	if has(r.Children) {
		out = append(out, HighlightChild)
	}
	if has(r.Related) {
		out = append(out, HighlightRelated)
	}
	return out
}

// showCard selects the Board card of uid, in the Lane view switching to its Lane, or leaves the Notice that the Board does not draw it.
func showCard(m Model, uid string) Model {
	if !m.Board.On() {
		return m
	}
	columns := m.BoardColumns()
	col, _ := CardAt(columns, uid)
	if col < 0 {
		m.Notice = fmt.Sprintf("%s isn't on the Board.", m.relatedID(uid))
		return m
	}
	if m.Board.Mode == BoardLane {
		m.Board.Lane = columns[col].Lane
	}
	m.SelectedUID, m.Focus = uid, ColumnList
	return m
}

// selectParent is `^`: the selected card's parent's card, or the Notice that it has none.
func selectParent(m Model) Model {
	issue := m.issueByUID(m.SelectedUID)
	if issue == nil {
		return m
	}
	parent := NewRelationshipIndex(m.Snapshot.Links).Of(issue.UID).Parent
	if parent == nil {
		m.Notice = fmt.Sprintf("%s has no parent.", issue.ShortID)
		return m
	}
	return showCard(m, parent.UID)
}

// relatedID is uid's id as the selected Issue's Relationships line names it, the bare uid when it is not one of them.
func (m Model) relatedID(uid string) string {
	issue := m.issueByUID(m.SelectedUID)
	if issue == nil {
		return uid
	}
	r := NewRelationshipIndex(m.Snapshot.Links).Of(issue.UID)
	groups := [][]RelationshipRef{r.Children, r.Blocks, r.BlockedBy, r.Related}
	if r.Parent != nil {
		groups = append(groups, []RelationshipRef{*r.Parent})
	}
	for _, refs := range groups {
		if i := slices.IndexFunc(refs, func(ref RelationshipRef) bool { return ref.UID == uid }); i >= 0 {
			return refs[i].ID(issue.ProjectName)
		}
	}
	return uid
}
