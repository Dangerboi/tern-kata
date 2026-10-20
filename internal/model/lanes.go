package model

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The pure Lane rules: functions of an Issue and a Lane list (a Project's ordered Lane labels) alone.
//
// A Lane is named by its label. The two columns that are not a label have keys no label can have: kata labels hold
// only [a-z0-9._:-], so neither "" nor a key with a space is ever a label.
const (
	// LaneNone is the key of No lane: open Issues with no Lane label.
	LaneNone = ""
	// LaneDone is the key of Done: recently closed Issues.
	LaneDone = " done"
)

// DefaultLanes is the Lane list of a Project that has none saved.
func DefaultLanes() []string {
	return []string{"needs-triage", "needs-info", "ready-for-agent", "ready-for-human"}
}

// kata's label rule: 1 to 64 characters of [a-z0-9._:-].
var labelJunk = regexp.MustCompile(`[^a-z0-9._:-]`)

const maxLabelLen = 64

// NormalizeLabel turns a name typed as "In review" into the kata label `in-review`. It reports false for a name that
// leaves no label (empty, or over kata's 64 characters).
func NormalizeLabel(text string) (string, bool) {
	label, err := CheckLabel(text)
	return label, err == nil
}

// CheckLabel is NormalizeLabel with the reason a name leaves no label.
func CheckLabel(text string) (string, error) {
	label := labelJunk.ReplaceAllString(strings.Join(strings.Fields(strings.ToLower(text)), "-"), "")
	switch {
	case label == "":
		return "", errors.New("leaves no label")
	case utf8.RuneCountInString(label) > maxLabelLen:
		return "", fmt.Errorf("is longer than %d characters", maxLabelLen)
	}
	return label, nil
}

// LaneLabels are the Lane labels of issue, in Lane order.
func LaneLabels(issue Issue, lanes []string) []string {
	var found []string
	for _, lane := range lanes {
		if slices.Contains(issue.Labels, lane) {
			found = append(found, lane)
		}
	}
	return found
}

// LabelLane is the Lane the labels of issue point at, open or closed: the first of its Lane labels in Lane order, else LaneNone.
func LabelLane(issue Issue, lanes []string) string {
	if found := LaneLabels(issue, lanes); len(found) > 0 {
		return found[0]
	}
	return LaneNone
}

// LaneOf is the column issue stands in: Done when it is closed, else the Lane its labels point at.
func LaneOf(issue Issue, lanes []string) string {
	if IsClosed(issue) {
		return LaneDone
	}
	return LabelLane(issue, lanes)
}

// LaneTitle is the heading of a Lane: "ready-for-agent" reads "Ready for agent".
func LaneTitle(lane string) string {
	switch lane {
	case LaneNone:
		return "No lane"
	case LaneDone:
		return "Done"
	}
	title := []rune(strings.NewReplacer("-", " ", "_", " ").Replace(lane))
	if len(title) > 0 {
		title[0] = unicode.ToUpper(title[0])
	}
	return string(title)
}
