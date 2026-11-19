package view

import (
	"strings"
	"testing"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func rowCols() listColumns {
	return listColumns{id: 5, owner: ownerWidth, prioShown: true, statusShown: true, kidsShown: true}
}

func flagged(flags ...string) rowParts {
	parts := rowParts{
		id:     ui.Span{T: " dddd"},
		prio:   ui.Span{T: "P1"},
		status: ui.Span{T: "open"},
		title:  "A rather long Dated title",
	}
	for _, flag := range flags {
		parts.flags = append(parts.flags, ui.Span{T: flag})
	}
	return parts
}

func labelText(row listRow) string {
	var b strings.Builder
	for _, span := range row.label {
		b.WriteString(span.T)
	}
	return b.String()
}

// The flags follow the title; the Agent badge goes first when the whole row does not fit and the attention word stays.
func TestLayoutRowDropsTheAgentBadgeBeforeTheAttentionWord(t *testing.T) {
	parts := flagged("agent working", "needs review")
	parts.agentFlags = 1
	wide := labelText(layoutRow(rowCols(), 200, parts))
	if !strings.Contains(wide, "agent working") || !strings.HasSuffix(wide, cellGap+"needs review") {
		t.Errorf("wide row %q, want the title then both flags", wide)
	}
	narrow := labelText(layoutRow(rowCols(), 70, parts))
	if strings.Contains(narrow, "agent working") || !strings.HasSuffix(narrow, cellGap+"needs review") {
		t.Errorf("narrow row %q, want the Agent badge dropped and the attention word kept", narrow)
	}
}

// A title that still does not fit once the Agent badge is gone is cut, so the attention word is never pushed out.
func TestLayoutRowCutsTheTitleBeforeTheAttentionWord(t *testing.T) {
	parts := flagged("needs review")
	parts.title = strings.Repeat("long ", 30)
	laid := labelText(layoutRow(rowCols(), 70, parts))
	if !strings.HasSuffix(laid, cellGap+"needs review") || !strings.Contains(laid, "…") {
		t.Errorf("row %q, want a cut title and the attention word", laid)
	}
	if got := rowPadding + itemDetailWidth + displayWidth(laid); got > 70 {
		t.Errorf("label is %d cells with the padding, want at most 70", got)
	}
}

func TestLayoutHeaderLabelsOnlyTheColumnsShown(t *testing.T) {
	full := layoutHeader(rowCols())
	text := labelText(full)
	for _, word := range []string{"ID", "Priority", "Status", "Title"} {
		if !strings.Contains(text, word) {
			t.Errorf("header %q lacks %q", text, word)
		}
	}
	bare := labelText(layoutHeader(listColumns{id: 5}))
	if strings.Contains(bare, "Priority") || strings.Contains(bare, "Status") {
		t.Errorf("header %q labels hidden columns", bare)
	}
	if got := strings.Fields(strings.ReplaceAll(labelText(listRow{label: layoutHeader(rowCols()).value}), "\u200b", "")); strings.Join(got, " ") != "Children Updated" {
		t.Errorf("right header %q, want Children Updated", got)
	}
}

func TestListColumnsRoleNamesHiddenColumnsAndTheDateWidth(t *testing.T) {
	role := listColumns{id: 4, date: 9}.role()
	for _, want := range []string{ListNoPrioRole, ListNoStatusRole, ListNoKidsRole, ListDateRole + "9"} {
		if !strings.Contains(role, want) {
			t.Errorf("role %q lacks %q", role, want)
		}
	}
	if role := rowCols().role(); strings.Contains(role, "tern-kata.no") {
		t.Errorf("role %q hides a column that is shown", role)
	}
}

func TestLayoutTwoLineHoldsTheDateChildrenAndOwnerAtTheEnd(t *testing.T) {
	parts := flagged("needs review")
	parts.date = ui.Span{T: "due today"}
	parts.kids, parts.owner = "1/2", "ada"
	laid := layoutTwoLine(parts)
	var meta string
	for _, span := range laid.meta {
		meta += span.T
	}
	if meta != "due today · 1/2 · ada" {
		t.Errorf("meta %q", meta)
	}
	var head string
	for _, span := range laid.head {
		head += span.T
	}
	if strings.Contains(head, "open") {
		t.Errorf("first line %q names the status of an open Issue", head)
	}
	parts.status = ui.Span{T: "closed"}
	if head := layoutTwoLine(parts).head; !strings.Contains(head[len(head)-1].T, "closed") {
		t.Errorf("first line %v lacks the closed status", head)
	}
}

// A narrow list drops the Date column, so one dated row cannot squeeze every other row's title and cells; the badge then follows
// its own row's title and is the first flag to go when that row is too wide.
func TestFitDateDropsTheColumnWhereTheTitleWouldBeSqueezed(t *testing.T) {
	cols := rowCols()
	cols.date = 20
	wide := cols
	wide.fitDate(200)
	if wide.date != 20 {
		t.Errorf("a wide list lost its Date column: %d", wide.date)
	}
	narrow := cols
	narrow.fitDate(71)
	if narrow.date != 0 {
		t.Errorf("a 71-cell list keeps a Date column of %d", narrow.date)
	}
	parts := flagged()
	parts.date = ui.Span{T: "due 2026-12-01"}
	if got := labelText(layoutRow(narrow, 200, parts)); !strings.HasSuffix(got, cellGap+"due 2026-12-01") {
		t.Errorf("row %q, want the badge after the title", got)
	}
	if got := labelText(layoutRow(narrow, 80, parts)); !strings.HasSuffix(got, cellGap+"due 2026-12-01") || !strings.Contains(got, "…") {
		t.Errorf("row %q, want the badge kept and the title cut", got)
	}
	// Between the full badge and the least title there is room only for the compact badge; below that the badge goes.
	if got := labelText(layoutRow(narrow, 68, parts)); !strings.HasSuffix(got, cellGap+"due Dec 1") {
		t.Errorf("row %q, want the compact badge, due Dec 1", got)
	}
	if got := labelText(layoutRow(narrow, 40, parts)); strings.Contains(got, "due ") {
		t.Errorf("row %q, want the badge dropped where less than %d cells of title fit beside it", got, minFlaggedTitle)
	}
}

func TestCompactDateWritesTheDayShortAndLeavesWordsAlone(t *testing.T) {
	for in, want := range map[string]string{"due 2026-12-01": "due Dec 1", "scheduled 2026-11-25": "scheduled Nov 25", "due today": "due today", "overdue 3d": "overdue 3d"} {
		if got := compactDate(in); got != want {
			t.Errorf("compactDate(%q) = %q, want %q", in, got, want)
		}
	}
}
