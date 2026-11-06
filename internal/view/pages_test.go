package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestWideTitleMovesThePageBarToItsOwnLineByCells(t *testing.T) {
	const length, maxCols = 40, 400
	titled := func(title string, cols int) model.Model {
		m := apply(browsing(), model.ResizeMsg{Cols: cols})
		m.Collection.Title = title
		return m
	}
	narrow, wide := strings.Repeat("a", length), strings.Repeat("字", length)

	cols := model.MinCols
	for ; cols <= maxCols; cols++ {
		if _, own := render(t, titled(narrow, cols))["main.pagebar"]; !own {
			break
		}
	}
	if cols > maxCols {
		t.Fatalf("the ASCII title still puts the Page bar on its own line at %d columns", maxCols)
	}
	if _, ok := render(t, titled(wide, cols))["main.pagebar"]; !ok {
		t.Errorf("a %d-rune CJK title wider than %d columns kept the Page bar beside the title", length, cols)
	}
}
