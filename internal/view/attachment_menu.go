package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// attachmentMenuRows are the items of an Attachment row's context menu, in order. Each is the detail's key for its action, so a pick
// presses what the footer and the help sheet name.
var attachmentMenuRows = []struct {
	action model.Action
	label  string
	// way is what the row does with the file, which says whether it is refused; Unlink has none, and WayOpen is never refused.
	way model.AttachmentWay
}{
	{model.DrillIn, "Open", model.WayOpen},
	{model.OpenAttachmentBesideAction, "Open beside", model.WayBeside},
	{model.OpenAttachmentSystemAction, "Open with the system app", model.WaySystem},
	{model.SaveAttachmentAction, "Save to Downloads", model.WaySave},
	{model.CopyAttachmentAction, "Copy…", model.WayCopy},
	{model.UnlinkAttachmentAction, "Unlink…", model.WayOpen},
}

// attachmentMenu is the right-click menu and the long-press list of the Attachment row with rowKey.
func attachmentMenu(rowKey string, a model.Attachment) []ui.MenuEntry {
	menu := make([]ui.MenuEntry, 0, len(attachmentMenuRows))
	for _, row := range attachmentMenuRows {
		keys, chord := cardKeys(model.ModeDetail, row.action)
		if chord == "" {
			continue
		}
		item := ui.MenuItem{ID: AttachmentMenuPickClick(rowKey, chord), Label: row.label, Keys: keys}
		if reason := model.AttachmentRefusal(a, row.way); reason != "" {
			item.Disabled = ui.Because(reason)
		}
		menu = append(menu, item)
	}
	return menu
}

// attachmentCopyLayer is the Copy card: the downloaded file's path and the kata command that fetches it, each read-only with a
// button that copies it on the machine that shows the window.
func attachmentCopyLayer(m model.Model) tern.Element {
	c := m.AttachmentCopy
	if !c.Open {
		return nil
	}
	copyField := func(key, label, text string) []tern.Element {
		return []tern.Element{
			toneText(key+"-label", ui.ToneNeutral, label),
			ui.Input{Common: ui.Common{Key: key, Aria: label}, Field: ui.Field{Text: text, Readonly: true,
				Buttons: []ui.FieldButton{{Act: "copy", Icon: ui.IconCopy, Title: "Copy " + label}}}},
		}
	}
	children := []tern.Element{toneText("name", ui.ToneMuted, c.Name)}
	children = append(children, copyField("path", "Downloaded file", c.Path)...)
	children = append(children, copyField("command", "kata command", c.Command)...)
	children = append(children, buttonRow(cardButton("close", model.ModeAttachmentCopy, model.AttachmentCopyClose, "Close", false)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAttachmentCopy)}, Modal: true, Size: ui.SizeLG, Head: ui.T("Copy"), Icon: ui.IconCopy}, children))
}
