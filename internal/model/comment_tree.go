package model

import (
	"cmp"
	"slices"
	"strings"
)

// maxCommentDepth is how far a reply nests under the comment it answers; deeper replies stay at this depth, as in kata's TUI.
const maxCommentDepth = 4

// threadedComment is a comment placed in the Comments tab's reply tree. Leaves are the replies from other Issues, which have
// no row of their own here.
type threadedComment struct {
	Comment
	depth  int
	leaves []commentLeaf
}

type commentLeaf struct {
	link  CommentLink
	depth int
}

func compareCommentLinks(a, b CommentLink) int {
	return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(deref(a.UID), deref(b.UID)))
}

// deref reads a value kata leaves out as empty. It is cronjobclient.Str for the model, which stays clear of the client layer.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// threadOrder lays the comments out as kata's TUI does: each reply under the comment it answers. Replies from other Issues are
// leaves, drawn with the parent row, and the local replies follow it; each group goes by creation time then UID. Roots keep
// their input order and a cycle is visited once.
func threadOrder(cs []Comment) []threadedComment {
	byUID := make(map[string]int, len(cs))
	children := make(map[string][]CommentLink, len(cs))
	for i, c := range cs {
		if c.UID != "" {
			byUID[c.UID] = i
		}
	}
	for _, c := range cs {
		if c.UID == "" {
			continue
		}
		for _, link := range c.Backlinks {
			if _, local := byUID[deref(link.UID)]; !local {
				children[c.UID] = append(children[c.UID], link)
			}
		}
	}
	var roots []int
	for i, c := range cs {
		parent, visible := "", false
		if c.Reply != nil && c.Reply.UID != nil && *c.Reply.UID != "" {
			parent = *c.Reply.UID
			_, visible = byUID[parent]
		}
		if visible && c.UID != "" {
			uid := c.UID
			children[parent] = append(children[parent], CommentLink{UID: &uid, CreatedAt: c.CreatedAt, Kind: c.Reply.Kind, TargetEdited: c.Reply.TargetEdited})
		} else {
			roots = append(roots, i)
		}
	}
	for _, links := range children {
		slices.SortStableFunc(links, compareCommentLinks)
	}
	ordered := make([]threadedComment, 0, len(cs))
	visited := make([]bool, len(cs))
	var visit func(i, depth int)
	visit = func(i, depth int) {
		if visited[i] {
			return
		}
		visited[i] = true
		at := len(ordered)
		ordered = append(ordered, threadedComment{Comment: cs[i], depth: depth})
		if cs[i].UID == "" {
			return
		}
		for _, link := range children[cs[i].UID] {
			childDepth := min(depth+1, maxCommentDepth)
			if child, local := byUID[deref(link.UID)]; local {
				visit(child, childDepth)
			} else {
				ordered[at].leaves = append(ordered[at].leaves, commentLeaf{link, childDepth})
			}
		}
	}
	for _, root := range roots {
		visit(root, 0)
	}
	for i := range cs {
		visit(i, 0)
	}
	return ordered
}

// CommentLeaf is a reply from another Issue, drawn under the comment it answers: Text is "↳ <kind> <header>" and Edited says
// its target changed after the reply.
type CommentLeaf struct {
	Depth  int
	Text   string
	Edited bool
}

func (m Model) commentLeafRow(l commentLeaf) CommentLeaf {
	actor := deref(l.link.Author)
	if t := deref(l.link.Teammate); t != "" {
		actor += " / " + t
	}
	parts := []string{"↳", l.link.Kind}
	if h := deref(l.link.Handle); h != "" {
		parts = append(parts, h)
	}
	if l.link.IssueShortID != nil {
		parts = append(parts, "in #"+*l.link.IssueShortID)
	}
	if actor != "" {
		parts = append(parts, "by "+actor)
	}
	if !l.link.CreatedAt.IsZero() {
		parts = append(parts, m.DetailTime(l.link.CreatedAt))
	}
	if l.link.EditedAt != nil {
		parts = append(parts, "(edited)")
	}
	if s := deref(l.link.Status); s != "" {
		parts = append(parts, "("+s+")")
	}
	return CommentLeaf{Depth: l.depth, Text: strings.Join(parts, " "), Edited: l.link.TargetEdited}
}

// commentTags are the confirm and refute tags of a comment's replies: a kind with only evidence that predates an edit of the
// comment reads "(before edit)".
func commentTags(links []CommentLink) []RowSpan {
	var tags []RowSpan
	for _, kind := range []string{"confirm", "refute"} {
		present, fresh := false, false
		for _, l := range links {
			if l.Kind == kind {
				present = true
				fresh = fresh || !l.TargetEdited
			}
		}
		if !present {
			continue
		}
		tag, style := kind, SpanStyle("success")
		if kind == "refute" {
			style = "error"
		}
		if !fresh {
			tag += " (before edit)"
		}
		tags = append(tags, RowSpan{tag, style})
	}
	return tags
}
