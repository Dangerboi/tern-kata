package view

import (
	"cmp"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

type refFacts struct{ UID, Qualified, Status string }

// refSummary is what a hovercard, a tooltip and a child's cell say about an Issue. Ref is the id as the Pane reads it: "#id"
// inside the Project on screen, "project#id" otherwise. Meta is the owner (when there is one) and the time of the last update,
// as the facts of the one muted line.
type refSummary struct{ Ref, Title, Status, Owner, Updated string }

func summarize(m model.Model, f refFacts, now time.Time) refSummary {
	if issue, ok := m.LoadedIssue(f.UID); ok {
		s := refSummary{
			Ref: m.RefName(issue.ProjectName, issue.ShortID), Title: issue.Title, Status: issue.Status,
			Owner: m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String(),
		}
		if !issue.UpdatedAt.IsZero() {
			s.Updated = m.Calendar.Read(issue.UpdatedAt, now)
		}
		if s.Ref == "#" {
			s.Ref = cmp.Or(issue.Qualified(), f.Qualified)
		}
		return s
	}
	return refSummary{Ref: relativeRef(m, f.Qualified), Status: f.Status}
}

// relativeRef names an Issue given as "project#id" the way the Pane's Collection does.
func relativeRef(m model.Model, qualified string) string {
	project, short, ok := strings.Cut(qualified, "#")
	if !ok || short == "" {
		return qualified
	}
	return m.RefName(project, short)
}

func refCardBody(s refSummary) tern.Element {
	head := ui.Nodes(ui.Text{Common: ui.Common{Key: "ref"}, Spans: []ui.Span{monoSpan(s.Ref, "strong")}})
	if s.Status != "" {
		head = append(head, ui.Text{Common: ui.Common{Key: "status"}, Spans: []ui.Span{{T: s.Status, S: toneStyle(issueStateTone(s.Status != "open"))}}})
	}
	lines := ui.Nodes(ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Children: head})
	if s.Title != "" {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "title"}, Text: s.Title})
	}
	var meta []ui.Span
	if s.Owner != "" {
		meta = append(meta, ui.Span{T: s.Owner, S: "muted"})
	}
	if s.Updated != "" {
		if meta != nil {
			meta = append(meta, ui.Span{T: " · ", S: "muted"})
		}
		meta = append(meta, timeSpan(s.Updated))
	}
	if meta != nil {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "meta"}, Spans: meta})
	}
	return ui.Col{Common: ui.Common{Key: "card"}, Gap: ui.GapXS, Children: lines}
}

func refCard(m model.Model, f refFacts, now time.Time) ui.Hovercard {
	return ui.Hovercard{Element: refCardBody(summarize(m, f, now))}
}

func refCards(m model.Model, facts []refFacts, now time.Time) ui.Hovercard {
	if len(facts) == 1 {
		return refCard(m, facts[0], now)
	}
	bodies := make(tern.Nodes, len(facts))
	for i, f := range facts {
		bodies[i] = refCardBody(summarize(m, f, now))
	}
	return ui.Hovercard{Element: ui.Col{Common: ui.Common{Key: "cards"}, Gap: ui.GapMD, Children: bodies}}
}

// refTooltip is the card's facts as one plain line, for a node that takes a tooltip and no hovercard.
func refTooltip(s refSummary) string {
	parts := []string{s.Ref}
	for _, p := range []string{s.Status, s.Title} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if s.Owner != "" {
		parts = append(parts, s.Owner)
	}
	if s.Updated != "" {
		parts = append(parts, s.Updated)
	}
	return strings.Join(parts, " · ")
}

func relFacts(r model.RelationshipRef) refFacts {
	return refFacts{UID: r.UID, Qualified: r.QualifiedID, Status: r.Status}
}

func rowFacts(row model.DetailRow) refFacts {
	return refFacts{UID: row.ReferenceUID, Qualified: cmp.Or(row.Qualified, row.Ref), Status: row.Status}
}
