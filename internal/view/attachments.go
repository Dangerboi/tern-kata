package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// emptyTabText is what an Activity tab with no rows says.
func emptyTabText(tab model.ActivityTab) string {
	if tab == model.TabAttachments {
		return "No attachments, u to attach"
	}
	return "No " + strings.ToLower(tab.String()) + "."
}

// attachmentSpans is an Attachment row: the name as a link to its file when this machine holds it, else `<name> on <host>`.
func attachmentSpans(row model.DetailRow) []ui.Span {
	if row.URL != "" {
		return []ui.Span{{T: row.File.Name, S: "accent link", Href: row.URL}}
	}
	return []ui.Span{{T: row.File.Name + " on " + row.File.Host}}
}
