package view

// DocCSS keeps a doc's inner layers inside the doc. Tern gives the doc's text, markers and margin z-indexes of 1 to 3 without
// making the doc a stacking context, so they compete with the layer region that holds every modal and card (which has no
// z-index) and draw above the Menu, the editor and the conflict card. Isolating the doc confines them to it.
const DocCSS = `.sf-doc { isolation: isolate; }`
