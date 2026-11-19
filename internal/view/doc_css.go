package view

// DocCSS keeps a doc's inner layers inside the doc. Tern gives the doc's text, markers and margin z-indexes of 1 to 3 without
// making the doc a stacking context, so they compete with the layer region that holds every modal and card (which has no
// z-index) and draw above the Menu, the editor and the conflict card. Isolating the doc confines them to it.
//
// The rest tells the parts of the Issue detail apart without a new meaning for any tone, using only the theme's text variables:
// the properties sit in a quiet panel with a rule on its left, their keys and the created line in a brighter muted than the
// body text's, and each comment is a block of its own with the same rule. A body and a comment use the detail's whole width, and
// Markdown headings take the default text colour in strong, with h2 and below at the body size (hierarchy by weight).
const DocCSS = `.sf-doc { isolation: isolate; }
.sf-md :is(h1, h2, h3, h4, h5, h6) { color: inherit; font-weight: 700; }
.sf-md h2, .sf-md h3, .sf-md h4, .sf-md h5, .sf-md h6 { font-size: 1em; }
[data-role~='` + PropertiesRole + `'] { display: grid; grid-template-columns: max-content minmax(0, 1fr); column-gap: 1em; row-gap: 2px; align-items: start; padding: 6px 10px; border-left: 2px solid color-mix(in srgb, var(--t1) 28%, transparent); border-radius: 3px; background: color-mix(in srgb, var(--t1) 10%, transparent); }
[data-role~='` + PropertiesRole + `'] > .sf-row { display: contents; }
[data-role~='` + PropertiesRole + `'] > .sf-row > * { justify-self: start; max-width: 100%; }
[data-role~='` + PropertiesRole + `'] > :not(.sf-row) { grid-column: 1 / -1; color: color-mix(in srgb, var(--t3), var(--t1) 45%); }
[data-role~='` + PropertiesRole + `'] .sf-text[data-role~='` + PropertyKeyRole + `'] { min-width: 0; }
[data-role~='` + PropertiesRole + `'] .sf-t-muted { color: color-mix(in srgb, var(--t3), var(--t1) 45%); }
.sf-card[data-role~='` + CommentRole + `'] { border-left: 3px solid color-mix(in srgb, var(--t1) 45%, transparent); background: color-mix(in srgb, var(--t1) 12%, transparent) !important; border-radius: 4px; padding: 8px 12px; margin: 4px 0; }
.sf-card[data-role~='` + CommentRole + `'] .sf-t-strong { color: var(--t1); font-weight: 700; }
.sf-card[data-role~='` + CommentRole + `'] .sf-md { color: color-mix(in srgb, var(--t1) 82%, var(--t3)); }`

// CommentRole marks a comment's card, which DocCSS draws as a block of its own.
const CommentRole = "tern-kata.comment"
