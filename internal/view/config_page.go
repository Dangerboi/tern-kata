package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const ConfigListID = "main.columns.list.config"

func ConfigFileID(key model.ConfigFileKey) string { return itemID(ConfigListID, string(key)) }

func configPage(m model.Model) tern.Element {
	c := m.ConfigPage
	children := ui.Nodes(heading(ui.Span{T: "Config", S: "strong"}))
	if notice, shown := c.Notice(); shown {
		return listPanel(m, 0.5, append(children, noticeLines(notice.Err, false, notice.Text, "")...))
	}
	children = append(children, plainText("summary", ui.ToneMuted, c.Summary()))
	cards := make([]tern.Element, len(c.Files))
	for i, f := range c.Files {
		tone := ui.ToneNeutral
		switch f.Status {
		case model.ConfigBroken:
			tone = ui.ToneError
		case model.ConfigLoads:
			tone = ui.ToneSuccess
		}
		title := ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: f.Title, S: "strong"}}, Wrap: ui.WrapNone}
		if words := f.StatusWords(); words != "" {
			title.Spans = append(title.Spans, ui.Span{T: " · " + words, S: "muted"})
		}
		body := ui.Nodes(title)
		if f.Path != "" {
			body = append(body, ui.Text{Common: ui.Common{Key: "path", Tone: ui.ToneMuted}, Text: f.Path, Wrap: ui.WrapNone})
		}
		body = append(body, ui.Text{Common: ui.Common{Key: "summary"}, Text: f.Summary})
		for _, a := range []struct {
			action model.ConfigAction
			key    string
		}{{model.ConfigOpen, "↵"}, {model.ConfigEdit, "e"}, {model.ConfigSwitchDocbank, "s"}, {model.ConfigTurnOn, "s"}} {
			if f.Has(a.action) {
				body = append(body, ui.Button{Common: ui.Common{Key: string(a.action), Actions: &ui.Actions{Click: ConfigActClick(f.Key, a.action)}}, Text: model.ConfigActionLabel(f, a.action), Keys: []string{a.key}})
			}
		}
		cards[i] = ui.Card{Common: ui.Common{Key: string(f.Key), Tone: tone}, Selected: i == c.Cursor, Children: body}
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("config"), Max: fillMax, Children: cards})
	if f := m.SelectedConfigFile(); f != nil && f.Err != "" {
		children = append(children, plainText("error", ui.ToneError, f.Err))
	}
	if c.Message != "" {
		tone := ui.ToneSuccess
		if c.MessageErr {
			tone = ui.ToneError
		}
		children = append(children, plainText("message", tone, c.Message))
	}
	return listPanel(m, 0.5, children)
}
