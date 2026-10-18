package driver_test

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// collectionLinks builds the Links among a snapshot's Issues, which still arrive as kata's wire shape, from the model Links
// the detail fixtures use.
func collectionLinks(links []model.Link) []g.UILink {
	out := make([]g.UILink, len(links))
	for i, l := range links {
		out[i] = g.UILink{ID: l.ID, Type: l.Type, FromIssueUID: l.FromIssueUID, FromQualifiedID: l.FromQualifiedID, ToIssueUID: l.ToIssueUID, ToQualifiedID: l.ToQualifiedID}
	}
	return out
}
