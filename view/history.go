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

// referenceTail is the reference as a link followed, only when the peer is
// loaded, by ` - ` and its title; the dash and title jump like the reference.
// Tern collapses each text node's edge spaces in layout, so the spaces around
// the dash are no-break spaces.
func referenceTail(uid, ref, title string) tern.Nodes {
	nodes := ui.Nodes(jumpLink(uid, ref))
	if title != "" {
		follow := &ui.Actions{Click: "jump=" + uid}
		nodes = append(nodes, ui.Text{Common: ui.Common{Key: "dash", Actions: follow}, Text: "\u00a0-\u00a0"}, ui.Text{Common: ui.Common{Key: "title", Actions: follow}, Text: title})
	}
	return nodes
}

// linkRow is a Link row heading: `relation | ref - title`, the relation and
// separator muted, then the referenceTail. click is the relation's click, if any.
func linkRow(row model.DetailRow, click *ui.Actions) tern.Element {
	lead := ui.Text{Common: ui.Common{Key: "heading", Actions: click}, Spans: []ui.Span{{T: row.Relation + " |\u00a0", S: "muted"}}}
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapNone, Wrap: true, Children: append(ui.Nodes(lead), referenceTail(row.ReferenceUID, row.Ref, row.RefTitle)...)}
}

// historySentence is what an event did, verb toned, the object after it. An event with a Ref makes the
// reference a link after the sentence. The author and the time are the card's head.
func historySentence(row model.DetailRow, action string) tern.Element {
	sentence := row.History
	spans := []ui.Span{{T: sentence.Verb, S: string(sentence.Tone)}}
	if object := strings.TrimRight(sentence.Object, " "); object != "" {
		spans = append(spans, ui.Span{T: " " + object})
	}
	text := ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: action}}, Spans: spans}
	if row.Ref == "" {
		return text
	}
	return referenceRow(row.ReferenceUID, text, row.Ref)
}
