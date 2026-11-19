package data

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

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
	if response.JSON200.Selected == nil {
		return model.DetailLoaded{}, fmt.Errorf("kata daemon returned no detail for Issue %s", uid)
	}
	detail := translateDetail(response.JSON200.Selected, response.JSON200.Capabilities)
	c.linkTitles(ctx, detail)
	return model.DetailLoaded{UID: uid, Selected: detail}, nil
}

// linkTitleTTL is how long a title read for a Link's far end is trusted before it is read again.
const linkTitleTTL = 5 * time.Minute

type cachedTitle struct {
	title string
	at    time.Time
}

// linkTitles gives each Link of the detail the titles of its ends, so the Links tab names an Issue that is in another Project or
// not in the Pane's list. Titles come from a cache, and the misses are read from kata at once with a short deadline each; an Issue
// that cannot be read keeps no title and the row shows its id alone.
func (c *Client) linkTitles(ctx context.Context, d *model.IssueDetailData) {
	if d == nil || d.Issue == nil || len(d.Links) == 0 {
		return
	}
	titles := map[string]string{}
	var missing []string
	for _, l := range d.Links {
		for _, uid := range []string{l.FromIssueUID, l.ToIssueUID} {
			if uid == "" || uid == d.Issue.UID {
				continue
			}
			if _, seen := titles[uid]; seen || slices.Contains(missing, uid) {
				continue
			}
			if v, ok := c.titles.Load(uid); ok && time.Since(v.(cachedTitle).at) < linkTitleTTL {
				titles[uid] = v.(cachedTitle).title
				continue
			}
			missing = append(missing, uid)
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, uid := range missing {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readCtx, cancel := context.WithTimeout(ctx, linkTitleDeadline)
			defer cancel()
			r, err := c.ShowIssueByUIDWithResponse(readCtx, &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: uid}})
			if err != nil || r.JSON200 == nil {
				return
			}
			c.titles.Store(uid, cachedTitle{title: r.JSON200.Issue.Title, at: time.Now()})
			mu.Lock()
			titles[uid] = r.JSON200.Issue.Title
			mu.Unlock()
		}()
	}
	wg.Wait()
	for i := range d.Links {
		d.Links[i].FromTitle, d.Links[i].ToTitle = titles[d.Links[i].FromIssueUID], titles[d.Links[i].ToIssueUID]
	}
}

// linkTitleDeadline bounds each title read, so a slow or remote daemon cannot hold the detail back.
const linkTitleDeadline = 2 * time.Second

// translateDetail is the one place kata's selected authority becomes the model's Issue detail; nil stays nil.
//
// Capabilities say what the access may do: a scoped access gets no recurrences at all while the Issue stays visible, so for it the Recurrence stays nil ("not provided"),
// not stopped.
func translateDetail(s *g.UISelectedAuthority, capabilities g.UICapabilities) *model.IssueDetailData {
	scoped := capabilities.Scope != nil
	if s == nil {
		return nil
	}
	d := &model.IssueDetailData{AttachmentsOff: !docbankConfigured(capabilities)}
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
			IssueUID: derefString(e.IssueUID), IssueShortID: derefString(e.IssueShortID),
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
	out := &model.CommentLink{
		UID: l.UID, Kind: l.Kind, Author: l.Author, IssueUID: l.IssueUID, IssueShortID: l.IssueShortID, ProjectUID: l.ProjectUID,
		Handle: l.Handle, Teammate: l.Teammate, Status: l.Status, EditedAt: l.EditedAt,
	}
	if l.CreatedAt != nil {
		out.CreatedAt = *l.CreatedAt
	}
	out.TargetEdited = l.TargetEdited != nil && *l.TargetEdited
	return out
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
