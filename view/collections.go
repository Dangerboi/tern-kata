package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CollectionsID = "main.columns.collections"

func Collections(m model.Model) tern.Element {
	items := make([]tern.Element, 0, len(m.Collections)+1)
	for _, collection := range m.Collections {
		items = append(items, ui.Item{Common: ui.Common{Key: collection.Key()}, Label: ui.T(collection.Title)})
	}
	selected := ""
	if len(m.Collections) > 0 {
		// The New Project row follows the Collections; the cursor reaches it one past the last.
		items = append(items, ui.Item{Common: ui.Common{Key: model.NewProjectKey}, Label: ui.T(model.NewProjectTitle)})
		selected = CollectionsID + "." + model.NewProjectKey
		if m.CollectionCursor < len(m.Collections) {
			selected = CollectionsID + "." + m.Collections[m.CollectionCursor].Key()
		}
	}
	return ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "collections"}, Selected: selected, Empty: ui.T("Loading Collections…"), Children: items}
}
