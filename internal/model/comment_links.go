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
	// A link names its target, never the relation: the relation is already in the comment's head ("replying to dana", "1 reply").
	if comment.Reply != nil {
		label := "a comment"
		if comment.Reply.Author != nil {
			label = *comment.Reply.Author + "'s comment"
		}
		add(*comment.Reply, label)
	}
	for _, back := range comment.Backlinks {
		add(back, replyOf(back.Author, back.Kind))
	}
	return links
}

// replyOf names a comment that answers this one: "dana's reply", "dana's confirmation".
func replyOf(author *string, kind string) string {
	noun := map[string]string{"confirm": "confirmation", "refute": "refutation", "supersede": "supersession"}[kind]
	if noun == "" {
		noun = "reply"
	}
	if author == nil {
		return "a " + noun
	}
	return *author + "'s " + noun
}

// answeredAuthor names the author of the comment a reply answers; a target with no author is "a comment".
func answeredAuthor(author *string) string {
	if author == nil {
		return "a comment"
	}
	return *author
}

// replyingTo is the reply kind as a participle for a reply's own head: "replying to dana", "confirming dana".
func replyingTo(kind string) string {
	if verb := map[string]string{"confirm": "confirming", "refute": "refuting", "supersede": "superseding"}[kind]; verb != "" {
		return verb
	}
	return "replying to"
}

func wrapIndex(i, n int) int { return (i%n + n) % n }

// chosenLink is the index of the link the cursor comment has chosen among n links: the first until `]` or `[` stepped.
func (m Model) chosenLink(row rowID, n int) int {
	if n == 0 || m.linkOn != row {
		return 0
	}
	return wrapIndex(m.link, n)
}

// CommentLinkLabels are the labels of the links of the comment with the uid, in the order ] and [ step them; none for a comment
// without links.
func (m Model) CommentLinkLabels(uid string) []string {
	links := m.commentLinks(uid)
	labels := make([]string, len(links))
	for i, l := range links {
		labels[i] = l.label
	}
	return labels
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
