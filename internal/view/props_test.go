package view

import (
	"testing"

	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func TestPropRowKeyHasNoColonAndSharesTheKeyBox(t *testing.T) {
	row := propRow("Scheduled", ui.Text{Common: ui.Common{Key: "v"}, Text: "2026-11-18"}).(ui.Row)
	if row.Key != "scheduled" || row.Gap != ui.GapMD {
		t.Errorf("row key %q gap %v", row.Key, row.Gap)
	}
	key := row.Children[0].(ui.Text)
	if key.Role != PropertyKeyRole || key.Spans[0].T != "Scheduled" || key.Spans[0].S != "muted" || key.Tone != ui.ToneMuted {
		t.Errorf("key = %+v", key)
	}
	if len(row.Children) != 2 {
		t.Errorf("%d children, want the key and the value", len(row.Children))
	}
}

func TestPropGridIsTheGridRole(t *testing.T) {
	grid := propGrid(propRow("Owner")).(ui.Col)
	if grid.Role != PropertiesRole || grid.Gap != ui.GapXS {
		t.Errorf("grid role %q gap %v", grid.Role, grid.Gap)
	}
	var _ tern.Element = grid
}
