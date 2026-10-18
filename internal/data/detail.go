package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) ReadDetail(ctx context.Context, uid string) (model.DetailLoaded, error) {
	view, history := "all-open", true
	response, err := c.ReadUISnapshotWithResponse(ctx, &g.ReadUISnapshotRequestOptions{
		Query: &g.ReadUISnapshotQuery{View: &view, SelectedIssueUID: &uid, IncludeHistory: &history},
	})
	if err != nil {
		return model.DetailLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil || response.JSON200.Selected == nil {
		return model.DetailLoaded{}, fmt.Errorf("kata daemon returned no detail for Issue %s", uid)
	}
	return model.DetailLoaded{UID: uid, Selected: translateDetail(response.JSON200.Selected)}, nil
}

// translateDetail is the one place kata's selected authority becomes the model's Issue detail; nil stays nil.
func translateDetail(s *g.UISelectedAuthority) *model.IssueDetailData {
	if s == nil {
		return nil
	}
	d := &model.IssueDetailData{}
	if s.Issue != nil {
		issue := translateIssue(s.Issue)
		d.Issue = &issue
	}
	for _, c := range s.Comments {
		comment := model.Comment{
			UID: c.UID, Handle: c.Handle, Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt,
			ReplyKind: c.ReplyKind, ReplyToUID: c.ReplyToUID, Reply: translateCommentLink(c.Reply),
		}
		for i := range c.Backlinks {
			comment.Backlinks = append(comment.Backlinks, *translateCommentLink(&c.Backlinks[i]))
		}
		d.Comments = append(d.Comments, comment)
	}
	for _, e := range s.History {
		d.History = append(d.History, model.Event{
			UID: e.UID, Type: e.Type, Actor: e.Actor, CreatedAt: e.CreatedAt, Payload: e.Payload, ProjectName: e.ProjectName,
			RelatedIssueUID: e.RelatedIssueUID, RelatedIssueShortID: e.RelatedIssueShortID,
		})
	}
	for _, l := range s.Links {
		d.Links = append(d.Links, model.Link{
			ID: l.ID, Type: l.Type, FromIssueUID: l.FromIssueUID, FromQualifiedID: l.FromQualifiedID,
			ToIssueUID: l.ToIssueUID, ToQualifiedID: l.ToQualifiedID,
		})
	}
	return d
}

func translateCommentLink(l *g.CommentLink) *model.CommentLink {
	if l == nil {
		return nil
	}
	return &model.CommentLink{
		UID: l.UID, Handle: l.Handle, Kind: l.Kind, Author: l.Author, Body: l.Body,
		IssueUID: l.IssueUID, IssueShortID: l.IssueShortID, Status: l.Status, TargetEdited: l.TargetEdited,
	}
}
