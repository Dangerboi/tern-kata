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
	loc := m.zone
	if loc == nil {
		loc = time.UTC
	}
	at := expires.In(loc)
	layout := "2006-01-02 15:04"
	if y, mo, d := now.In(loc).Date(); at.Year() == y && at.Month() == mo && at.Day() == d {
		layout = "15:04"
	}
	return OwnerText{Name: *owner, Until: " until " + at.Format(layout)}
}

// RelativeTime is kata's TUI "updated" cell: "2h ago", "3d ago", "-" when unset.
func RelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	}
}

// ChildCells are a loaded child's status, title, owner and updated cells as the Children table shows them.
type ChildCells struct{ Status, Title, Owner, Updated string }

// ChildCells shapes a loaded child's row for the table.
func (m Model) ChildCells(c ChildRow, now time.Time) ChildCells {
	return ChildCells{Status: c.Status, Title: c.Title, Owner: m.OwnerText(c.Owner, c.OwnerExpires, now).String(), Updated: RelativeTime(c.UpdatedAt, now)}
}
