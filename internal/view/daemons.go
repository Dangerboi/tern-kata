package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// DaemonListID is the Daemons Page's list of daemon cards; a card is the daemon's name under it.
const DaemonListID = "main.columns.list.daemons"

// DaemonID is the id of the card of the daemon called name.
func DaemonID(name string) string { return itemID(DaemonListID, name) }

// daemons is the Daemons Page's list column: the title with its count, the header in kata tui's columns, then what stands in place of
// the list or one card per daemon with the selected one's name and endpoint under the list.
func daemons(m model.Model) tern.Element {
	d := m.Daemons
	children := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Daemons", S: "strong"}}, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: d.Summary(), Wrap: ui.WrapNone},
	)
	column := func() tern.Element {
		return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
	}
	if notice, shown := d.Notice(); shown {
		tone := ui.TonePending
		if notice.Err {
			tone = ui.ToneError
		} else if !d.Loading {
			tone = ui.ToneMuted
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "notice", Tone: tone}, Text: notice.Text})
		if notice.Detail != "" {
			children = append(children, ui.Text{Common: ui.Common{Key: "detail", Tone: ui.ToneMuted}, Text: notice.Detail})
		}
		return column()
	}
	children = append(children, ui.Text{Common: ui.Common{Key: "header", Tone: ui.ToneMuted}, Text: model.DaemonsHeader(), Wrap: ui.WrapNone})
	cards := make([]tern.Element, len(d.Entries))
	for i, e := range d.Entries {
		tone := ui.ToneNeutral
		if e.Current {
			tone = ui.ToneSuccess
		}
		cards[i] = ui.Card{Common: ui.Common{Key: e.Name, Tone: tone}, Selected: i == d.Cursor,
			Children: ui.Nodes(ui.Text{Common: ui.Common{Key: "row"}, Text: e.Row(), Wrap: ui.WrapNone})}
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("daemons"), Max: scrollMax(m, m.RowPx+6+8), Children: cards})
	if entry := m.SelectedDaemon(); entry != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "footer", Tone: ui.ToneMuted}, Text: entry.Footer()})
	}
	if line := d.SwitchingLine(); line != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "switching", Tone: ui.TonePending}, Text: line})
	}
	return column()
}
