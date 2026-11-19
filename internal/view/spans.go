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

// KeyStyle is the span style of a keycap. Tern's own "key" style rewrites the text it draws into its notation (ctrl+d becomes ^D),
// and the keycaps here name a chord in words (Ctrl+D). A span has no class of its own and Tern drops a style it does not know, so
// a keycap uses "code", the one chip style Tern draws as typed; the plugin uses it for keycaps only, and KeyCSS gives it the keycap look.
const KeyStyle = "code"

// KeyCSS draws a KeyStyle span as Tern draws its keycap: a mono run on the raised surface with a one-pixel ring, from theme
// variables only.
const KeyCSS = `.sf-t-` + KeyStyle + ` { padding: 0 0.35em; color: var(--sf-c-key, var(--t2)); background: var(--l1); box-shadow: inset 0 0 0 1px var(--l2), inset 0 -1px 0 var(--l1); }`

// keySpan is a key drawn as a keycap, named in words (see KeyWords).
func keySpan(key string) ui.Span { return ui.Span{T: KeyWords(key), S: KeyStyle} }

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
