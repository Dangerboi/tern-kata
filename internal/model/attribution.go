package model

import "strings"

// Attribution is the creation attribution kata returns for an Issue or a comment. Verification is empty when kata gave none.
type Attribution struct {
	Verification string
	Accountable  string
	Source       string
	Teammate     string
}

// Line is the creation line under an Issue's properties and on each comment. Verified and pending read as kata tui's
// "creation: verified|pending", then "accountable: <actor>" only when verified and set, then "source: <source actor or
// author>" with " / <teammate>" when there is one. A legacy record (an unknown state counts as legacy) drops the state, since
// every record on an unfederated Project is legacy, and reads "source: …" only when it has a teammate or a source actor other
// than the author; otherwise it is empty. It is empty when kata returned no verification state.
func (a Attribution) Line(author string) string {
	if a.Verification == "" {
		return ""
	}
	state := a.Verification
	var fields []string
	if state == "verified" || state == "pending" {
		fields = append(fields, "creation: "+state)
	} else if a.Teammate == "" && (a.Source == "" || a.Source == author) {
		return ""
	}
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
