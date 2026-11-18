package view

import (
	"strings"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// monoSpan is text in the mono face. Mono is explicit: a caller names the text that is an id, a time, a path or a command, and
// nothing is guessed from the text's shape.
func monoSpan(text, style string) ui.Span { return ui.Span{T: text, S: mono(style)} }

// refSpan is an Issue id or ref; link draws it as a link.
func refSpan(ref string, link bool) ui.Span {
	if link {
		return monoSpan(ref, "accent link")
	}
	return monoSpan(ref, "")
}

// timeSpan is a timestamp or a duration: mono and muted.
func timeSpan(text string) ui.Span { return monoSpan(text, "muted") }

// keySpan is a key drawn as Tern's keycap.
func keySpan(key string) ui.Span { return ui.Span{T: key, S: "key"} }

// prose is text whose "{W}" markers are keycaps: each marker becomes keySpan("W"), "{{" is a literal brace, and the rest carries
// style. A "{" with no closing "}" is literal text. The model names a key with model.Key so a notice never spells one by hand.
func prose(text, style string) []ui.Span {
	var spans []ui.Span
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			spans = append(spans, ui.Span{T: run.String(), S: style})
			run.Reset()
		}
	}
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			run.WriteByte(text[i])
			continue
		}
		if i+1 < len(text) && text[i+1] == '{' {
			run.WriteByte('{')
			i++
			continue
		}
		end := strings.IndexByte(text[i+1:], '}')
		if end <= 0 {
			run.WriteByte('{')
			continue
		}
		flush()
		spans = append(spans, keySpan(text[i+1:i+1+end]))
		i += end + 1
	}
	flush()
	return spans
}
