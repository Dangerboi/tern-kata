//go:build windows

package data

// osZone has no Windows source yet, so the calendar falls back to UTC with a note.
func osZone() string { return "" }
