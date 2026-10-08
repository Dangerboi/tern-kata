package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CollectionsID = "main.columns.collections"

func CollectionKey(c model.Collection) string { return c.Key() }

func Collections(m model.Model) tern.Element {
	items := make([]tern.Element, 0, len(m.Collections))
	for _, collection := range m.Collections {
		items = append(items, ui.Item{Common: ui.Common{Key: CollectionKey(collection)}, Label: ui.T(collection.Title)})
	}
	selected := ""
	if len(m.Collections) > 0 && m.CollectionCursor < len(m.Collections) {
		selected = CollectionsID + "." + CollectionKey(m.Collections[m.CollectionCursor])
	}
	return ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "collections"}, Selected: selected, Empty: ui.T("Loading Collections…"), Children: items}
}
