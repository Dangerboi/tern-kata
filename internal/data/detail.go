package data

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// ReadDetail reads an Issue's detail without the titles of its Links' far ends: for reads that only look at the Issue itself (a
// delete's reason key, a draft conflict, a recurrence target), which must not pay a request per linked Issue.
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
	return model.DetailLoaded{UID: uid, Selected: detail}, nil
}

// ReadDetailWithLinkTitles is ReadDetail for a read that shows the detail: opening an Issue. The Links carry the titles of their ends.
func (c *Client) ReadDetailWithLinkTitles(ctx context.Context, uid string) (model.DetailLoaded, error) {
	loaded, err := c.ReadDetail(ctx, uid)
	if err != nil {
		return loaded, err
	}
	c.linkTitles(ctx, loaded.Selected)
	return loaded, nil
}

// linkTitleTTL is how long a title read for a Link's far end is trusted before it is read again. A title that cannot be read again
// stays as it was: a daemon that is busy or briefly gone must not blank a title the Pane already showed.
const linkTitleTTL = 5 * time.Minute

// linkTitleWorkers bounds the reads made at once, so a parent with a hundred children does not open a hundred requests.
const linkTitleWorkers = 8

// linkTitleBudget bounds all of one detail's title reads together; the titles read by then are kept and the rest stay with the
// cache or empty, and the next refresh asks again for what is still missing.
const linkTitleBudget = 4 * time.Second

type cachedTitle struct {
	title string
	at    time.Time
}

// linkTitles gives each Link of the detail the titles of its ends, so the Links tab and the Children table name an Issue that is
// closed, in another Project or not in the Pane's list. Only the reads that show a detail (the one that opens it and each live
// refresh) come through here. Titles come from a cache; the stale or missing ones are read from kata with a few reads at once and
// a deadline on the whole. An Issue that cannot be read keeps the title it had, or none; one kata refuses (deleted, no access) is
// remembered as unreadable for the cache period, so a refresh does not ask for it again.
func (c *Client) linkTitles(ctx context.Context, d *model.IssueDetailData) {
	if d == nil || d.Issue == nil || len(d.Links) == 0 {
		return
	}
	titles := map[string]string{}
	previous := map[string]string{}
	var stale []string
	for _, l := range d.Links {
		for _, uid := range []string{l.FromIssueUID, l.ToIssueUID} {
			if uid == "" || uid == d.Issue.UID {
				continue
			}
			if _, seen := titles[uid]; seen {
				continue
			}
			v, ok := c.titles.Load(uid)
			if ok {
				cached := v.(cachedTitle)
				titles[uid], previous[uid] = cached.title, cached.title
				if time.Since(cached.at) < linkTitleTTL {
					continue
				}
			} else {
				titles[uid] = ""
			}
			stale = append(stale, uid)
		}
	}
	if len(stale) > 0 {
		budget, cancel := context.WithTimeout(ctx, linkTitleBudget)
		defer cancel()
		var mu sync.Mutex
		var wg sync.WaitGroup
		queue := make(chan string)
		for range min(linkTitleWorkers, len(stale)) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for uid := range queue {
					r, err := c.ShowIssueByUIDWithResponse(budget, &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: uid}})
					if err != nil || r.JSON200 == nil {
						if refused(err, r) && budget.Err() == nil {
							c.titles.Store(uid, cachedTitle{title: previous[uid], at: time.Now()})
						}
						continue
					}
					c.titles.Store(uid, cachedTitle{title: r.JSON200.Issue.Title, at: time.Now()})
					mu.Lock()
					titles[uid] = r.JSON200.Issue.Title
					mu.Unlock()
				}
			}()
		}
	feed:
		for _, uid := range stale {
			select {
			case queue <- uid:
			case <-budget.Done():
				break feed
			}
		}
		close(queue)
		wg.Wait()
	}
	for i := range d.Links {
		d.Links[i].FromTitle, d.Links[i].ToTitle = titles[d.Links[i].FromIssueUID], titles[d.Links[i].ToIssueUID]
	}
}

// refused says kata answered the read of an Issue with a client error (deleted, no access), not that the daemon was busy or gone.
func refused(err error, r *g.ShowIssueByUIDResp) bool {
	if err == nil {
		return r != nil && r.JSON200 == nil
	}
	status := client.StatusCode(err)
	return status >= 400 && status < 500
}

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
