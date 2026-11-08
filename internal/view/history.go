package view

import (
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func referenceRow(uid string, lead tern.Element, ref string, card ui.Hovercard, trail ...tern.Element) tern.Element {
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapXS, Wrap: true, Children: append(ui.Nodes(lead, jumpLink(uid, ref, card)), trail...)}
}

func jumpLink(uid, ref string, card ui.Hovercard) ui.Text {
	return ui.Text{Common: ui.Common{Key: "jump", Hovercard: card, Actions: &ui.Actions{Click: JumpClick(uid)}}, Spans: []ui.Span{{T: ref, S: mono("accent link")}}}
}

func referenceText(key, uid, ref, title string, card ui.Hovercard) ui.Text {
	spans := []ui.Span{{T: ref, S: mono("accent link")}}
	if title != "" {
		spans = append(spans, ui.Span{T: " - " + title})
	}
	return ui.Text{Common: ui.Common{Key: key, Hovercard: card, Actions: &ui.Actions{Click: JumpClick(uid)}}, Spans: spans}
}

func linkRow(m model.Model, row model.DetailRow, click *ui.Actions, now time.Time) tern.Element {
	noShrink := 0.0
	lead := ui.Text{Common: ui.Common{Key: "heading", Actions: click, Shrink: &noShrink}, Spans: []ui.Span{{T: row.Relation + " |\u00a0", S: "muted"}}, Wrap: ui.WrapNone}
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapNone, Align: ui.AlignStart, Children: ui.Nodes(lead, referenceText("jump", row.ReferenceUID, row.Ref, row.RefTitle, refCard(m, rowFacts(row), now)))}
}

func historySentence(m model.Model, row model.DetailRow, action string, now time.Time) tern.Element {
	sentence := row.History
	spans := []ui.Span{
		{T: "[" + row.Type + "]", S: "muted"}, {T: " " + row.Time, S: mono("muted")}, {T: " "}, {T: row.Author, S: "strong"}, {T: " - "},
		{T: sentence.Verb, S: string(sentence.Tone)},
	}
	if object := strings.TrimRight(sentence.Object, " "); object != "" {
		spans = append(spans, ui.Span{T: " " + object})
	}
	text := ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: action}}, Spans: spans}
	if row.Ref == "" {
		return text
	}
	return referenceRow(row.ReferenceUID, text, row.Ref, refCard(m, rowFacts(row), now))
}
