package view

// StripeRole is the role of every other drawn Issue row; StripeCSS shades it.
const StripeRole = "tern-kata.stripe"

// StripeCSS is the plugin's style sheet, installed once on the surface (Tern
// confines it to the surface). The shade mixes the theme's text colour into the
// row, so it is a light tint on a dark theme and a dark one on a light theme.
// :not(.sel) leaves the selected row's own highlight, :not(:hover) the hover's.
const StripeCSS = `.sf-item[data-role='` + StripeRole + `']:not(.sel):not(:hover) { background-color: color-mix(in srgb, var(--t1) 5%, transparent); }`
