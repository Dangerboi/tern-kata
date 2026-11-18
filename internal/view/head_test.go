package view

import (
	"testing"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func TestPageHeadIsAStrongTitleThenMutedFactsOnOneLine(t *testing.T) {
	row := pageHead("jobs", "Jobs", "3", "", "archived hidden").(ui.Row)
	if row.Key != "jobs" || len(row.Children) != 2 {
		t.Fatalf("head %+v", row)
	}
	title := row.Children[0].(ui.Text)
	facts := row.Children[1].(ui.Text)
	if title.Key != "title" || len(title.Spans) != 1 || title.Spans[0].T != "Jobs" || title.Spans[0].S != "strong" {
		t.Errorf("title %+v", title)
	}
	if facts.Key != "facts" || facts.Tone != ui.ToneMuted || facts.Text != "3 · archived hidden" {
		t.Errorf("facts %+v", facts)
	}
}

func TestPageHeadWithoutFactsIsTheTitleAlone(t *testing.T) {
	if row := pageHead("x", "Runs", "", "").(ui.Row); len(row.Children) != 1 {
		t.Fatalf("children %d, want only the title", len(row.Children))
	}
}

func TestCountSpanIsAMutedBareNumberAndNothingAtZero(t *testing.T) {
	if got := countSpan(4); got.T != "4" || got.S != "muted" {
		t.Errorf("countSpan(4) = %+v", got)
	}
	if got := countSpan(0); got.T != "" || got.S != "" {
		t.Errorf("countSpan(0) = %+v, want the zero span", got)
	}
}
