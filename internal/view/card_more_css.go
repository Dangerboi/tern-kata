package view

// CardMoreRole marks the more menu button of the selected Board card.
const CardMoreRole = "tern-kata.card-more"

// CardMoreCSS puts the more button at the card's top right corner, out of the badge row's flow, so it can never wrap alone
// onto a second line. The card is positioned in Tern, so the button's offsets match the card's padding, and the row that holds
// the button keeps that much room on the right: the button's width and a gap. Tern draws a small button 28 px wide.
const CardMoreCSS = `[data-role~='` + CardMoreRole + `'] {
	position: absolute;
	top: 6px;
	right: 8px;
}
.sf-row:has(> [data-role~='` + CardMoreRole + `']) {
	padding-right: 36px;
}`
