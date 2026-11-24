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
