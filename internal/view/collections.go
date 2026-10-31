package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CollectionsID = "main.columns.collections"

func Collections(m model.Model) tern.Element {
	entries := m.SidebarEntries()
	items := make([]tern.Element, 0, len(entries))
	for _, entry := range entries {
		items = append(items, ui.Item{Common: ui.Common{Key: entry.Key()}, Label: ui.T(m.EntryTitle(entry))})
	}
	selected := ""
	if len(entries) > 0 {
		selected = CollectionItemID(entries[min(m.CollectionCursor, len(entries)-1)].Key())
	}
	list := ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "collections"}, Selected: selected, Children: items}
	return listOrEmpty(list, false, emptyLane("collections", "No Menu yet", "kata has not listed its Collections."))
}
