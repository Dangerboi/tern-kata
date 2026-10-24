package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// CredentialListID is the Credentials Page's list of token cards; a card is the token's id under it.
const CredentialListID = "main.columns.list.credentials"

// CredentialID is the id of the card of the token with id.
func CredentialID(id int64) string { return itemID(CredentialListID, strconv.FormatInt(id, 10)) }

// credentialTone is the tone of a token's state: live green, expired amber, revoked muted.
func credentialTone(state string) ui.Tone {
	switch state {
	case "live":
		return ui.ToneSuccess
	case "expired":
		return ui.ToneWarning
	case "revoked":
		return ui.ToneMuted
	}
	return ui.ToneNeutral
}

// credentials is the Credentials Page's list column: a heading with the summary, then what stands in place of the list or one
// card per token, newest first. It is read only.
func credentials(m model.Model) tern.Element {
	c := m.Credentials
	children := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Credentials", S: "strong"}}, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: c.Summary(), Wrap: ui.WrapNone},
	)
	if notice, shown := c.Notice(); shown {
		tone := ui.TonePending
		if notice.Err {
			tone = ui.ToneError
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "notice", Tone: tone}, Text: notice.Text})
		if notice.Detail != "" {
			children = append(children, ui.Text{Common: ui.Common{Key: "detail", Tone: ui.ToneMuted}, Text: notice.Detail})
		}
		return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
	}
	cards := make([]tern.Element, len(c.Tokens))
	for i, token := range c.Tokens {
		cards[i] = credentialCard(token, i == c.Cursor)
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("credentials"), Max: scrollMax(m, 2*(m.RowPx+6+8)), Children: cards})
	if failure := c.RefreshError(); failure != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "refresh-error", Tone: ui.ToneError}, Text: failure})
	}
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
}

// credentialCard is one token on kata tui's four lines.
func credentialCard(token model.Credential, selected bool) tern.Element {
	lines := token.Lines()
	body := make([]tern.Element, len(lines))
	for i, line := range lines {
		body[i] = ui.Text{Common: ui.Common{Key: "line" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone}
	}
	return ui.Card{Common: ui.Common{Key: strconv.FormatInt(token.ID, 10), Tone: credentialTone(token.State)}, Selected: selected, Children: body}
}
