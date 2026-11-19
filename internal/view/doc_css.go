package view

// DocCSS keeps a doc's inner layers inside the doc. Tern gives the doc's text, markers and margin z-indexes of 1 to 3 without
// making the doc a stacking context, so they compete with the layer region that holds every modal and card (which has no
// z-index) and draw above the Menu, the editor and the conflict card. Isolating the doc confines them to it.
//
// The rest tells the parts of the Issue detail apart without a new meaning for any tone and without touching layout: the facts panel
// and each comment sit on one soft surface (the theme's text colour at 7%, so it follows light and dark) with the radius of the
// Pane's cards, and their text keeps the normal tones. A body and a comment use the detail's whole width, and Markdown headings
// take the default text colour in strong, with h2 and below at the body size (hierarchy by weight).
const DocCSS = `.sf-doc { isolation: isolate; }
.sf-md :is(h1, h2, h3, h4, h5, h6) { color: inherit; font-weight: 700; }
.sf-md h2, .sf-md h3, .sf-md h4, .sf-md h5, .sf-md h6 { font-size: 1em; }
[data-role~='` + PropertiesRole + `'], .sf-card[data-role~='` + CommentRole + `'] { background-color: color-mix(in srgb, var(--t1) 7%, transparent) !important; border-radius: var(--r-card, 10px) !important; padding: 8px 12px; }
.sf-card[data-role~='` + CommentRole + `'] { margin: 4px 0; }
.sf-col[data-role~='` + DetailContentRole + `'] { padding-right: 12px; }
.sf-card[data-role~='` + CommentRole + `'] .sf-card-body { padding-left: 0 !important; margin-left: 0 !important; }`

// DetailContentRole marks the detail's scrolled content; DocCSS leaves a gutter on its right for the scrollbar, which would
// otherwise be drawn over the right edge of the comment cards.
const DetailContentRole = "tern-kata.detailcontent"

// CommentRole marks a comment's card, which DocCSS draws as a block of its own.
const CommentRole = "tern-kata.comment"
