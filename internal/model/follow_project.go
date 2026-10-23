package model

import "strings"

// Following a ticket that belongs to another Project from a Project Collection switches the Pane to that Project: the same Page,
// the ticket selected once the Project's snapshot arrives. The jump history remembers where the Pane stood so Esc returns to it.

// crossPlace is where the Pane stood when a jump left for another Project.
type crossPlace struct {
	collection Collection
	page       Page
	mode       BoardMode
	lane       string
	focus      Column
	hidden     bool
}

// followState is a followed ticket waiting for its Project: its snapshot, and on the Board the Project's Lane list, which decide
// where its card is.
type followState struct {
	uid string
	// restore is a return to an earlier place: the selection is the only thing left to settle.
	restore  bool
	snapshot bool
}

// projectNameOf is the name of the Project uid's Issue belongs to, read from what the Pane holds of it; "" when it holds nothing.
func (m Model) projectNameOf(uid string) string {
	for _, issue := range m.Issues {
		if issue.UID == uid {
			return issue.ProjectName
		}
	}
	if m.authority != nil && m.authority.Issue != nil && m.authority.Issue.UID == uid {
		return m.authority.Issue.ProjectName
	}
	links := append(m.Snapshot.Links[:len(m.Snapshot.Links):len(m.Snapshot.Links)], m.authority.links()...)
	for _, link := range links {
		qualified := ""
		switch uid {
		case link.FromIssueUID:
			qualified = link.FromQualifiedID
		case link.ToIssueUID:
			qualified = link.ToQualifiedID
		}
		if name, _, ok := cutQualified(qualified); ok {
			return name
		}
	}
	return ""
}

func (d *IssueDetailData) links() []Link {
	if d == nil {
		return nil
	}
	return d.Links
}

// cutQualified splits a qualified id, project#short.
func cutQualified(id string) (project, short string, ok bool) {
	i := strings.LastIndex(id, "#")
	if i <= 0 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

// otherProject says following uid leaves the Project: the Collection is a Project's, and the Issue belongs to another Project the
// catalog lists. Collections that span Projects never switch.
func (m Model) otherProject(uid string) (CatalogProject, bool) {
	return m.otherProjectNamed(m.projectNameOf(uid))
}

func (m Model) otherProjectNamed(name string) (CatalogProject, bool) {
	if name == "" || !m.Collection.IsProject() {
		return CatalogProject{}, false
	}
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.Name == name && entry.Project.UID != m.Collection.ProjectUID {
			return entry, true
		}
	}
	return CatalogProject{}, false
}

// placeEntry is the jump history entry for leaving this place for another Project.
func (m Model) placeEntry() jumpEntry {
	row, hasRow := m.cursorRowID()
	return jumpEntry{uid: m.SelectedUID, tab: m.tab, row: row, hasRow: hasRow, cross: &crossPlace{
		collection: m.Collection, page: m.Page, mode: m.Board.Mode, lane: m.Board.Lane, focus: m.Focus, hidden: m.hidden,
	}}
}

// crossJump follows uid into target's Project. The Pane switches Collection at once and selects uid once the Project's snapshot
// lists it; on the Issues Page the detail is focused as any jump puts it, on the Board the card is selected where the Board draws it.
func (m Model) crossJump(uid string, target CatalogProject, landing *jumpEntry) (Model, []Cmd) {
	m.back = append(m.back[:len(m.back):len(m.back)], m.placeEntry())
	m.SelectedUID, m.landedOn = uid, uid
	m.landing = nil
	if landing != nil {
		landing.uid = uid
		m.tab, m.landing = landing.tab, landing
	}
	m.pending = uid
	m.follow = &followState{uid: uid}
	m.Collection = ProjectCollection(target.Project.UID, target.Project.Name)
	m.listJump = m.OnBoard()
	if m.OnBoard() {
		m.Focus = ColumnList
	} else {
		m.openedUID = uid
		m = m.focusDetail()
	}
	return m.collectionChosen()
}

// restorePlace is going back across Projects: the entry's Collection, Page, Lane view, selection and focus, once its snapshot arrives.
func (m Model) restorePlace(entry jumpEntry) (Model, []Cmd) {
	place := entry.cross
	m.Collection, m.Page, m.Board.Mode, m.Board.Lane = place.collection, place.page, place.mode, place.lane
	m.Focus, m.hidden = place.focus, place.hidden
	m.listJump = place.focus != ColumnDetail && len(m.back) > 0
	m.pending = entry.uid
	m.follow = &followState{uid: entry.uid, restore: true}
	return m.collectionChosen()
}

// backAcross is Esc with the list focused after a follow landed on a card: back through the jump history.
func (m Model) backAcross() (Model, []Cmd, bool) {
	if !m.EscGoesBack() {
		return m, nil, false
	}
	return m.jumpBack()
}

// EscGoesBack says Esc would return across Projects: a follow landed on this card or row and nothing has moved since.
func (m Model) EscGoesBack() bool {
	return m.listJump && len(m.back) > 0 && m.Focus != ColumnDetail && m.SelectedUID == m.landedOn
}

// followArrived says the Collection's snapshot is on screen.
func (m Model) followArrived() Model {
	if m.follow != nil {
		follow := *m.follow
		follow.snapshot = true
		m.follow = &follow
	}
	return m.landFollow()
}

// waitingForFollow says the Project followed into has not answered yet, so the selection must not be tidied.
func (m Model) waitingForFollow() bool { return m.follow != nil && !m.follow.snapshot }

// landFollow settles a followed ticket once the Project's snapshot has arrived and, on the Board, its Lane list: the Board puts
// its card on screen, or the ticket opens in the detail beside the Lane view when the Board does not draw it.
func (m Model) landFollow() Model {
	follow := m.follow
	if follow == nil || !follow.snapshot || (m.OnBoard() && !m.Board.loaded) {
		return m
	}
	m.follow = nil
	if !m.OnBoard() {
		return m
	}
	m.pending = ""
	if follow.restore {
		m = m.keepLane()
		if m.Focus == ColumnDetail && m.OpenedIssueUID() != "" && m.SelectedUID == m.OpenedIssueUID() {
			return m
		}
		return m.WithVisibleSelection()
	}
	return m.placeCard(follow.uid, false)
}
