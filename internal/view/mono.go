package view

import (
	"regexp"
	"strings"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// MonoRole marks a badge whose text is a machine string (an Issue ref, a priority, a date); MonoCSS draws it in the mono
// face, as the `mono` span token does for a span, since a badge takes no span styles.
const MonoRole = "tern-kata.mono"

// MonoCSS draws a badge with MonoRole in the surface's mono face, scaled to the badge's 11px sans text as Tern scales mono
// spans (12.5px) to its 14px sans text, so a mono badge reads no larger than the sans badges beside it.
const MonoCSS = `.sf-badge[data-role~='` + MonoRole + `'] { font-family: var(--mono); font-size: 10px; }`

// mono adds the mono span token to a style.
func mono(style string) string { return strings.TrimSpace(style + " mono") }

// machineString matches the machine strings inside prose: a day with its time, and a word with a digit in it (a count, an id, a
// duration, a version) with the "ago" of a relative time.
var machineString = regexp.MustCompile(`\d{4}-\d\d-\d\d(?: \d\d:\d\d)?|\d+[smhd](?: \d+[smhd])*(?: ago)?|#?[\w.:/\-]*\d[\w.:/\-]*(?: ago)?`)

// monoSpans is text as spans of style, with its machine strings also in the mono face; the spans' text joins to text.
func monoSpans(text, style string) []ui.Span {
	var spans []ui.Span
	at := 0
	for _, loc := range machineString.FindAllStringIndex(text, -1) {
		if loc[0] > at {
			spans = append(spans, ui.Span{T: text[at:loc[0]], S: style})
		}
		spans = append(spans, ui.Span{T: text[loc[0]:loc[1]], S: mono(style)})
		at = loc[1]
	}
	if at < len(text) {
		spans = append(spans, ui.Span{T: text[at:], S: style})
	}
	return spans
}
