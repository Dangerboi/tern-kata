package view

import (
	"strconv"
	"strings"

	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// pageHead is the one Page head: the title in strong text, then the facts on the same line in muted text, joined by " · ".
// A fact that is empty is left out, so a caller passes its optional facts unconditionally; a count is a bare number string
// ("3"). With no facts the head is the title alone. Callers pass their existing key so the node ids under it stay.
func pageHead(key, title string, facts ...string) tern.Element {
	var shown []string
	for _, f := range facts {
		if f != "" {
			shown = append(shown, f)
		}
	}
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: title, S: "strong"}}, Wrap: ui.WrapNone})
	if len(shown) > 0 {
		children = append(children, ui.Text{Common: ui.Common{Key: "facts", Tone: ui.ToneMuted}, Text: strings.Join(shown, " · "), Wrap: ui.WrapNone})
	}
	return ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: children}
}

// countSpan is a muted bare number after a tab or heading label ("Events 4"); a zero count draws nothing, so the zero value
// span is returned and callers drop it with their other empty spans.
func countSpan(n int) ui.Span {
	if n == 0 {
		return ui.Span{}
	}
	return ui.Span{T: strconv.Itoa(n), S: "muted"}
}
