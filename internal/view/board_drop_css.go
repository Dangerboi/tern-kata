package view

// BoardDropCSS replaces Tern's mark of a drop target. Tern draws a 2 px accent bar at the target's edge, which on a Lane column
// reads as inserting a Lane between columns. The Pane lights the whole target Lane instead, a tint extended past it with a
// ring, and shows nothing on the Lane of the card being dragged (it holds the dragging list), where a drop changes nothing.
//
// Tern draws the lifted card inside its own Lane's scrolling list, so the list's scroll box, the Lane column and the panel (a
// card beside the detail column), each clipping what overflows it, would hide the card over any other Lane. While a list drags,
// those boxes stop clipping. A scroll box that stops clipping stops scrolling too: a scrolled Lane shows from its top during the
// drag and stays there after the drop, and its lower cards show below the Board until the release. No style keeps both, so the
// Board takes that cost for a dragged card that stays visible.
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
}
.sf-list.sf-dragging > .sf-list-scroll,
.sf-clip:has(.sf-list.sf-dragging) {
	overflow: visible !important;
}`
