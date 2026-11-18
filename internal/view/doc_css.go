package view

// DocCSS keeps a doc's inner layers inside the doc. Tern gives the doc's text, markers and margin z-indexes of 1 to 3 without
// making the doc a stacking context, so they compete with the layer region that holds every modal and card (which has no
// z-index) and draw above the Menu, the editor and the conflict card. Isolating the doc confines them to it.
//
// A body or comment is capped at 70 columns of text so a line stays readable in a wide detail, and its Markdown headings take the
// default text colour in strong: hierarchy by weight, not by colour.
const DocCSS = `.sf-doc { isolation: isolate; }
.sf-md, .sf-doc { max-width: 70ch; }
.sf-md :is(h1, h2, h3, h4, h5, h6) { color: inherit; font-weight: 700; }
.sf-md h2, .sf-md h3, .sf-md h4, .sf-md h5, .sf-md h6 { font-size: 1em; }`
