package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func help(m model.Model) tern.Element {
	if !m.HelpOpen {
		return nil
	}
	rows := make([]tern.Element, 0, len(model.Keymap))
	for i, binding := range model.Keymap {
		mode := string(binding.Mode)
		if mode == "" {
			mode = "all modes"
		}
		rows = append(rows, ui.Text{
			Common: ui.Common{Key: "binding-" + strconv.Itoa(i)},
			Text:   binding.Key + "  " + binding.Help + " (" + mode + ")",
		})
	}
	return ui.Nodes(ui.Overlay{
		Common:   ui.Common{Key: "help"},
		Modal:    true,
		Head:     ui.T("Keyboard help"),
		Size:     ui.SizeLG,
		Children: rows,
	})
}
