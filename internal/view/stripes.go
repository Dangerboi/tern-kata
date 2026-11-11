package view

import (
	"fmt"
	"strings"
)

const StripeRole = "tern-kata.stripe"

// The roles of an Issue list row besides StripeRole, joined to it with spaces: ListRowRole marks a row (and the header), and
// ListIDRole and ListOwnerRole are followed by the width of that column in characters. cellBoxCSS reads them.
const (
	ListRowRole   = "tern-kata.row"
	ListIDRole    = "tern-kata.id"
	ListOwnerRole = "tern-kata.ow"
)

var CSS = StripeCSS + "\n" + SelectedCSS + "\n" + ModeChipCSS + "\n" + OrderedListCSS + "\n" + ColumnFillCSS + "\n" + PropertyKeyCSS + "\n" + PropertiesCSS + "\n" + MonoCSS + "\n" + cellBoxCSS + "\n" + TwoLineRowCSS

const PropertyKeyRole = "tern-kata.propkey"

// PropertyKeyCSS gives every property key one box, wide enough for the longest label ("Scheduled:", "Recurring:", "Cron jobs:")
// in either face, so every value starts at the same x. The box is in em because the label's own width depends on the face.
const PropertyKeyCSS = `.sf-text[data-role~='` + PropertyKeyRole + `'] { display: inline-block; white-space: pre; min-width: 6.4em; }`

const PropertiesRole = "tern-kata.props"

// PropertiesCSS draws the grid's badges 2px shorter than Tern's 18px, the height of a text row, centred in the row by a 1px
// margin, so the badges of rows one under another (Owner, Priority, Labels) keep a gap instead of touching.
const PropertiesCSS = `[data-role~='` + PropertiesRole + `'] .sf-badge { height: 16px; margin: 1px 0; }`

const (
	ColumnRole = "tern-kata.column"
	FillRole   = "tern-kata.fill"
)

// ColumnFillCSS makes each column's scrolling list fill its column and scroll inside it, so the Pane has nothing to scroll and
// the top bar stays put. Tern has no fill for a list, only a cap, and a card's body sizes to its content: the rules stretch a
// column card's body to the height the columns row gives the card, let its scrolling lists (a Board Lane's too) take the room
// left, and lift their cap.
// shortcut: .sf-card-body and .sf-list-scroll are Tern's inner classes; switch to a Tern prop for a list that fills its column
// if one appears.
const ColumnFillCSS = `.sf-card[data-role~='` + ColumnRole + `'] { display: flex !important; flex-direction: column !important; }
.sf-card[data-role~='` + ColumnRole + `'] > .sf-card-body { flex: 1 1 0 !important; min-height: 0 !important; display: flex !important; flex-direction: column !important; }
.sf-card[data-role~='` + ColumnRole + `'] .sf-list[data-role~='` + FillRole + `'] { flex: 1 1 0 !important; min-height: 0 !important; display: flex !important; flex-direction: column !important; }
.sf-card[data-role~='` + ColumnRole + `'] .sf-list[data-role~='` + FillRole + `'] > .sf-list-scroll { flex: 1 1 0 !important; min-height: 0 !important; max-height: none !important; }`

// ModeChipCSS hides the mode label Tern draws at the end of an input. Every
// single-line input sets a mode so Tern's native editing stays off (the model
// handles the keys), and the label means nothing to the user.
const ModeChipCSS = `.sf-ed-mode { display: none !important; }`

// OrderedListCSS gives ordered lists in markdown room for a three-character
// marker ("10."); Tern's default start padding clips two-digit numbers.
const OrderedListCSS = `.md ol { padding-left: 40px !important; }`

// StripeCSS is the plugin's style sheet, installed once on the surface (Tern
// confines it to the surface). The shades mix the theme's text colour into the
// row, so they are light tints on a dark theme and dark ones on a light theme: 5%
// for a striped row, 12% for the row under the pointer, striped or not. Tern's own
// hover is hardly apart from the stripe, so the rule sets its own, and has the
// specificity to win. The darker hover leaves Tern's muted text (--t3) under 4.5:1 in a
// light theme, so a hovered row's muted spans move 40% toward --t1. :not(.sel) leaves
// the selected row's own highlight and white text; :not(:hover) leaves the hover of
// rows that are not Issue rows (Workflows) to Tern.
const StripeCSS = `.sf-item[data-role~='` + StripeRole + `']:not(.sel):not(:hover), .sf-row[data-role~='` + StripeRole + `']:not(:hover) { background-color: color-mix(in srgb, var(--t1) 5%, transparent); }
.sf-item[data-role~='` + ListRowRole + `']:not(.sel):hover { background-color: color-mix(in srgb, var(--t1) 12%, transparent); }
.sf-item[data-role~='` + ListRowRole + `']:not(.sel):hover .sf-t-muted { color: color-mix(in srgb, var(--t3), var(--t1) 40%); }`

// SelectedCSS paints the selected Issue row (Tern marks it .sel from the list's
// Selected) with the theme's accent fill and white text, the pairing Tern's own
// examples use. Every child is forced to that text so muted (closed) and accent
// spans stay readable.
// shortcut: white assumes a dark accent fill (a light theme accent gives white on light); switch to a Tern on-accent text variable if one appears. !important beats the hover; scoped to the Issue list.
const SelectedCSS = `.sf-item.sel[data-id^='` + IssueListID + `.'] { background-color: var(--accent-fill) !important; color: #fff !important; }
.sf-item.sel[data-id^='` + IssueListID + `.'] * { color: #fff !important; }`

// cellEm is a column character's width in em: the surface CSS has no ch unit, and the mono face the cells use advances 0.6em.
const cellEm = 0.6

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

const (
	TwoLineRowRole      = "tern-kata.row2"
	TwoLineSelectedRole = "tern-kata.row2sel"
)

// TwoLineRowCSS gives the two-line row, a Col, the padding of an Item and the selected look of SelectedCSS, which only reaches
// Items. It also shades striped rows and the row under the pointer as StripeCSS does Items.
const TwoLineRowCSS = `.sf-col[data-role~='` + TwoLineRowRole + `'] { padding: 2px 8px; }
.sf-col[data-role~='` + StripeRole + `']:not([data-role~='` + TwoLineSelectedRole + `']) { background-color: color-mix(in srgb, var(--t1) 5%, transparent); }
.sf-col[data-role~='` + TwoLineRowRole + `']:not([data-role~='` + TwoLineSelectedRole + `']):hover { background-color: color-mix(in srgb, var(--t1) 12%, transparent); }
.sf-col[data-role~='` + TwoLineRowRole + `']:not([data-role~='` + TwoLineSelectedRole + `']):hover .sf-t-muted { color: color-mix(in srgb, var(--t3), var(--t1) 40%); }
.sf-col[data-role~='` + TwoLineSelectedRole + `'], .sf-col[data-role~='` + TwoLineSelectedRole + `'] * { background-color: var(--accent-fill) !important; color: #fff !important; }`
