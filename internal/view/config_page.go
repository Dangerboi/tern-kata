package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// ConfigListID is the Config Page's list of file cards; a card is the row's key under it.
const ConfigListID = "main.columns.list.config"

// ConfigFileID is the id of the card of the row with key.
func ConfigFileID(key model.ConfigFileKey) string { return itemID(ConfigListID, string(key)) }

// configPage is the Config Page's list column: the title with the folder, then what stands in place of the list or one card per
// file (title and status, path, summary), with the selected row's full error, wrapped, under the list.
func configPage(m model.Model) tern.Element {
	c := m.ConfigPage
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Config", S: "strong"}}, Wrap: ui.WrapNone})
	column := func() tern.Element {
		return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
	}
	if notice, shown := c.Notice(); shown {
		tone := ui.TonePending
		if notice.Err {
			tone = ui.ToneError
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "notice", Tone: tone}, Text: notice.Text})
		return column()
	}
	children = append(children, ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: c.Summary()})
	cards := make([]tern.Element, len(c.Files))
	for i, f := range c.Files {
		tone := ui.ToneNeutral
		switch f.Status {
		case model.ConfigLoads:
			tone = ui.ToneSuccess
		case model.ConfigBroken:
			tone = ui.ToneError
		}
		body := ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: f.Title, S: "strong"}, {T: " · " + f.StatusWords(), S: "muted"}}, Wrap: ui.WrapNone},
			ui.Text{Common: ui.Common{Key: "path", Tone: ui.ToneMuted}, Text: f.Path, Wrap: ui.WrapNone},
			ui.Text{Common: ui.Common{Key: "summary"}, Text: f.Summary},
		)
		for _, a := range []struct {
			action model.ConfigAction
			key    string
		}{{model.ConfigOpen, "↵"}, {model.ConfigEdit, "e"}, {model.ConfigSwitch, "s"}} {
			if f.Has(a.action) {
				body = append(body, ui.Button{Common: ui.Common{Key: string(a.action), Actions: &ui.Actions{Click: ConfigActClick(f.Key, a.action)}}, Text: model.ConfigActionLabel(f, a.action), Keys: []string{a.key}})
			}
		}
		cards[i] = ui.Card{Common: ui.Common{Key: string(f.Key), Tone: tone}, Selected: i == c.Cursor, Children: body}
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("config"), Max: fillMax, Children: cards})
	if f := m.SelectedConfigFile(); f != nil && f.Err != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err})
	}
	if c.Message != "" {
		tone := ui.ToneSuccess
		if c.MessageErr {
			tone = ui.ToneError
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "message", Tone: tone}, Text: c.Message})
	}
	return column()
}
