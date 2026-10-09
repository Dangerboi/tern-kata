package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// referenceRow is a row heading that names another Issue: lead, then ref as
// an accent underlined link carrying the jump click, then trail.
func referenceRow(uid string, lead tern.Element, ref string, trail ...tern.Element) tern.Element {
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapXS, Wrap: true, Children: append(ui.Nodes(lead, jumpLink(uid, ref)), trail...)}
}

// jumpLink is the accent underlined reference whose click jumps to the Issue uid.
func jumpLink(uid, ref string) ui.Text {
	return ui.Text{Common: ui.Common{Key: "jump", Actions: &ui.Actions{Click: "jump=" + uid}}, Spans: []ui.Span{{T: ref, S: "accent link"}}}
}

// linkRow is a Link row heading: `relation | ref - title`, the relation and
// separator muted, the reference a link, the title only when the peer is loaded.
// Tern collapses each text node's edge spaces in layout, so the spaces between
// the pieces are no-break spaces.
// click is the relation's click, if any; the dash and title jump like the reference.
func linkRow(row model.DetailRow, click *ui.Actions) tern.Element {
	follow := &ui.Actions{Click: "jump=" + row.ReferenceUID}
	lead := ui.Text{Common: ui.Common{Key: "heading", Actions: click}, Spans: []ui.Span{{T: row.Relation + " |\u00a0", S: "muted"}}}
	nodes := ui.Nodes(lead, jumpLink(row.ReferenceUID, row.Ref))
	if row.RefTitle != "" {
		nodes = append(nodes, ui.Text{Common: ui.Common{Key: "dash", Actions: follow}, Text: "\u00a0-\u00a0"}, ui.Text{Common: ui.Common{Key: "title", Actions: follow}, Text: row.RefTitle})
	}
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapNone, Wrap: true, Children: nodes}
}

// historyHeading shows an event as its sentence, with the verb toned and the
// local time muted after it. An event with a Ref makes it a link and the actor muted.
func historyHeading(row model.DetailRow, action string) tern.Element {
	sentence := row.History
	actor := ui.Span{T: sentence.Actor, S: "strong"}
	object, linked := sentence.Object, row.Ref != ""
	if linked {
		actor.S = "muted"
	}
	spans := []ui.Span{actor, {T: " "}, {T: sentence.Verb, S: string(sentence.Tone)}}
	if object != "" {
		spans = append(spans, ui.Span{T: " " + strings.TrimRight(object, " ")})
	}
	when := ui.Span{T: "  " + sentence.When, S: "muted"}
	click := &ui.Actions{Click: action}
	if !linked {
		return ui.Text{Common: ui.Common{Key: "heading", Actions: click}, Spans: append(spans, when)}
	}
	return referenceRow(row.ReferenceUID, ui.Text{Common: ui.Common{Key: "heading", Actions: click}, Spans: spans}, row.Ref, ui.Text{Common: ui.Common{Key: "when", Actions: click}, Spans: []ui.Span{when}})
}
