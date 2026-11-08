package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const DaemonListID = "main.columns.list.daemons"

func daemons(m model.Model) tern.Element {
	d := m.Daemons
	children := ui.Nodes(
		heading(ui.Span{T: "Daemons", S: "strong"}),
		ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: d.Summary(), Wrap: ui.WrapNone},
	)
	if notice, shown := d.Notice(); shown {
		return listPanel(m, 0.5, append(children, noticeLines(notice.Err, !d.Loading, notice.Text, notice.Detail)...))
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
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("daemons"), Max: fillMax, Children: cards})
	if entry := m.SelectedDaemon(); entry != nil {
		children = append(children, plainText("footer", ui.ToneMuted, entry.Footer()))
	}
	if line := d.SwitchingLine(); line != "" {
		children = append(children, plainText("switching", ui.TonePending, line))
	}
	return listPanel(m, 0.5, children)
}
