package view

import (
	"fmt"
	"strings"
)

// StripeRole is the role of every other drawn Issue row and Children row; StripeCSS shades it.
const StripeRole = "tern-kata.stripe"

// The roles of an Issue list row besides StripeRole, joined to it with spaces: ListRowRole marks a row (and the header), and
// ListIDRole and ListOwnerRole are followed by the width of that column in characters. cellBoxCSS reads them.
const (
	ListRowRole   = "tern-kata.row"
	ListIDRole    = "tern-kata.id"
	ListOwnerRole = "tern-kata.ow"
)

// CSS is the plugin's whole style sheet: StripeCSS, SelectedCSS, ModeChipCSS, OrderedListCSS, then cellBoxCSS.
var CSS = StripeCSS + "\n" + SelectedCSS + "\n" + ModeChipCSS + "\n" + OrderedListCSS + "\n" + cellBoxCSS

// ModeChipCSS hides the mode label Tern draws at the end of an input. Every
// single-line input sets a mode so Tern's native editing stays off (the model
// handles the keys), and the label means nothing to the user.
const ModeChipCSS = `.sf-ed-mode { display: none !important; }`

// OrderedListCSS gives ordered lists in markdown room for a three-character
// marker ("10."); Tern's default start padding clips two-digit numbers.
const OrderedListCSS = `.md ol { padding-left: 40px !important; }`

// StripeCSS is the plugin's style sheet, installed once on the surface (Tern
// confines it to the surface). The shade mixes the theme's text colour into the
// row, so it is a light tint on a dark theme and a dark one on a light theme.
// :not(.sel) leaves the selected row's own highlight, :not(:hover) the hover's.
const StripeCSS = `.sf-item[data-role~='` + StripeRole + `']:not(.sel):not(:hover), .sf-row[data-role~='` + StripeRole + `']:not(:hover) { background-color: color-mix(in srgb, var(--t1) 5%, transparent); }`

// SelectedCSS paints the selected Issue row (Tern marks it .sel from the list's
// Selected) with the theme's accent fill and white text, the pairing Tern's own
// examples use. Every child is forced to that text so muted (closed) and accent
// spans stay readable.
// shortcut: white assumes a dark accent fill (a light theme accent gives white on light); switch to a Tern on-accent text variable if one appears. !important beats the hover; scoped to the Issue list.
const SelectedCSS = `.sf-item.sel[data-id^='` + IssueListID + `.'] { background-color: var(--accent-fill) !important; color: #fff !important; }
.sf-item.sel[data-id^='` + IssueListID + `.'] * { color: #fff !important; }`

// cellEm is a character's advance in em: the surface CSS has no ch unit, and the Pane's monospace font advances 0.6em.
const cellEm = 0.6

// maxBoxWidth is the widest id or owner column cellBoxCSS has a rule for; a wider one is drawn unboxed.
const maxBoxWidth = 80

// cellBoxCSS gives each cell of an Issue list row a box as wide as its column. Tern draws a glyph the font lacks, such as
// CJK, from a fallback font whose advance is not the character cell's, so padding a name with spaces leaves its column
// narrower than the header's and shifts every cell right of it; a box of the column's width does not move. The id and owner
// widths vary with the Issues, so a row names them in its role (see listColumns.role); the others are fixed.
// min-width, not width: a cell whose text outgrows its box widens instead of overlapping the next.
var cellBoxCSS = func() string {
	var css strings.Builder
	box := func(selector string, chars int) {
		fmt.Fprintf(&css, "%s { display: inline-block; white-space: pre; min-width: %.1fem; }\n", selector, float64(chars)*cellEm)
	}
	row := ".sf-item[data-role~='" + ListRowRole + "']"
	gap := displayWidth(cellGap)
	box(row+" .sf-item-label > span:nth-child(2)", gap+prioWidth)
	box(row+" .sf-item-label > span:nth-child(3)", gap+statusWidth)
	box(row+" .sf-item-value > span:first-child", kidsWidth)
	box(row+" .sf-item-value > span:last-child", gap+updatedWidth)
	for n := 1; n <= maxBoxWidth; n++ {
		box(fmt.Sprintf(".sf-item[data-role~='%s%d'] .sf-item-label > span:nth-child(1)", ListIDRole, n), n)
		box(fmt.Sprintf(".sf-item[data-role~='%s%d'] .sf-item-value > span:nth-child(2)", ListOwnerRole, n), gap+n)
	}
	return css.String()
}()
