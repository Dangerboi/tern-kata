package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const ConfigListID = "main.columns.list.config"

func ConfigFileID(key model.ConfigFileKey) string { return itemID(ConfigListID, string(key)) }

// configHead is the Page's head: the title, then the muted count and the folder the files live in. The folder is a path, so it
// is the one mono span. The row wraps, so a long folder drops under the title and never squeezes it.
func configHead(c model.ConfigPageState) tern.Element {
	title := ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: "Config", S: "strong"}}, Wrap: ui.WrapNone}
	if c.Folder == "" {
		return ui.Row{Common: ui.Common{Key: "heading"}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: ui.Nodes(title)}
	}
	facts := ui.Text{Common: ui.Common{Key: "facts", Tone: ui.ToneMuted}, Spans: []ui.Span{{T: strconv.Itoa(c.Count()) + " · ", S: "muted"}, monoSpan(c.Summary(), "muted")}, Wrap: ui.WrapNone}
	return ui.Row{Common: ui.Common{Key: "heading"}, Gap: ui.GapMD, Wrap: true, Align: ui.AlignCenter, Children: ui.Nodes(title, facts)}
}

// configStatusTone is the tone of a file row's status word: ok is an outcome that went well, broken is an error, and a file that
// is not created is no problem, so it is muted.
func configStatusTone(status model.ConfigFileStatus) ui.Tone {
	switch status {
	case model.ConfigBroken:
		return ui.ToneError
	case model.ConfigLoads:
		return ui.ToneSuccess
	}
	return ui.ToneMuted
}

func configPage(m model.Model) tern.Element {
	c := m.ConfigPage
	children := ui.Nodes(configHead(c))
	if notice, shown := c.Notice(); shown {
		tone := ui.TonePending
		if notice.Err {
			tone = ui.ToneError
		}
		return listPanel(m, 0.4, append(children, toneText("notice", tone, notice.Text)))
	}
	cards := make([]tern.Element, len(c.Files))
	for i, f := range c.Files {
		head := ui.Nodes(ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: f.Title, S: "strong"}}, Wrap: ui.WrapNone})
		if words := f.StatusWords(); words != "" {
			head = append(head, toneText("status", configStatusTone(f.Status), words))
		}
		body := ui.Nodes(ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapMD, Wrap: true, Children: head})
		body = append(body, ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Spans: []ui.Span{{T: f.Summary, S: "muted"}}})
		if path := f.ShownPath(c.Folder); path != "" {
			body = append(body, ui.Text{Common: ui.Common{Key: "path"}, Spans: []ui.Span{monoSpan(c.Short(path), "muted")}, Wrap: ui.WrapNone})
		}
		for _, a := range []struct {
			action model.ConfigAction
			key    string
		}{{model.ConfigOpen, "enter"}, {model.ConfigEdit, "e"}, {model.ConfigSwitchDocbank, "s"}, {model.ConfigTurnOn, "s"}, {model.ConfigRestart, "r"}} {
			// Open and Turn on of the Search by meaning row reach the same card, so the row offers the Turn on button alone while it is
			// on offer; enter stays bound to the card.
			if f.Has(a.action) && !(a.action == model.ConfigOpen && f.Has(model.ConfigTurnOn)) {
				body = append(body, ui.Button{Common: ui.Common{Key: string(a.action), Actions: &ui.Actions{Click: ConfigActClick(f.Key, a.action)}}, Text: model.ConfigActionLabel(f, a.action), Keys: []string{a.key}})
			}
		}
		cards[i] = ui.Card{Common: ui.Common{Key: string(f.Key)}, Selected: i == c.Cursor, Children: body}
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("config"), Max: fillMax, Children: cards})
	if f := m.SelectedConfigFile(); f != nil && f.Err != "" {
		children = append(children, toneText("error", ui.ToneError, f.Err))
	}
	if c.Message != "" {
		tone := ui.ToneSuccess
		if c.MessageErr {
			tone = ui.ToneError
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "message", Tone: tone}, Spans: prose(c.Message, toneStyle(tone))})
	}
	return listPanel(m, 0.4, children)
}
