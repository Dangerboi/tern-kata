package model

import (
	"errors"
	"fmt"
	"math"
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

// CheckLabel turns a name typed as "In review" into the kata label `in-review`, or says why it leaves no label (empty, or
// over kata's 64 characters).
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

// SharedLanesKey is the Project metadata key a Project shares its Lane list under, in tern-kata's own namespace: the fields of
// a lanes.toml table, {"labels": [...], "done_days": N, "done_cards": N}. kata stores it without reading it.
const SharedLanesKey = "tern.lanes"

// SharedLanes reads a tern.lanes value with the Lane rules: every label one CheckLabel accepts (and tidies), none twice, and
// each Done limit, when present, a whole number from 1 to its maximum. No labels is the default list and an absent limit
// the default. Anything else is refused with why.
func SharedLanes(value any) (lanes []string, doneDays, doneCards int, err error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, 0, 0, errors.New("it is not an object")
	}
	if raw, held := object["labels"]; held && raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return nil, 0, 0, errors.New("labels is not a list")
		}
		for _, entry := range list {
			text, ok := entry.(string)
			if !ok {
				return nil, 0, 0, fmt.Errorf("label %v is not text", entry)
			}
			label, err := CheckLabel(text)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("label %q %w", shortEntry(text), err)
			}
			if slices.Contains(lanes, label) {
				return nil, 0, 0, fmt.Errorf("label %q appears twice", label)
			}
			lanes = append(lanes, label)
		}
	}
	if doneDays, err = sharedLimit(object, "done_days", maxDoneDays); err == nil {
		doneCards, err = sharedLimit(object, "done_cards", maxDoneCards)
	}
	if err != nil {
		return nil, 0, 0, err
	}
	return lanes, doneDays, doneCards, nil
}

// sharedLimit is the Done limit at key of a tern.lanes value: 0, the default, when absent.
func sharedLimit(object map[string]any, key string, most int) (int, error) {
	raw, held := object[key]
	if !held || raw == nil {
		return 0, nil
	}
	var n float64
	switch v := raw.(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	default:
		return 0, fmt.Errorf("%s is not a number", key)
	}
	if n != math.Trunc(n) || n < 1 || n > float64(most) {
		return 0, fmt.Errorf("%s must be a whole number from 1 to %d", key, most)
	}
	return int(n), nil
}

// SharedLanesValue is the tern.lanes value of a Lane list and Done's limits: the labels always, so an empty list shares the
// default Lanes, and each limit only when it is set.
func SharedLanesValue(lanes []string, doneDays, doneCards int) map[string]any {
	value := map[string]any{"labels": append([]string{}, lanes...)}
	if doneDays > 0 {
		value["done_days"] = doneDays
	}
	if doneCards > 0 {
		value["done_cards"] = doneCards
	}
	return value
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
