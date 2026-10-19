package model

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// RowKind says what a row of the Issue detail is, so the view can draw it and a click or enter knows what it opens.
type RowKind uint8

const (
	RowChild RowKind = iota
	RowComment
	RowLink
	RowEvent
	RowAttachment
)

// DetailItem is one row of the Issue detail's cursor list, shaped for the view. A RowChild row carries Child; every other
// row carries Activity. Key is unique within the list, and ReferenceUID is the Issue the row opens ("" when it opens none).
// ReplyKey is the key of the comment in this Issue that a reply row answers, which enter moves the cursor to.
type DetailItem struct {
	Kind         RowKind
	Key          string
	ReferenceUID string
	ReplyKey     string
	// ReferenceKey is the comment in the ReferenceUID Issue that a cross-Issue reply answers: the row a jump lands on.
	ReferenceKey string
	Child        ChildRow
	Activity     DetailRow
}

// tabKinds is the row kind each Activity tab lists.
var tabKinds = [TabCount]RowKind{TabComments: RowComment, TabLinks: RowLink, TabEvents: RowEvent, TabAttachments: RowAttachment}

func (t ActivityTab) kind() RowKind { return tabKinds[t] }

// Tab is the Activity tab that lists rows of the kind; a child row belongs to none and answers the first tab.
func (k RowKind) Tab() ActivityTab { return ActivityTab(max(slices.Index(tabKinds[:], k), 0)) }

// ChildRow is one row of the Children table. Key is the Link's, which is daemon-wide unique, so a
// row's id never ends in the child's UID as the list row's does. A child that is not loaded has only its UID, Key and Ref.
type ChildRow struct {
	Key, UID, Ref, Title, Status string
	Owner                        *string
	OwnerExpires                 *time.Time
	UpdatedAt                    time.Time
	Loaded, Closed               bool
}

// rowID names a row across refreshes.
type rowID struct {
	kind RowKind
	key  string
}

// rowInputs are what the rows are worked out from; the rows are rebuilt only when one of them changes.
type rowInputs struct {
	authority  *IssueDetailData
	firstIssue *Issue // with issueN, tells when the loaded Issues changed
	issueN     int
	tab        ActivityTab
	calendar   PaneCalendar
	host       string
}

// detailRows is the Issue detail's cursor list: the children, then the active tab's rows.
type detailRows struct {
	inputs rowInputs
	items  []DetailItem
	parent *DetailRow
}

// DetailRows is the Issue detail's rows: the children first, then the active tab's comments, links or events. The cursor
// (DetailIndex) is an index into it; cursor movement, reveal and clicks all go through this list.
func (m Model) DetailRows() []DetailItem { return m.rows.items }

// DetailParent is the detail Issue's parent link, when it has one.
func (m Model) DetailParent() (DetailRow, bool) {
	if m.rows.parent == nil {
		return DetailRow{}, false
	}
	return *m.rows.parent, true
}

// DetailChildCounts is the shown Issue's direct children, counted from its own detail links, so an Issue outside the Collection has them too.
func (m Model) DetailChildCounts() ChildCounts {
	if m.authority == nil || m.authority.Issue == nil {
		return ChildCounts{}
	}
	return CountChildren(m.authority.Links, m.authority.Issue.UID)
}

// withRows brings the rows up to date with the authority, the loaded Issues and the active tab.
func (m Model) withRows() Model {
	in := rowInputs{authority: m.authority, issueN: len(m.Issues), tab: m.tab, calendar: m.Calendar, host: m.Host}
	if len(m.Issues) > 0 {
		in.firstIssue = &m.Issues[0]
	}
	if in == m.rows.inputs {
		return m
	}
	m.rows = detailRows{inputs: in}
	if m.authority == nil {
		return m
	}
	links := make([]DetailRow, len(m.authority.Links))
	for i, link := range m.authority.Links {
		links[i] = m.linkRow(link)
	}
	m.rows.items = m.childItems(links)
	for i := range links {
		if links[i].Relation == "parent" {
			m.rows.parent = &links[i]
			break
		}
	}
	switch m.tab {
	case TabLinks:
		for _, link := range links {
			m.rows.items = append(m.rows.items, DetailItem{Kind: RowLink, Key: link.Key, ReferenceUID: link.ReferenceUID, Activity: link})
		}
	case TabAttachments:
		if m.authority.Issue != nil {
			for _, a := range Attachments(m.authority.Issue.Metadata) {
				row := DetailRow{Key: AttachmentRowKey(a.Name), File: a}
				if m.Host != "" && SameHost(a.Host, m.Host) {
					row.URL = fileURL(a.Path)
				}
				m.rows.items = append(m.rows.items, DetailItem{Kind: RowAttachment, Key: row.Key, Activity: row})
			}
		}
	case TabComments:
		for _, comment := range m.authority.Comments {
			item := m.commentItem(comment)
			m.rows.items = append(m.rows.items, item)
		}
	case TabEvents:
		// The snapshot lists History newest first; rows read oldest first, like Comments.
		for _, event := range slices.Backward(m.authority.History) {
			row := m.historyRow(event)
			m.rows.items = append(m.rows.items, DetailItem{Kind: RowEvent, Key: row.Key, ReferenceUID: row.ReferenceUID, Activity: row})
		}
	}
	return m
}

