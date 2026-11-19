package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// laneIdentityTones are the tones a Lane's identity colour cycles through: the three that carry no other meaning (rule 2), so a
// Lane's colour never reads as a state. The Lane's name is always drawn beside it.
var laneIdentityTones = []ui.Tone{ui.ToneInfo, ui.ToneUser, ui.ToneNeutral}

// quietLane is a Lane that has no identity colour: Done and the No lane are finished or empty, so they are muted.
func quietLane(lane string) bool { return lane == model.LaneDone || lane == model.LaneNone }

// laneTone is the identity colour of lane among lanes: Done and the No lane are muted, and every other Lane takes the tone of
// its index among the Lanes that are not, cycling laneIdentityTones. A Lane that is not in lanes is neutral.
func laneTone(lanes []string, lane string) ui.Tone {
	if quietLane(lane) {
		return ui.ToneMuted
	}
	index := 0
	for _, l := range lanes {
		if l == lane {
			return laneIdentityTones[index%len(laneIdentityTones)]
		}
		if !quietLane(l) {
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

// priorityText is a priority's text and tone: P0 error, P1 warning, the lower ones muted so only an urgent Issue stands out.
func priorityText(p *int64) (string, ui.Tone) {
	if p == nil {
		return "", ui.ToneMuted
	}
	text := model.PriorityText(p)
	switch *p {
	case 0:
		return text, ui.ToneError
	case 1:
		return text, ui.ToneWarning
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

// labelChip is a label as a neutral chip: a label names a thing and carries no colour meaning.
func labelChip(text string) ui.Badge {
	return ui.Badge{Common: ui.Common{Key: "label-" + text, Tone: ui.ToneNeutral}, Text: text}
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
