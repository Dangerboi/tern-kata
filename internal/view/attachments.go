package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func attachmentSpans(row model.DetailRow) []ui.Span {
	return []ui.Span{{T: row.File.Name, S: "accent link"}, {T: "  " + row.File.Description(), S: "muted"}}
}
