package view

import (
	"slices"
	"strings"
	"sync"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// attachmentKeys is the menu of an Attachment row: each item is the detail's key for the table's action, so a pick presses what the
// footer and the help sheet name. The bindings do not change while the Pane runs, so the keys are read once.
var attachmentKeys = sync.OnceValue(func() contextMenuKeys {
	rows := make([]contextMenuRow, 0, len(model.AttachmentActions()))
	for _, action := range model.AttachmentActions() {
		rows = append(rows, contextMenuRow{action: action.Action, label: action.Label})
	}
	return newMenuKeys(model.ModeDetail, rows)
})

// attachmentMenu is the right-click menu and the long-press list of the Attachment row with rowKey, on the detail of Issue uid.
func attachmentMenu(uid, rowKey string, a model.Attachment) []ui.MenuEntry {
	table := model.AttachmentActions()
	refusal := func(action model.Action) string {
		i := slices.IndexFunc(table, func(row model.AttachmentAction) bool { return row.Action == action })
		return table[i].Refusal(a)
	}
	return attachmentKeys().entries(uid, rowKey, refusal, nil)
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
				Buttons: []ui.FieldButton{{Act: "copy", Icon: ui.IconCopy, Title: "Copy " + strings.ToLower(label)}}}},
		}
	}
	var children []tern.Element
	children = append(children, copyField("path", "Downloaded file", c.Path)...)
	children = append(children, copyField("command", "Command", c.Command)...)
	children = append(children, buttonRow(exitButton("close", model.ModeAttachmentCopy, model.AttachmentCopyClose, "Close")))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAttachmentCopy)}, Modal: true, Size: ui.SizeLG, Head: ui.T("Copy attachment"), Meta: ui.Spans(monoSpan(c.Name, "")), Icon: ui.IconCopy}, children))
}
