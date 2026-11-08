package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CredentialListID = "main.columns.list.credentials"

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

func credentials(m model.Model) tern.Element {
	c := m.Credentials
	children := ui.Nodes(
		heading(ui.Span{T: "Credentials", S: "strong"}),
		ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: c.Summary(), Wrap: ui.WrapNone},
	)
	if notice, shown := c.Notice(); shown {
		return listPanel(m, 0.5, append(children, noticeLines(notice.Err, false, notice.Text, notice.Detail)...))
	}
	cards := make([]tern.Element, len(c.Tokens))
	for i, token := range c.Tokens {
		cards[i] = credentialCard(token, i == c.Cursor)
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("credentials"), Max: fillMax, Children: cards})
	if failure := c.RefreshError(); failure != "" {
		children = append(children, plainText("refresh-error", ui.ToneError, failure))
	}
	return listPanel(m, 0.5, children)
}

func credentialCard(token model.Credential, selected bool) tern.Element {
	lines := token.Lines()
	body := make([]tern.Element, len(lines))
	for i, line := range lines {
		body[i] = ui.Text{Common: ui.Common{Key: "line" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone}
	}
	return ui.Card{Common: ui.Common{Key: strconv.FormatInt(token.ID, 10), Tone: credentialTone(token.State)}, Selected: selected, Children: body}
}
