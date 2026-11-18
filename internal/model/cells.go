package model

import (
	"fmt"
	"time"
)

// OwnerText is an owner cell's text: the owner's name, then " until …" when the claim expires, so a list can cut the
// name and keep the rest whole.
type OwnerText struct{ Name, Until string }

func (o OwnerText) String() string { return o.Name + o.Until }

// OwnerText is the one owner text of list rows, the children table and the Issue detail: the owner, and "until HH:MM"
// when the claim expires today or "until YYYY-MM-DD HH:MM" otherwise, in the Pane's zone. Zero when nobody owns the Issue.
func (m Model) OwnerText(owner *string, expires *time.Time, now time.Time) OwnerText {
	if owner == nil || *owner == "" {
		return OwnerText{}
	}
	if expires == nil {
		return OwnerText{Name: *owner}
	}
	return OwnerText{Name: *owner, Until: " until " + m.Calendar.Read(*expires, now)}
}

// None is the one "no value" glyph, an em dash.
const None = "—"

// Age is how long ago t was, compact and without a space: "7s", "4m", "2h", "3d", "1w". A t ahead of now reads "0s".
// None when t is unset.
func Age(t, now time.Time) string {
	if t.IsZero() {
		return None
	}
	d := max(now.Sub(t), 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}

// RelativeTime is the "updated" cell of a narrow list or table: "2h ago", "3d ago", None when unset. Its hovercard
// carries the absolute time.
func RelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return None
	}
	return Age(t, now) + " ago"
}

type ChildCells struct{ Status, Title, Owner, Updated string }

func (m Model) ChildCells(c ChildRow, now time.Time) ChildCells {
	return ChildCells{Status: c.Status, Title: c.Title, Owner: m.OwnerText(c.Owner, c.OwnerExpires, now).String(), Updated: RelativeTime(c.UpdatedAt, now)}
}
