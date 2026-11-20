package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
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

// markdown is Markdown the Pane draws: Issue text written by people, agents and other Projects, so no shell code block offers
// Run, and code block heads are icons.
func markdown(key, text string) ui.Md {
	noRun := false
	return ui.Md{Common: ui.Common{Key: key}, Text: text, Run: &noRun, Icons: true}
}

// prose is text whose model.Key markers are keycaps: each marker becomes keySpan of its key, and everything else, braces included,
// is drawn as typed in style. The markers are private-use runes (see model.KeyOpen), so a file name or an error message in a notice
// can never forge a keycap. A marker with no closing rune is dropped.
func prose(text, style string) []ui.Span {
	var spans []ui.Span
	flush := func(run string) {
		if run != "" {
			spans = append(spans, ui.Span{T: run, S: style})
		}
	}
	for {
		before, rest, found := strings.Cut(text, model.KeyOpen)
		flush(before)
		if !found {
			return spans
		}
		key, after, closed := strings.Cut(rest, model.KeyClose)
		if !closed {
			return spans
		}
		if key != "" {
			spans = append(spans, keySpan(key))
		}
		text = after
	}
}
