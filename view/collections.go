package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CollectionsID = "main.columns.collections"

func Collections(m model.Model) tern.Element {
	entries := m.SidebarEntries()
	items := make([]tern.Element, 0, len(entries))
	for _, entry := range entries {
		items = append(items, ui.Item{Common: ui.Common{Key: entry.Key()}, Label: ui.T(entry.Title)})
	}
	selected := ""
	if len(entries) > 0 {
		selected = CollectionItemID(entries[min(m.CollectionCursor, len(entries)-1)].Key())
	}
	return ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "collections"}, Selected: selected, Empty: ui.T("Loading Menu…"), Children: items}
}
