package view_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func TestBelowMinimumWidthShowsOnlyTheNotice(t *testing.T) {
	for _, tt := range []struct {
		name string
		msgs []model.Msg
	}{
		{"list", nil},
		{"detail focused", []model.Msg{model.KeyMsg{Chord: "enter"}}},
		{"help open", []model.Msg{model.KeyMsg{Chord: "?"}}},
		{"picker open", []model.Msg{model.KeyMsg{Chord: "p"}}},
		{"daemon failure", []model.Msg{model.Failed{Err: errors.New("daemon down")}}},
		{"tiny", []model.Msg{model.ResizeMsg{Cols: 2}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nodes := render(t, apply(browsing(), append([]model.Msg{model.ResizeMsg{Cols: model.MinCols - 1}}, tt.msgs...)...))
			var texts []string
			for id, node := range nodes {
				if id != "main" && !strings.HasPrefix(id, "main.notice") {
					t.Errorf("%s shown below 80 columns", id)
				}
				if s := text(node); s != "" {
					texts = append(texts, s)
				}
			}
			if len(texts) != 1 || !strings.Contains(texts[0], "80 columns") {
				t.Fatalf("texts = %q, want one notice asking for 80 columns", texts)
			}
		})
	}
}

func TestEightyColumnsLaysOutAgain(t *testing.T) {
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: model.MinCols}))
	if len(nodes.under("main.notice")) != 0 || len(nodes.under("main.columns.list")) == 0 {
		t.Fatalf("80 columns: %v", nodes.under("main"))
	}
}

func TestWideningRestoresTheScreen(t *testing.T) {
	before := browsing()
	want := render(t, before)
	got := render(t, apply(before, model.ResizeMsg{Cols: 40}, model.ResizeMsg{Cols: 160}))
	sameIDs(t, "main", want, got)
	sameIDs(t, "dock", want, got)
}
