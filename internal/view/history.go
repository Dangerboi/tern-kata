package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
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
	return ui.Text{Common: ui.Common{Key: "jump", Actions: &ui.Actions{Click: JumpClick(uid)}}, Spans: []ui.Span{{T: ref, S: "accent link"}}}
}

// referenceText is the reference as an accent link followed, only when the peer is loaded, by
// ` - ` and its title, as one text so it wraps word by word; a click anywhere jumps to the peer.
func referenceText(key, uid, ref, title string) ui.Text {
	spans := []ui.Span{{T: ref, S: "accent link"}}
	if title != "" {
		spans = append(spans, ui.Span{T: " - " + title})
	}
	return ui.Text{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: JumpClick(uid)}}, Spans: spans}
}

// linkRow is a Link row heading: `relation | ref - title`, the relation and separator muted and
// pinned at the left, the referenceText wrapping beside it. click is the relation's click, if any.
func linkRow(row model.DetailRow, click *ui.Actions) tern.Element {
	noShrink := 0.0
	lead := ui.Text{Common: ui.Common{Key: "heading", Actions: click, Shrink: &noShrink}, Spans: []ui.Span{{T: row.Relation + " |\u00a0", S: "muted"}}, Wrap: ui.WrapNone}
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapNone, Align: ui.AlignStart, Children: ui.Nodes(lead, referenceText("jump", row.ReferenceUID, row.Ref, row.RefTitle))}
}

// historySentence is the compact event line `[type] time actor - what it did`; the verb is toned, verb toned, the object after it. An event with a Ref makes the
// reference a link after the sentence. The author and the time are the card's head.
func historySentence(row model.DetailRow, action string) tern.Element {
	sentence := row.History
	spans := []ui.Span{
		{T: "[" + row.Type + "]", S: "muted"}, {T: " " + row.Time, S: "muted"}, {T: " "}, {T: row.Author, S: "strong"}, {T: " - "},
		{T: sentence.Verb, S: string(sentence.Tone)},
	}
	if object := strings.TrimRight(sentence.Object, " "); object != "" {
		spans = append(spans, ui.Span{T: " " + object})
	}
	text := ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: action}}, Spans: spans}
	if row.Ref == "" {
		return text
	}
	return referenceRow(row.ReferenceUID, text, row.Ref)
}
