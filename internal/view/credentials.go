package view

import (
	"strconv"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CredentialListID = "main.columns.list.credentials"

// credentialTone is the tone of a credential's state word: live went well, expired needs attention, revoked is finished.
func credentialTone(state string) ui.Tone {
	if state == "live" {
		return ui.ToneSuccess
	}
	return runStateTone(state)
}

// credentials is the Credentials Page: a card per API token with its state and its label→value rows. The ids kata knows a token
// by are in the card's tooltip, not on it.
func credentials(m model.Model, now time.Time) tern.Element {
	c := m.Credentials
	notice, noticed := c.Notice()
	count := ""
	if !noticed {
		count = strconv.Itoa(len(c.Tokens))
	}
	children := ui.Nodes(pageHead("heading", "Credentials", count, c.AsOf(m.Calendar, now)))
	if noticed {
		if notice.Empty {
			empty := emptyState(notice.Text, "Create a token for a script or an agent to use.", emptyButton(m, "new", model.NewToken, "New token", true)...)
			empty.Key = "empty"
			return listPanel(m, 0.5, append(children, empty))
		}
		return listPanel(m, 0.5, append(children, noticeLines(notice.Err, !notice.Pending, notice.Text, "")...))
	}
	cards := make([]tern.Element, len(c.Tokens))
	for i, token := range c.Tokens {
		cards[i] = credentialCard(m, token, i == c.Cursor, now)
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("credentials"), Max: fillMax, Children: cards})
	if failure := c.RefreshError(); failure != "" {
		children = append(children, plainText("refresh-error", ui.ToneError, failure))
	}
	return listPanel(m, 0.5, children)
}

func credentialCard(m model.Model, token model.Credential, selected bool, now time.Time) tern.Element {
	state := propRow("State", ui.Text{Common: ui.Common{Key: "v"}, Spans: []ui.Span{{T: token.State, S: toneStyle(credentialTone(token.State))}}, Wrap: ui.WrapNone})
	rows := append(ui.Nodes(state), factPropRows(token.Rows(m.Calendar, now))...)
	grid := propGrid(rows...)
	return ui.Card{Common: ui.Common{Key: strconv.FormatInt(token.ID, 10), Tone: credentialTone(token.State), Title: token.Title()}, Selected: selected, Children: ui.Nodes(grid)}
}