// commentItem is a comment row; a reply opens the comment it answers, here or in another Issue.
func (m Model) commentItem(comment Comment) DetailItem {
	row := DetailRow{Key: comment.UID, Author: comment.Author, Time: m.DetailTime(comment.CreatedAt), Body: comment.Body}
	item := DetailItem{Kind: RowComment, Key: row.Key}
	if n := len(comment.Backlinks); n == 1 {
		row.Replies = "1 reply"
	} else if n > 1 {
		row.Replies = fmt.Sprintf("%d replies", n)
	}
	if target := comment.Reply; target != nil && comment.ReplyKind != nil {
		verb := map[string]string{"confirm": "confirms", "refute": "refutes", "supersede": "supersedes"}[*comment.ReplyKind]
		if verb == "" {
			verb = "replies to"
		}
		whose := "a comment"
		if target.Author != nil {
			whose = *target.Author + "'s comment"
		}
		row.Reply = verb + " " + whose
		switch {
		case target.IssueUID != nil && *target.IssueUID != m.DetailIssueUID():
			if target.IssueShortID != nil {
				row.Reply += " in #" + *target.IssueShortID
			}
			item.ReferenceUID = *target.IssueUID
			if target.UID != nil {
				item.ReferenceKey = *target.UID
			}
		case target.UID != nil && slices.ContainsFunc(m.authority.Comments, func(c Comment) bool { return c.UID == *target.UID }):
			item.ReplyKey = *target.UID
		}
	}
	row.Head = []RowSpan{{row.Author, SpanStrong}, {"  ", ""}, {row.Time, SpanMuted}}
	for _, note := range []string{row.Reply, row.Replies} {
		if note != "" {
			row.Head = append(row.Head, RowSpan{"  " + note, SpanAccent})
		}
	}
	item.Activity = row
	return item
}

func (m Model) linkRow(link Link) DetailRow {
	uid, ref := link.ToIssueUID, link.ToQualifiedID
	relation := link.Type
	if uid == m.DetailIssueUID() {
		uid, ref = link.FromIssueUID, link.FromQualifiedID
		switch relation {
		case "parent":
			relation = "child"
		case "child":
			relation = "parent"
		case "blocks":
			relation = "blocked_by"
		case "blocked_by":
			relation = "blocks"
		}
	}
	if project, short, ok := strings.Cut(ref, "#"); ok && m.authority.Issue != nil && project == m.authority.Issue.ProjectName {
		ref = "#" + short
	}
	row := DetailRow{Key: fmt.Sprint(link.ID), Relation: relation, Ref: ref, ReferenceUID: uid}
	if i := slices.IndexFunc(m.Issues, func(issue Issue) bool { return issue.UID == uid }); i >= 0 {
		row.RefTitle = m.Issues[i].Title
	}
	return row
}

// childItems are the detail Issue's children: the loaded ones in the list's order, then the ones not loaded in the Link
// order. Titles, statuses and times come from the loaded Issues.
func (m Model) childItems(links []DetailRow) []DetailItem {
	byUID := make(map[string]DetailRow)
	var uids []string
	for _, link := range links {
		if link.Relation == "child" {
			byUID[link.ReferenceUID] = link
			uids = append(uids, link.ReferenceUID)
		}
	}
	items := make([]DetailItem, 0, len(uids))
	add := func(child ChildRow) {
		items = append(items, DetailItem{Kind: RowChild, Key: child.Key, ReferenceUID: child.UID, Child: child})
		delete(byUID, child.UID)
	}
	for _, issue := range m.Issues {
		if link, ok := byUID[issue.UID]; ok {
			add(ChildRow{Key: link.Key, UID: issue.UID, Ref: link.Ref, Title: issue.Title, Status: issue.Status, Owner: issue.Owner, OwnerExpires: issue.AssignmentExpiresOn, UpdatedAt: issue.UpdatedAt, Loaded: true, Closed: IsClosed(issue)})
		}
	}
	for _, uid := range uids {
		if link, ok := byUID[uid]; ok {
			add(ChildRow{Key: link.Key, UID: uid, Ref: link.Ref})
		}
	}
	return items
}

// cursorRow is the row under the cursor.
func (m Model) cursorRow() (DetailItem, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows.items) {
		return DetailItem{}, false
	}
	return m.rows.items[m.cursor], true
}

func (m Model) cursorRowID() (rowID, bool) {
	row, ok := m.withRows().cursorRow()
	return rowID{row.Kind, row.Key}, ok
}

// reanchor keeps the cursor on the row it was on (anchor) after the rows changed, else on the nearest row left.
func (m Model) reanchor(anchor rowID, anchored bool) Model {
	m = m.withRows()
	if anchored {
		if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return r.Kind == anchor.kind && r.Key == anchor.key }); i >= 0 {
			m.cursor = i
			return m
		}
	}
	m.cursor = min(m.cursor, max(len(m.rows.items)-1, 0))
	return m
}

// firstActivityIndex is the cursor index of the active tab's first row (past the children).
func (m Model) firstActivityIndex() int {
	if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return r.Kind != RowChild }); i >= 0 {
		return i
	}
	return len(m.rows.items)
}
