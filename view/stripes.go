package view

// StripeRole is the role of every other drawn Issue row; StripeCSS shades it.
const StripeRole = "tern-kata.stripe"

// CSS is the plugin's whole style sheet: StripeCSS, SelectedCSS, ModeChipCSS, then OrderedListCSS.
const CSS = StripeCSS + "\n" + SelectedCSS + "\n" + ModeChipCSS + "\n" + OrderedListCSS

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
const StripeCSS = `.sf-item[data-role='` + StripeRole + `']:not(.sel):not(:hover) { background-color: color-mix(in srgb, var(--t1) 5%, transparent); }`

// SelectedCSS paints the selected Issue row (Tern marks it .sel from the list's
// Selected) with the theme's accent fill and white text, the pairing Tern's own
// examples use. Every child is forced to that text so muted (closed) and accent
// spans stay readable.
// shortcut: white assumes a dark accent fill (a light theme accent gives white on light); switch to a Tern on-accent text variable if one appears. !important beats the hover; scoped to the Issue list.
const SelectedCSS = `.sf-item.sel[data-id^='` + IssueListID + `.'] { background-color: var(--accent-fill) !important; color: #fff !important; }
.sf-item.sel[data-id^='` + IssueListID + `.'] * { color: #fff !important; }`
