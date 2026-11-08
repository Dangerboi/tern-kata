package model

import "slices"

const (
	NextCommentLink Action = "next-comment-link"
	PrevCommentLink Action = "previous-comment-link"
)

type commentLink struct {
	issueUID, commentUID, projectUID, label string
	qualified                               string
}

// CommentLinkChoice is the link chosen on the comment under the cursor: its place among the comment's links (1-based), their
// count and what the chosen one is. IssueUID and Qualified name the Issue a link into another Issue opens ("" for a link in
// this Issue), Qualified being its "#short" id when kata gives it.
type CommentLinkChoice struct {
	Index, Count        int
	Label               string
	IssueUID, Qualified string
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
		qualified := ""
		if issue != m.DetailIssueUID() && l.IssueShortID != nil {
			qualified = "#" + *l.IssueShortID
			label += " in " + qualified
		}
		project := ""
		if l.ProjectUID != nil {
			project = *l.ProjectUID
		}
		links = append(links, commentLink{issue, *l.UID, project, label, qualified})
	}
	if comment.Reply != nil && comment.ReplyKind != nil {
		add(*comment.Reply, replyVerb(*comment.ReplyKind)+" "+answeredComment(comment.Reply.Author))
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

func answeredComment(author *string) string {
	if author == nil {
		return "a comment"
	}
	return *author + "'s comment"
}

func replyVerb(kind string) string {
	if verb := map[string]string{"confirm": "confirms", "refute": "refutes", "supersede": "supersedes"}[kind]; verb != "" {
		return verb
	}
	return "replies to"
}

func wrapIndex(i, n int) int { return (i%n + n) % n }

// chosenLink is the index of the link the cursor comment has chosen among n links: the first until `]` or `[` stepped.
func (m Model) chosenLink(row rowID, n int) int {
	if n == 0 || m.linkOn != row {
		return 0
	}
	return wrapIndex(m.link, n)
}

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
	i := m.chosenLink(row.id(), len(links))
	choice := CommentLinkChoice{Index: i + 1, Count: len(links), Label: links[i].label}
	if links[i].issueUID != m.DetailIssueUID() {
		choice.IssueUID, choice.Qualified = links[i].issueUID, links[i].qualified
	}
	return choice, true
}

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
		id := row.id()
		i := m.chosenLink(id, n)
		if forward {
			i++
		} else {
			i--
		}
		m.linkOn, m.link = id, wrapIndex(i, n)
		return m
	}
}

func (m Model) openCommentLink(row DetailItem) (Model, []Cmd, bool) {
	links := m.commentLinks(row.Key)
	if len(links) == 0 {
		return m, nil, false
	}
	link := links[m.chosenLink(row.id(), len(links))]
	if link.issueUID == m.DetailIssueUID() {
		if i := slices.IndexFunc(m.rows.items, func(r DetailItem) bool { return r.Kind == RowComment && r.Key == link.commentUID }); i >= 0 {
			m.cursor = i
		}
		return m, nil, true
	}
	landing := commentLanding(link.commentUID)
	if m.Collection.IsProject() && link.projectUID != "" && link.projectUID != m.Collection.ProjectUID {
		if i := slices.IndexFunc(m.Snapshot.Catalog, func(c CatalogProject) bool { return c.Project.UID == link.projectUID }); i >= 0 {
			m, cmds := m.crossJump(link.issueUID, m.Snapshot.Catalog[i], landing)
			return m, cmds, true
		}
	}
	m, cmds := m.jumpTo(link.issueUID, landing)
	return m, cmds, true
}
