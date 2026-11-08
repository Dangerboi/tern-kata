package model

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type RowKind uint8

const (
	RowChild RowKind = iota
	RowComment
	RowLink
	RowEvent
	RowAttachment
	RowCron
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
	// Depth is how far a comment row nests under the comment it answers (0 for a root), and Leaves are the replies from other
	// Issues drawn under it.
	Depth  int
	Leaves []CommentLeaf
	Child  ChildRow
	// JobUID and Cron are a RowCron row's job and what the property says of it; enter opens the job on the Jobs Page.
	JobUID   string
	Cron     CronRow
	Activity DetailRow
}

func (i DetailItem) id() rowID { return rowID{i.Kind, i.Key} }

func commentLanding(key string) *jumpEntry {
	return &jumpEntry{tab: TabComments, row: rowID{RowComment, key}, hasRow: true}
}

func (t ActivityTab) kind() RowKind { return tabTable[t].kind }

// InActivity says rows of the kind belong to an Activity tab; a child row and a Cron job row do not.
func (k RowKind) InActivity() bool { return k != RowChild && k != RowCron }

// Tab is the Activity tab that lists rows of the kind; a child row belongs to none and answers the first tab.
func (k RowKind) Tab() ActivityTab {
	return ActivityTab(max(slices.IndexFunc(tabTable[:], func(e tabSpec) bool { //nolint:gosec // G115: an index into the few tabs
		return e.kind == k
	}), 0))
}

// ChildRow is one row of the Children table. Key is the Link's, which is daemon-wide unique, so a
// row's id never ends in the child's UID as the list row's does. A child that is not loaded has only its UID, Key and Ref.
type ChildRow struct {
	Key, UID, Ref, Title, Status string
	Qualified                    string
	Owner                        *string
	OwnerExpires                 *time.Time
	UpdatedAt                    time.Time
	Loaded, Closed               bool
}

type rowID struct {
	kind RowKind
	key  string
}

type rowInputs struct {
	authority                         *IssueDetailData
	firstIssue                        *Issue // with issueN, tells when the loaded Issues changed
	issueN                            int
	tab                               ActivityTab
	calendar                          PaneCalendar
	jobsProject, jobsRequest, jobsRev int64
	jobsLoading                       bool
}

type detailRows struct {
	inputs rowInputs
	items  []DetailItem
	parent *DetailRow
}

// DetailRows is the Issue detail's rows: the children first, then the active tab's comments, links or events. The cursor
// (DetailIndex) is an index into it; cursor movement, reveal and clicks all go through this list.
func (m Model) DetailRows() []DetailItem { return m.rows.items }

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

func (m Model) withRows() Model {
	in := rowInputs{authority: m.authority, firstIssue: first(m.Issues), issueN: len(m.Issues), tab: m.tab, calendar: m.Calendar,
		jobsProject: m.Jobs.ProjectID, jobsRequest: m.Jobs.reads.Newest(), jobsRev: m.Jobs.rev, jobsLoading: m.Jobs.Loading}
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
	m.rows.items = append(m.cronItems(), m.childItems(links)...)
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
				row := DetailRow{Key: AttachmentRowKey(a.ID), File: a}
				m.rows.items = append(m.rows.items, DetailItem{Kind: RowAttachment, Key: row.Key, Activity: row})
			}
		}
	case TabComments:
		for _, comment := range threadOrder(m.authority.Comments) {
			item := m.commentItem(comment.Comment)
			item.Depth = comment.depth
			for _, leaf := range comment.leaves {
				item.Leaves = append(item.Leaves, m.commentLeafRow(leaf))
			}
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

func (m Model) commentItem(comment Comment) DetailItem {
	row := DetailRow{Key: comment.UID, Author: comment.Author, Time: m.DetailTime(comment.CreatedAt), Body: comment.Body,
		Creation: comment.Attribution.Line(comment.Author)}
	item := DetailItem{Kind: RowComment, Key: row.Key}
	if n := len(comment.Backlinks); n == 1 {
		row.Replies = "1 reply"
	} else if n > 1 {
		row.Replies = fmt.Sprintf("%d replies", n)
	}
	if target := comment.Reply; target != nil && comment.ReplyKind != nil {
		row.Reply = replyVerb(*comment.ReplyKind) + " " + answeredComment(target.Author)
		switch {
		case target.IssueUID != nil && *target.IssueUID != m.DetailIssueUID():
			if target.IssueShortID != nil {
				row.Qualified = "#" + *target.IssueShortID
				row.Reply += " in " + row.Qualified
			}
			item.ReferenceUID = *target.IssueUID
			if target.UID != nil {
				item.ReferenceKey = *target.UID
			}
		case target.UID != nil && slices.ContainsFunc(m.authority.Comments, func(c Comment) bool { return c.UID == *target.UID }):
			item.ReplyKey = *target.UID
		}
		if target.TargetEdited {
			row.Reply += " — Target edited after this reply"
		}
	}
	row.Tags = commentTags(comment.Backlinks)
	row.Head = []RowSpan{{row.Author, SpanStrong}, {"  ", ""}, {row.Time, SpanTime}}
	for _, note := range []string{row.Reply, row.Replies} {
		if note != "" {
			row.Head = append(row.Head, RowSpan{"  " + note, SpanAccent})
		}
	}
	item.Activity = row
	return item
}

func (m Model) linkRow(link Link) DetailRow {
	uid, ref, status := link.ToIssueUID, link.ToQualifiedID, link.ToStatus
	qualified := ref
	relation := link.Type
	if uid == m.DetailIssueUID() {
		uid, ref, status = link.FromIssueUID, link.FromQualifiedID, link.FromStatus
		qualified = ref
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
	row := DetailRow{Key: fmt.Sprint(link.ID), Relation: relation, Ref: ref, ReferenceUID: uid, Qualified: qualified, Status: status}
	if i := slices.IndexFunc(m.Issues, func(issue Issue) bool { return issue.UID == uid }); i >= 0 {
		row.RefTitle = m.Issues[i].Title
	}
	return row
}

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
			add(ChildRow{Key: link.Key, UID: issue.UID, Ref: link.Ref, Qualified: link.Qualified, Title: issue.Title, Status: issue.Status, Owner: issue.Owner, OwnerExpires: issue.AssignmentExpiresOn, UpdatedAt: issue.UpdatedAt, Loaded: true, Closed: IsClosed(issue)})
		}
	}
	for _, uid := range uids {
		if link, ok := byUID[uid]; ok {
			add(ChildRow{Key: link.Key, UID: uid, Ref: link.Ref, Qualified: link.Qualified, Status: link.Status})
		}
	}
	return items
}

func (m Model) cursorRow() (DetailItem, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows.items) {
		return DetailItem{}, false
	}
	return m.rows.items[m.cursor], true
}

func (m Model) cursorRowID() (rowID, bool) {
	row, ok := m.withRows().cursorRow()
	return row.id(), ok
}

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

func (m Model) firstActivityIndex() int {
	if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return r.Kind.InActivity() }); i >= 0 {
		return i
	}
	return len(m.rows.items)
}
