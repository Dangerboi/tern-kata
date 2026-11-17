// Package ternkata holds what the tern-kata binary carries from the repository root: its changelog, which a new version's first
// Pane shows.
package ternkata

import _ "embed"

//go:embed CHANGELOG.md
var Changelog string
