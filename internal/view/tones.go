package view

import (
	"slices"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// laneNameTones are the tones of the default Lanes, keyed by label, so a Lane keeps its colour wherever it sits in the list.
var laneNameTones = map[string]ui.Tone{
	"needs-triage":    ui.ToneWarning,
	"needs-info":      ui.ToneUser,
	"ready-for-agent": ui.ToneInfo,
	"ready-for-human": ui.TonePending,
	"waiting":         ui.ToneError,
	model.LaneDone:    ui.ToneSuccess,
}

// laneCycleTones are the tones a Lane with no tone of its own cycles through, by its index among such Lanes.
var laneCycleTones = []ui.Tone{ui.ToneInfo, ui.ToneUser, ui.TonePending, ui.ToneWarning, ui.ToneError}

// laneTone is the colour of lane among lanes: a default Lane has its own tone wherever it is named, Done is success, the No
// lane is muted, and any other Lane takes laneCycleTones by its index among the Lanes that have no tone of their own; a Lane
// that is neither a default nor in lanes is neutral.
func laneTone(lanes []string, lane string) ui.Tone {
	if lane == model.LaneNone {
		return ui.ToneMuted
	}
	if tone, ok := laneNameTones[lane]; ok {
		return tone
	}
	index := 0
	for _, l := range lanes {
		if l == lane {
			return laneCycleTones[index%len(laneCycleTones)]
		}
		if _, named := laneNameTones[l]; !named {
			index++
		}
	}
	return ui.ToneNeutral
}

// issueStateTone is the tone of an Issue's status word: open is plain text and closed is muted. Neither takes accent (where you
// are) or success (an outcome), so the status never competes with the selection or a run's result.
func issueStateTone(closed bool) ui.Tone {
	if closed {
		return ui.ToneMuted
	}
	return ui.ToneNeutral
}

// priorityText is a priority's text and tone: P0 error, P1 user, P2 warning, P3 info, P4 muted.
func priorityText(p *int64) (string, ui.Tone) {
	if p == nil {
		return "", ui.ToneMuted
	}
	text := model.PriorityText(p)
	switch *p {
	case 0:
		return text, ui.ToneError
	case 1:
		return text, ui.ToneUser
	case 2:
		return text, ui.ToneWarning
	case 3:
		return text, ui.ToneInfo
	}
	return text, ui.ToneMuted
}

// ownerSpan is an Issue's owner as plain text, never a badge; no owner reads as a muted "unassigned".
func ownerSpan(name string) ui.Span {
	if name == "" {
		return ui.Span{T: "unassigned", S: "muted"}
	}
	return ui.Span{T: name}
}

// labelChip is a label as a chip: a label that is one of lanes is drawn in that Lane's tone, so it matches the Lane's header;
// every other label names a thing and stays neutral.
func labelChip(text string, lanes []string) ui.Badge {
	tone := ui.ToneNeutral
	if slices.Contains(lanes, text) {
		tone = laneTone(lanes, text)
	}
	return ui.Badge{Common: ui.Common{Key: "label-" + text, Tone: tone}, Text: text}
}

// runStateTone is the tone of a run's or job's state word: success went well, error failed, warning needs attention, pending
// is in progress, muted is cancelled or not known.
func runStateTone(state string) ui.Tone {
	switch state {
	case "succeeded":
		return ui.ToneSuccess
	case "failed":
		return ui.ToneError
	case "held", "paused", "expired":
		return ui.ToneWarning
	case "running", "pending":
		return ui.TonePending
	}
	return ui.ToneMuted
}

// toneStyle is the text span style closest to tone. Pending is Tern's own span token for words that wait on something; neutral is the plain text colour.
// Tern has no span token for user, so a user-toned span draws in the plain text colour, and its info token is the theme's syntax
// type colour, not the cyan of an info badge.
func toneStyle(tone ui.Tone) string {
	switch tone {
	case ui.ToneNeutral:
		return ""
	}
	return string(tone)
}

// toneText is a line of text drawn in tone. Tern draws a text node's Common.Tone only as chrome, which text has none of, so
// the text is one span in toneStyle(tone); Common.Tone still marks the line's role for tests.
func toneText(key string, tone ui.Tone, text string) ui.Text {
	return ui.Text{Common: ui.Common{Key: key, Tone: tone}, Spans: []ui.Span{{T: text, S: toneStyle(tone)}}}
}
