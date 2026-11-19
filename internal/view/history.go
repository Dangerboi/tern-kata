package view

import (
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func jumpLink(uid, ref string, card ui.Hovercard) ui.Text {
	return ui.Text{Common: ui.Common{Key: "jump", Hovercard: card, Actions: &ui.Actions{Click: JumpClick(uid)}}, Spans: []ui.Span{{T: ref, S: mono("accent link")}}}
}

func referenceText(key, uid, ref, title string, card ui.Hovercard) ui.Text {
	spans := []ui.Span{{T: ref, S: mono("accent link")}}
	if title != "" {
		spans = append(spans, ui.Span{T: " " + title})
	}
	return ui.Text{Common: ui.Common{Key: key, Hovercard: card, Actions: &ui.Actions{Click: JumpClick(uid)}}, Spans: spans}
}

// linkRow is a Links row: the linked Issue's id and its title. The relation is named once, by the group heading above the rows.
func linkRow(m model.Model, row model.DetailRow, click *ui.Actions, now time.Time) tern.Element {
	ref := referenceText("jump", row.ReferenceUID, row.Ref, row.RefTitle, refCard(m, rowFacts(row), now))
	return ui.Row{Common: ui.Common{Key: "row", Actions: click}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(ref)}
}

// linkGroupHeading heads the rows of one relation: its name and how many Issues it holds.
func linkGroupHeading(m model.Model, row model.DetailRow) tern.Element {
	spans := []ui.Span{{T: row.RelationLabel, S: "strong"}}
	if n := countSpan(m.LinkGroupCount(row.Relation)); n.T != "" {
		spans = append(spans, ui.Span{T: " "}, n)
	}
	return ui.Text{Common: ui.Common{Key: "group-" + row.Relation}, Spans: spans}
}

// EventTimeRole marks an event's time, which EventTimeCSS sets off from the sentence with a thin rule.
const EventTimeRole = "tern-kata.event-time"

const EventTimeCSS = `.sf-text[data-role~='` + EventTimeRole + `'] { border-right: 2px solid var(--t3); padding-right: 10px; white-space: pre; }`

// historySentence is an event as its time, a rule, then "<author> <verb> <object>" with the verb and object in the verb's tone; a
// referenced Issue's id follows as a link.
func historySentence(m model.Model, row model.DetailRow, action string, now time.Time) tern.Element {
	sentence := row.History
	noShrink := 0.0
	clicks := &ui.Actions{Click: action}
	when := ui.Text{Common: ui.Common{Key: "time", Role: EventTimeRole, Actions: clicks, Shrink: &noShrink}, Spans: []ui.Span{timeSpan(row.Time)}, Wrap: ui.WrapNone}
	spans := []ui.Span{{T: row.Author, S: "strong"}, {T: " "}, {T: sentence.Verb, S: string(sentence.Tone)}}
	if object := strings.TrimRight(sentence.Object, " "); object != "" {
		spans = append(spans, ui.Span{T: " " + object, S: string(sentence.Tone)})
	}
	what := ui.Text{Common: ui.Common{Key: "heading", Actions: clicks}, Spans: spans}
	children := ui.Nodes(when, what)
	if row.Ref != "" {
		children = append(children, jumpLink(row.ReferenceUID, row.Ref, refCard(m, rowFacts(row), now)))
	}
	if sentence.After != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "after", Actions: clicks}, Spans: []ui.Span{{T: sentence.After, S: string(sentence.Tone)}}})
	}
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapMD, Align: ui.AlignStart, Wrap: true, Children: children}
}
