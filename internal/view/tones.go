package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var (
	namedLaneTones = map[string]ui.Tone{
		model.LaneNone:    ui.ToneMuted,
		"needs-triage":    ui.ToneWarning,
		"needs-info":      ui.ToneError,
		"ready-for-agent": ui.ToneInfo,
		"ready-for-human": ui.ToneUser,
		model.LaneDone:    ui.ToneSuccess,
	}
	customLaneTones = []ui.Tone{ui.ToneAccent, ui.TonePending, ui.ToneNeutral}
)

func laneTone(lanes []string, lane string) ui.Tone {
	if tone, ok := namedLaneTones[lane]; ok {
		return tone
	}
	custom := 0
	for _, l := range lanes {
		if l == lane {
			return customLaneTones[custom%len(customLaneTones)]
		}
		if _, named := namedLaneTones[l]; !named {
			custom++
		}
	}
	return ui.ToneNeutral
}

func issueLaneTone(m model.Model, issue model.Issue) ui.Tone {
	return issueLaneToneIn(m.LaneList(), issue)
}

func issueLaneToneIn(lanes []string, issue model.Issue) ui.Tone {
	return laneTone(lanes, model.LaneOf(issue, lanes))
}

const ownerTone = ui.ToneUser

const labelTone = ui.ToneInfo

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
	case 2:
		return text, ui.ToneInfo
	}
	return text, ui.ToneMuted
}

// toneStyle is the text span style closest to tone. Spans have no user style, so it takes the accent; pending is Tern's own
// span token for words that wait on something; neutral is the plain text colour.
func toneStyle(tone ui.Tone) string {
	switch tone {
	case ui.ToneUser:
		return string(ui.ToneAccent)
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
