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

type refSummary struct{ Ref, Title, Status, Owner, Updated string }

func summarize(m model.Model, f refFacts, now time.Time) refSummary {
	if issue, ok := m.LoadedIssue(f.UID); ok {
		s := refSummary{
			Ref: cmp.Or(issue.Qualified(), f.Qualified), Title: issue.Title, Status: issue.Status,
			Owner: m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String(),
		}
		if !issue.UpdatedAt.IsZero() {
			s.Updated = model.RelativeTime(issue.UpdatedAt, now)
		}
		return s
	}
	return refSummary{Ref: f.Qualified, Status: f.Status}
}

func refCardBody(s refSummary) tern.Element {
	head := ui.Nodes(ui.Text{Common: ui.Common{Key: "ref"}, Spans: []ui.Span{{T: s.Ref, S: "accent strong"}}})
	if s.Status != "" {
		head = append(head, ui.Badge{Common: ui.Common{Key: "status", Tone: statusTone(s.Status)}, Text: s.Status})
	}
	lines := ui.Nodes(ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Children: head})
	if s.Title != "" {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "title"}, Text: s.Title})
	}
	if s.Owner != "" {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "owner"}, Spans: []ui.Span{{T: "owner ", S: "muted"}, {T: s.Owner}}})
	}
	if s.Updated != "" {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "updated"}, Spans: []ui.Span{{T: "updated ", S: "muted"}, {T: s.Updated}}})
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
		parts = append(parts, "owner "+s.Owner)
	}
	if s.Updated != "" {
		parts = append(parts, "updated "+s.Updated)
	}
	return strings.Join(parts, " - ")
}

func relFacts(r model.RelationshipRef) refFacts {
	return refFacts{UID: r.UID, Qualified: r.QualifiedID, Status: r.Status}
}

func rowFacts(row model.DetailRow) refFacts {
	return refFacts{UID: row.ReferenceUID, Qualified: cmp.Or(row.Qualified, row.Ref), Status: row.Status}
}
