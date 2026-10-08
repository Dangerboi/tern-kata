package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// historyHeading shows an event as its sentence, with the verb toned and the
// local time muted after it.
func historyHeading(row model.DetailRow, action string) tern.Element {
	spans := []ui.Span{{T: row.Actor, S: "strong"}, {T: " "}, {T: row.Verb, S: string(row.VerbTone)}}
	if row.Object != "" {
		spans = append(spans, ui.Span{T: " " + row.Object})
	}
	spans = append(spans, ui.Span{T: "  " + row.When, S: "muted"})
	return ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: action}}, Spans: spans}
}

func historyDetail(row model.DetailRow) tern.Element {
	return ui.Text{Common: ui.Common{Key: "body"}, Spans: []ui.Span{{T: row.Body, S: "muted"}}}
}
