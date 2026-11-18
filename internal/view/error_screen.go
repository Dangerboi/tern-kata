package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// RetryID is the Retry button of the error screen.
const RetryID = "main.error.retry"

// daemonHint is the sentence the data layer adds to an error when it found no daemon to talk to. An error without it is one
// kata answered, with words meant to be read.
const daemonHint = "kata daemon start"

// failureView is the Pane's error screen, drawn in place of the Page while the daemon error stands. It is a centred empty state:
// what is wrong, what to do, Retry, and the Daemons button for switching to another daemon, under the top bar, which keeps the Menu
// in reach. When kata could not be reached the raw error is in the text's tooltip only; when kata answered with its own message
// (a Project that is not set up, a malformed .kata.toml) that message is the text, as the person needs its hint.
func failureView(m model.Model) tern.Nodes {
	empty := ui.Empty{
		CommonWithoutTitle: ui.CommonWithoutTitle{Key: "error", Grow: 1},
		Children: ui.Nodes(
			ui.Button{
				Common: ui.Common{Key: "retry", Actions: &ui.Actions{Click: RetryClick}},
				Text:   "Retry", Keys: retryKeys(), Variant: ui.ButtonPrimary,
			},
			cardButton("daemons", model.ModeError, model.ShowDaemonsPage, "Daemons", false),
		),
	}
	if strings.Contains(m.Err.Error(), daemonHint) {
		empty.Title = ui.T("Cannot reach kata")
		empty.Text = ui.Spans(
			ui.Span{T: "Start the daemon with "}, monoSpan("kata daemon start", ""),
			ui.Span{T: ", or check it with "}, monoSpan("kata doctor", ""), ui.Span{T: "."},
		)
		empty.Hovercard = ui.Hovercard{Element: plainText("reason", ui.ToneNeutral, model.Reason(m.Err))}
	} else {
		empty.Title = ui.T("Cannot open this Project")
		empty.Text = ui.T(m.Err.Error())
	}
	return ui.Nodes(topBar(m), empty)
}

// retryKeys is the keycap of the retry key: ctrl+r only, whichever aliases the mode binds.
func retryKeys() []string {
	keys, _ := cardKeys(model.ModeError, model.RetryAction)
	return keys
}
