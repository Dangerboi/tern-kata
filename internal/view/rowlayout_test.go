package view

import (
	"testing"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func datedParts(badges ...string) rowParts {
	parts := rowParts{
		id:     ui.Span{T: "dddd"},
		prio:   ui.Span{T: "p1"},
		status: ui.Span{T: "open"},
		title:  "A rather long Dated title",
		date:   ui.Span{T: "due today"},
	}
	for _, badge := range badges {
		parts.badges = append(parts.badges, ui.Span{T: badge})
	}
	return parts
}

// fitWidths is the smallest list width at which the row keeps its Date badge, and whether every wider width keeps it too.
func fitWidths(t *testing.T, cols listColumns, parts rowParts) int {
	t.Helper()
	first := -1
	for width := 20; width <= 200; width++ {
		fits := layoutRow(cols, width, parts).dateFits
		if fits && first < 0 {
			first = width
		}
		if !fits && first >= 0 {
			t.Fatalf("width %d drops the Date badge that width %d kept", width, first)
		}
	}
	if first < 0 {
		t.Fatal("no width keeps the Date badge")
	}
	return first
}

func TestLayoutRowKeepsTheDateBadgeOnlyWhereTheWholeRowFits(t *testing.T) {
	cols := listColumns{id: 4, owner: ownerWidth}
	parts := datedParts()
	first := fitWidths(t, cols, parts)
	kept, dropped := layoutRow(cols, first, parts), layoutRow(cols, first-1, parts)
	if len(kept.label) != len(dropped.label)+1 {
		t.Fatalf("kept row has %d label spans, dropped row %d, want one more for the badge", len(kept.label), len(dropped.label))
	}
	// The badge's span carries its gap, and the title stays whole either way.
	if got := kept.label[len(kept.label)-1].T; got != cellGap+"due today" {
		t.Errorf("badge span = %q", got)
	}
	if got := dropped.label[len(dropped.label)-1].T; got != cellGap+parts.title {
		t.Errorf("last span of the row without the badge = %q, want the whole title", got)
	}
	// With the owner column hidden the spans are the whole row: the first fitting width holds it to the cell.
	total := rowPadding + itemDetailWidth
	for _, span := range append(kept.label, kept.value...) {
		total += displayWidth(span.T)
	}
	if total != first {
		t.Errorf("row with its badge is %d cells wide, in the narrowest list that keeps it, of %d", total, first)
	}
}

func TestLayoutRowHasNoDateBadgeToKeepWhenTheIssueHasNone(t *testing.T) {
	parts := datedParts()
	parts.date = ui.Span{}
	if laid := layoutRow(listColumns{id: 4, owner: ownerWidth}, 400, parts); laid.dateFits || len(laid.label) != 4 {
		t.Errorf("dateFits %v with %d label spans, want no badge", laid.dateFits, len(laid.label))
	}
}

func TestLayoutRowNeedsRoomForTheOwnerColumnOnlyWhenItIsShown(t *testing.T) {
	parts := datedParts()
	hidden := fitWidths(t, listColumns{id: 4, owner: ownerWidth}, parts)
	shown := fitWidths(t, listColumns{id: 4, owner: ownerWidth, ownerShown: true}, parts)
	if want := hidden + displayWidth(cellGap) + ownerWidth; shown != want {
		t.Errorf("owner shown needs %d cells, hidden %d, want the owner box and its gap more", shown, hidden)
	}
}

func TestLayoutRowCountsWhatStandsAheadOfTheTitle(t *testing.T) {
	cols := listColumns{id: 4, owner: ownerWidth}
	plain := fitWidths(t, cols, datedParts())
	badged := fitWidths(t, cols, datedParts("agent"))
	both := fitWidths(t, cols, datedParts("agent", "review"))
	// The badge takes its gap of cellGap and the title then one cell, a cell less than the title's own gap.
	if want := plain + displayWidth(cellGap+"agent") - 1; badged != want {
		t.Errorf("Agent badge needs %d cells, plain row %d, want %d", badged, plain, want)
	}
	if want := badged + displayWidth(" review"); both != want {
		t.Errorf("Agent badge and Work attention word need %d cells, Agent badge alone %d, want %d", both, badged, want)
	}
}

func TestLayoutRowMeasuresTheIDColumnByDisplayWidth(t *testing.T) {
	narrow := fitWidths(t, listColumns{id: 4}, datedParts())
	wide := datedParts()
	wide.id = ui.Span{T: "田中"} // two cells each
	got := fitWidths(t, listColumns{id: displayWidth("田中")}, wide)
	if want := narrow; got != want {
		t.Errorf("a column of %d cells needs %d cells, one of 4 needs %d", displayWidth("田中"), got, narrow)
	}
}
