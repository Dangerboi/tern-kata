package model

import "time"

// IssueDetailData is what the Issue detail shows of one Issue: the Issue, its comments, history (newest first) and links.
// The Daemon gateway translates it from kata's wire shapes and keeps the field names and types of the fields tern-kata uses.
type IssueDetailData struct {
	Issue    *Issue
	Comments []Comment
	History  []Event
	Links    []Link
}

// Comment is one comment; ReplyKind and ReplyToUID are set when it replies to another, Reply is the answered comment and
// Backlinks are the comments replying to this one.
type Comment struct {
	UID        string
	Handle     *string
	Author     string
	Body       string
	CreatedAt  time.Time
	ReplyKind  *string
	ReplyToUID *string
	Reply      *CommentLink
	Backlinks  []CommentLink
}

// CommentLink is a comment on the other end of a reply; Kind is the reply's kind and Status says why the comment is
// unavailable ("pending", "hidden", "moved", "removed").
type CommentLink struct {
	UID          *string
	Handle       *string
	Kind         string
	Author       *string
	Body         *string
	IssueUID     *string
	IssueShortID *string
	Status       *string
	TargetEdited *bool
}

// Event is one history event; Payload is kata's JSON text of it, which historyRow turns into a sentence.
type Event struct {
	UID                 string
	Type                string
	Actor               string
	CreatedAt           time.Time
	Payload             string
	ProjectName         string
	RelatedIssueUID     *string
	RelatedIssueShortID *string
}

// Link is one Link touching the Issue, with both ends.
type Link struct {
	ID              int64
	Type            string
	FromIssueUID    string
	FromQualifiedID string
	ToIssueUID      string
	ToQualifiedID   string
}
