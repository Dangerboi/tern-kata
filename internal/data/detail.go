package data

import (
	"context"
	"fmt"
	"slices"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) ReadDetail(ctx context.Context, uid string) (model.DetailLoaded, error) {
	view, history := model.DetailDaemonView(), true
	response, err := c.ReadUISnapshotWithResponse(ctx, &g.ReadUISnapshotRequestOptions{
		Query: &g.ReadUISnapshotQuery{View: &view, SelectedIssueUID: &uid, IncludeHistory: &history},
	}, plainOverSocket)
	if err != nil {
		return model.DetailLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil || response.JSON200.Selected == nil {
		return model.DetailLoaded{}, fmt.Errorf("kata daemon returned no detail for Issue %s", uid)
	}
	return model.DetailLoaded{UID: uid, Selected: translateDetail(response.JSON200.Selected, response.JSON200.Capabilities.Scope != nil)}, nil
}

// translateDetail is the one place kata's selected authority becomes the model's Issue detail; nil stays nil.
//
// A scoped access gets no recurrences at all while the Issue stays visible, so for it the Recurrence stays nil ("not provided"),
// not stopped.
func translateDetail(s *g.UISelectedAuthority, scoped bool) *model.IssueDetailData {
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
			UID: c.UID, Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt,
			Attribution: translateAttribution(c.Verification, c.AccountableActor, c.SourceActor, c.Teammate),
			ReplyKind:   c.ReplyKind, Reply: translateCommentLink(c.Reply),
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
		d.Links = append(d.Links, translateLink(l))
	}
	if d.Issue != nil && d.Issue.RecurrenceID != nil && !scoped {
		d.Recurrence = translateRecurrence(*d.Issue.RecurrenceID, s.Recurrences)
	}
	return d
}

// translateRecurrence picks the Issue's recurrence out of the Project's live ones. kata omits a deleted recurrence, so an Issue
// whose recurrence is not there reads as stopped.
func translateRecurrence(id int64, live []g.Recurrence) *model.Recurrence {
	i := slices.IndexFunc(live, func(r g.Recurrence) bool { return r.ID == id })
	if i < 0 {
		return &model.Recurrence{Stopped: true}
	}
	out := translateLiveRecurrence(live[i])
	return &out
}

func translateLiveRecurrence(r g.Recurrence) model.Recurrence {
	out := model.Recurrence{UID: r.UID, RecurrenceTerms: model.RecurrenceTerms{Rule: r.Rrule, Start: r.Dtstart, TimeZone: r.Timezone}, Revision: r.Revision}
	if r.NextOccurrenceKey != nil {
		out.Next = *r.NextOccurrenceKey
	}
	return out
}

func translateLink(l g.UILink) model.Link {
	return model.Link{
		ID: l.ID, Type: l.Type, FromIssueUID: l.FromIssueUID, FromQualifiedID: l.FromQualifiedID, FromStatus: l.FromStatus,
		ToIssueUID: l.ToIssueUID, ToQualifiedID: l.ToQualifiedID, ToStatus: l.ToStatus,
	}
}

func translateCommentLink(l *g.CommentLink) *model.CommentLink {
	if l == nil {
		return nil
	}
	return &model.CommentLink{
		UID: l.UID, Kind: l.Kind, Author: l.Author, IssueUID: l.IssueUID, IssueShortID: l.IssueShortID, ProjectUID: l.ProjectUID,
	}
}

// translateAttribution carries kata's creation attribution fields; a missing verification state stays empty.
func translateAttribution[V ~string](verification *V, accountable, source, teammate *string) model.Attribution {
	var a model.Attribution
	if verification != nil {
		a.Verification = string(*verification)
	}
	for dst, src := range map[*string]*string{&a.Accountable: accountable, &a.Source: source, &a.Teammate: teammate} {
		if src != nil {
			*dst = *src
		}
	}
	return a
}
