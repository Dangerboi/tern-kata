package view

// BoardDropCSS replaces Tern's mark of a drop target. Tern draws a 2 px accent bar at the target's edge, which on a Lane column
// reads as inserting a Lane between columns. The Pane lights the whole target Lane instead, a tint extended past it with a
// ring, and shows nothing on the Lane of the card being dragged (it holds the dragging list), where a drop changes nothing.
const BoardDropCSS = `.sf-col[data-role~='` + LaneRole + `'].sf-drop-at {
	box-shadow: none;
}
.sf-col[data-role~='` + LaneRole + `'].sf-drop-at:not(:has(.sf-dragging)) {
	--lane-lit: rgb(from var(--accent) r g b / 9%);
	background: var(--lane-lit);
	box-shadow: 0 0 0 6px var(--lane-lit);
	outline: 1px solid rgb(from var(--accent) r g b / 45%);
	outline-offset: 6px;
	border-radius: var(--r-card);
}`

// CardMoreRole marks the more menu button of the selected Board card.
const CardMoreRole = "tern-kata.card-more"

// CardMoreCSS pushes the more button to the right end of the card's first row. In a wrapping row an auto margin keeps the
// button at the right end of whichever line it lands on.
const CardMoreCSS = `[data-role~='` + CardMoreRole + `'] {
	margin-left: auto;
}`
