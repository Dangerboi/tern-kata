package model

import "slices"

const (
	NextCommentLink Action = "next-comment-link"
	PrevCommentLink Action = "previous-comment-link"
)

// commentLink is one end of a Reply a comment can open: the comment it answers or a comment answering it.
type commentLink struct {
	issueUID, commentUID, projectUID, label string
}

// CommentLinkChoice is the link chosen on the comment under the cursor: its place among the comment's links (1-based), their
// count and what the chosen one is.
type CommentLinkChoice struct {
	Index, Count int
	Label        string
}

// commentLinks are the links of the comment with the uid: the comment it answers (if any), then its backlinks, as kata returned
// them with the detail. A link without a comment to open is not one.
func (m Model) commentLinks(uid string) []commentLink {
	if m.authority == nil {
		return nil
	}
	i := slices.IndexFunc(m.authority.Comments, func(c Comment) bool { return c.UID == uid })
	if i < 0 {
		return nil
	}
	comment := m.authority.Comments[i]
	var links []commentLink
	add := func(l CommentLink, label string) {
		if l.UID == nil || *l.UID == "" {
			return
		}
		issue := m.DetailIssueUID()
		if l.IssueUID != nil && *l.IssueUID != "" {
			issue = *l.IssueUID
		}
		if issue != m.DetailIssueUID() && l.IssueShortID != nil {
			label += " in #" + *l.IssueShortID
		}
		project := ""
		if l.ProjectUID != nil {
			project = *l.ProjectUID
		}
		links = append(links, commentLink{issue, *l.UID, project, label})
	}
	if comment.Reply != nil && comment.ReplyKind != nil {
		whose := "a comment"
		if comment.Reply.Author != nil {
			whose = *comment.Reply.Author + "'s comment"
		}
		add(*comment.Reply, replyVerb(*comment.ReplyKind)+" "+whose)
	} else if comment.Reply != nil {
		add(*comment.Reply, "replies to a comment")
	}
	for _, back := range comment.Backlinks {
		who := "a comment"
		if back.Author != nil {
			who = *back.Author
		}
		add(back, who+" "+replyVerb(back.Kind))
	}
	return links
}

func replyVerb(kind string) string {
	if verb := map[string]string{"confirm": "confirms", "refute": "refutes", "supersede": "supersedes"}[kind]; verb != "" {
		return verb
	}
	return "replies to"
}

// chosenLink is the index of the link the cursor comment has chosen among n links: the first until `]` or `[` stepped.
func (m Model) chosenLink(row rowID, n int) int {
	if n == 0 || m.linkOn != row {
		return 0
	}
	return ((m.link % n) + n) % n
}

// DetailCommentLink is the link chosen on the comment under the cursor, false when it has none.
func (m Model) DetailCommentLink() (CommentLinkChoice, bool) {
	if m.tab != TabComments {
		return CommentLinkChoice{}, false
	}
	row, ok := m.withRows().cursorRow()
	if !ok || row.Kind != RowComment {
		return CommentLinkChoice{}, false
	}
	links := m.commentLinks(row.Key)
	if len(links) == 0 {
		return CommentLinkChoice{}, false
	}
	i := m.chosenLink(rowID{row.Kind, row.Key}, len(links))
	return CommentLinkChoice{Index: i + 1, Count: len(links), Label: links[i].label}, true
}

// stepCommentLink is `]` (forward) and `[`: the cursor comment's link cursor moves one link, wrapping around.
func stepCommentLink(forward bool) func(Model) Model {
	return func(m Model) Model {
		if m.tab != TabComments || m.Focus != ColumnDetail {
			return m
		}
		row, ok := m.withRows().cursorRow()
		if !ok || row.Kind != RowComment {
			return m
		}
		n := len(m.commentLinks(row.Key))
		if n == 0 {
			return m
		}
		id := rowID{row.Kind, row.Key}
		i := m.chosenLink(id, n)
		if forward {
			i++
		} else {
			i--
		}
		m.linkOn, m.link = id, ((i%n)+n)%n
		return m
	}
}

// openCommentLink is enter on a comment with links: the chosen link's comment, here or in another Issue or Project.
func (m Model) openCommentLink(row DetailItem) (Model, []Cmd, bool) {
	links := m.commentLinks(row.Key)
	if len(links) == 0 {
		return m, nil, false
	}
	link := links[m.chosenLink(rowID{row.Kind, row.Key}, len(links))]
	if link.issueUID == m.DetailIssueUID() {
		if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return r.Kind == RowComment && r.Key == link.commentUID }); i >= 0 {
			m.cursor = i
		}
		return m, nil, true
	}
	landing := &jumpEntry{tab: TabComments, row: rowID{RowComment, link.commentUID}, hasRow: true}
	if m.Collection.IsProject() && link.projectUID != "" && link.projectUID != m.Collection.ProjectUID {
		if i := slices.IndexFunc(m.Snapshot.Catalog, func(c CatalogProject) bool { return c.Project.UID == link.projectUID }); i >= 0 {
			m, cmds := m.crossJump(link.issueUID, m.Snapshot.Catalog[i], landing)
			return m, cmds, true
		}
	}
	m, cmds := m.jumpTo(link.issueUID, landing)
	return m, cmds, true
}
