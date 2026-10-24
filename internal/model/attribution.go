package model

import "strings"

// Attribution is the creation attribution kata returns for an Issue or a comment. Verification is empty when kata gave none.
type Attribution struct {
	Verification string
	Accountable  string
	Source       string
	Teammate     string
}

// Line is kata tui's creation line: "creation: verified|pending|legacy" (an unknown state reads legacy), "accountable: <actor>"
// only when verified and set, and "source: <source actor or author>" with " / <teammate>" when there is one. It is empty when kata
// returned no verification state.
func (a Attribution) Line(author string) string {
	if a.Verification == "" {
		return ""
	}
	state := a.Verification
	if state != "verified" && state != "pending" {
		state = "legacy"
	}
	fields := []string{"creation: " + state}
	if state == "verified" && a.Accountable != "" {
		fields = append(fields, "accountable: "+a.Accountable)
	}
	source := a.Source
	if source == "" {
		source = author
	}
	if a.Teammate != "" {
		source += " / " + a.Teammate
	}
	return strings.Join(append(fields, "source: "+source), " · ")
}
