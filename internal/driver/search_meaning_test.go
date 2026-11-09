package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// searchLineDrawn is the text of the search line under the search box, empty when none is drawn.
func searchLineDrawn(t *testing.T, s driver.State) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasSuffix(n.ID, ".search-line") {
			return nodeTextOf(n)
		}
	}
	return ""
}

func TestCtrlTOutsideAProjectDrawsTheRefusalOnTheSearchLine(t *testing.T) {
	s := pressed(clickBase(), "/")
	if got := searchLineDrawn(t, s); got != "" {
		t.Fatalf("search line before ctrl+t: %q", got)
	}
	s = pressed(s, "ctrl+t")
	if got := searchLineDrawn(t, s); got != "search by meaning works inside one Project" {
		t.Fatalf("search line %q", got)
	}
	if s.Model.Search.Meaning {
		t.Fatal("search by meaning turned on outside a Project")
	}
	s, _ = inputs(tern.Key{Text: "x"})(s)
	if got := searchLineDrawn(t, s); got != "" {
		t.Fatalf("search line after typing: %q", got)
	}
}
