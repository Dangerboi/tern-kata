package view

import (
	"strconv"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// FederationListID is the Federation Page's list of spoke cards; a card is the Project's name under it.
const FederationListID = "main.columns.list.federation"

// FederationSpokeID is the id of the card of the spoke Project called name.
func FederationSpokeID(name string) string { return itemID(FederationListID, name) }

// federation is the Federation Page's list column: the title with its summary, the active daemon's header, then the screen shown:
// what stands in place of the list, one card per spoke Project with the selected one's hub, actor and credential under it, or one
// Project's detail.
func federation(m model.Model, now time.Time) tern.Element {
	f := m.Federation
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Federation", S: "strong"}}, Wrap: ui.WrapNone})
	if f.Screen == model.FederationList {
		children = append(children, ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: f.Summary(), Wrap: ui.WrapNone})
	}
	children = append(children, ui.Text{Common: ui.Common{Key: "header", Tone: ui.ToneMuted}, Text: f.Header()})
	column := func() tern.Element {
		return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
	}
	spoke := m.SelectedSpoke()
	if f.Screen == model.FederationDetail && spoke != nil {
		for i, line := range spoke.DetailLines() {
			children = append(children, ui.Text{Common: ui.Common{Key: "detail" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone})
		}
		return column()
	}
	if notice, shown := f.Notice(); shown {
		tone := ui.TonePending
		if notice.Err {
			tone = ui.ToneError
		} else if !f.Loading {
			tone = ui.ToneMuted
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "notice", Tone: tone}, Text: notice.Text})
		return column()
	}
	cards := make([]tern.Element, len(f.Spokes))
	for i, s := range f.Spokes {
		cards[i] = federationCard(s, i == f.Cursor, now)
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("federation"), Max: scrollMax(m, 2*(m.RowPx+6+8)), Children: cards})
	if spoke != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "footer", Tone: ui.ToneMuted}, Text: spoke.Footer()})
	}
	return column()
}

// federationCard is one spoke Project on two lines, toned by what needs attention: a badge draws it in a warning, none plain.
func federationCard(s model.FederationSpoke, selected bool, now time.Time) tern.Element {
	lines := s.Lines(now)
	body := make([]tern.Element, len(lines))
	for i, line := range lines {
		body[i] = ui.Text{Common: ui.Common{Key: "line" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone}
	}
	tone := ui.ToneNeutral
	if s.Badges() != "-" {
		tone = ui.ToneWarning
	}
	return ui.Card{Common: ui.Common{Key: s.ProjectName, Tone: tone}, Selected: selected, Children: body}
}
