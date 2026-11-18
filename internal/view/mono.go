package view

import "strings"

// MonoRole marks an Issue-id badge (priority and date badges are sans); MonoCSS draws it in the mono
// face, as the `mono` span token does for a span, since a badge takes no span styles.
const MonoRole = "tern-kata.mono"

// MonoCSS draws a badge with MonoRole in the surface's mono face, scaled to the badge's 11px sans text as Tern scales mono
// spans (12.5px) to its 14px sans text, so a mono badge reads no larger than the sans badges beside it.
const MonoCSS = `.sf-badge[data-role~='` + MonoRole + `'] { font-family: var(--mono); font-size: 10px; }`

func mono(style string) string { return strings.TrimSpace(style + " mono") }
