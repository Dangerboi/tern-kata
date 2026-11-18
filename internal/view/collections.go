package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const CollectionsID = "main.columns.collections"

// Collections is the Menu drawn as a sidebar. The top level groups its rows under labelled dividers, as the popup does; the
// Views level labels the saved views and sets New view apart. A Lettered Page shows the key that opens it. The Back row stays: the
// sidebar has no Back button.
func Collections(m model.Model) tern.Element {
	entries := m.SidebarEntries()
	items := make([]tern.Element, 0, len(entries)+3)
	section := ""
	for _, entry := range entries {
		switch {
		case m.SidebarLevel == model.MenuTop && entry.Section != section:
			section = entry.Section
			items = append(items, ui.Rule{Common: ui.Common{Key: "rule-" + section}, Label: ui.T(section)})
		case m.SidebarLevel == model.MenuViews && entry.Kind == model.EntryNewView:
			items = append(items, ui.Rule{Common: ui.Common{Key: "rule-new-view"}})
		}
		item := ui.Item{Common: ui.Common{Key: entry.Key()}, Label: ui.T(m.EntryTitle(entry))}
		if entry.Hint != "" {
			item.Hint = []string{entry.Hint}
		}
		items = append(items, item)
		if m.SidebarLevel == model.MenuViews && entry.Kind == model.EntryBack {
			items = append(items, ui.Rule{Common: ui.Common{Key: "rule-views"}, Label: ui.T(model.ViewsTitle)})
		}
	}
	selected := ""
	if len(entries) > 0 {
		selected = itemID(CollectionsID, entries[min(m.CollectionCursor, len(entries)-1)].Key())
	}
	return ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "collections"}, Selected: selected, Empty: ui.T("Loading Menu…"), Children: items}
}
