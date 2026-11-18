package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// attachmentSpans is an Attachments tab row: the file's name as a link, then its kind word and its size, each after a muted
// separator. The size is a number a person compares across rows, so it is mono.
func attachmentSpans(row model.DetailRow) []ui.Span {
	sep := ui.Span{T: " · ", S: "muted"}
	return []ui.Span{
		{T: row.File.Name, S: "accent link"}, sep,
		{T: row.File.Kind(), S: "muted"}, sep,
		monoSpan(model.FormatSize(row.File.Size), "muted"),
	}
}

// attachmentTitle is the row's tooltip: the raw media type, which the row shows only as a kind word.
func attachmentTitle(row model.DetailRow) string { return row.File.MediaType }
