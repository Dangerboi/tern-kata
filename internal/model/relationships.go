package model

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type RelationshipRef struct {
	UID, QualifiedID, Status string
}

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

func (r Relationships) OpenBlockers() []RelationshipRef { return openRefs(r.BlockedBy) }

func (r Relationships) OpenChildren() []RelationshipRef { return openRefs(r.Children) }

func (r Relationships) OpenBlocked() []RelationshipRef { return openRefs(r.Blocks) }

func openRefs(refs []RelationshipRef) []RelationshipRef {
	return slices.DeleteFunc(slices.Clone(refs), func(r RelationshipRef) bool { return !r.Open() })
}

type RelationshipIndex map[string]*Relationships

// NewRelationshipIndex reads links: a parent link runs from the child to the parent, a blocks link from the blocker to the Issue
// it blocks, a blocked_by link the other way round, and a related link has no direction. Self-links are ignored, an Issue is
// kept once per kind, and an Issue with several parent links has the first by link id.
func NewRelationshipIndex(links []Link) RelationshipIndex {
	ordered := slices.Clone(links)
	slices.SortStableFunc(ordered, func(a, b Link) int { return cmp.Compare(a.ID, b.ID) })
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

type Highlight string

// Highlights, in ring precedence: the first one an Issue has gives its ring.
const (
	HighlightBlocksThis    Highlight = "blocks this"
	HighlightBlockedByThis Highlight = "blocked by this"
	HighlightParent        Highlight = "parent"
	HighlightChild         Highlight = "child"
	HighlightRelated       Highlight = "related"
)

// Find is the Relationships of uid to the Issue these Relationships belong to, in ring precedence, each with the ref to uid
// as the Issue's Relationships name it; nil when unrelated.
func (r Relationships) Find(uid string) []Found {
	var out []Found
	look := func(h Highlight, refs ...RelationshipRef) {
		if i := slices.IndexFunc(refs, func(x RelationshipRef) bool { return x.UID == uid }); i >= 0 {
			out = append(out, Found{h, refs[i]})
		}
	}
	look(HighlightBlocksThis, r.BlockedBy...)
	look(HighlightBlockedByThis, r.Blocks...)
	if r.Parent != nil {
		look(HighlightParent, *r.Parent)
	}
	look(HighlightChild, r.Children...)
	look(HighlightRelated, r.Related...)
	return out
}

type Found struct {
	Highlight Highlight
	Ref       RelationshipRef
}

// HighlightsOf are the Relationships of uid to the Issue these Relationships belong to, in ring precedence, nil when unrelated.
func (r Relationships) HighlightsOf(uid string) []Highlight {
	var out []Highlight
	for _, f := range r.Find(uid) {
		out = append(out, f.Highlight)
	}
	return out
}

// showCard selects the Board card of uid, in the Lane view switching to its Lane. An Issue the Board does not draw opens in the
// detail beside the Lane view as a jump, so Esc comes back to the card.
func showCard(m Model, uid string) (Model, []Cmd) {
	if !m.OnBoard() {
		return m, nil
	}
	if target, across := m.otherProject(uid); across {
		return m.crossJump(uid, target, nil)
	}
	return m.placeCard(uid, true), nil
}

// placeCard puts uid's card on screen in the Board's Project. jump is whether an Issue the Board does not draw opens in the
// detail as a new step of the jump history; a follow across Projects recorded its step already.
func (m Model) placeCard(uid string, jump bool) Model {
	columns := m.BoardColumns()
	col, _ := CardAt(columns, uid)
	if col < 0 {
		if m.BoardShown() {
			m = m.laneView()
		}
		if jump {
			return m.jumpWithin(uid, nil)
		}
		m.SelectedUID, m.openedUID, m.landedOn, m.listJump = uid, uid, uid, false
		return m.focusDetail()
	}
	if m.LaneViewShown() {
		m.Board.Lane = columns[col].Lane
	}
	m.SelectedUID, m.Focus = uid, ColumnList
	return m
}

func selectParent(m Model) (Model, []Cmd) {
	issue := m.issueByUID(m.SelectedUID)
	if issue == nil {
		return m, nil
	}
	parent := m.Relationships.Of(issue.UID).Parent
	if parent == nil {
		m.Notice = fmt.Sprintf("%s has no parent.", issue.ShortID)
		return m, nil
	}
	return showCard(m, parent.UID)
}
