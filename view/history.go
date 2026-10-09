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
	jump := ui.Text{Common: ui.Common{Key: "jump", Actions: &ui.Actions{Click: "jump=" + uid}}, Spans: []ui.Span{{T: ref, S: "accent link"}}}
	return ui.Row{Common: ui.Common{Key: "row"}, Gap: ui.GapXS, Wrap: true, Children: append(ui.Nodes(lead, jump), trail...)}
}

// historyHeading shows an event as its sentence, with the verb toned and the
// local time muted after it. An event whose sentence ends in the short id of
// the Issue it references makes that id a link and the actor muted.
func historyHeading(row model.DetailRow, action string) tern.Element {
	sentence := row.History
	actor := ui.Span{T: sentence.Actor, S: "strong"}
	object, linked := strings.CutSuffix(sentence.Object, sentence.Ref)
	linked = linked && sentence.Ref != "" && row.ReferenceUID != ""
	if linked {
		actor.S = "muted"
	} else {
		object = sentence.Object
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
	return referenceRow(row.ReferenceUID, ui.Text{Common: ui.Common{Key: "heading", Actions: click}, Spans: spans}, sentence.Ref, ui.Text{Common: ui.Common{Key: "when", Actions: click}, Spans: []ui.Span{when}})
}
