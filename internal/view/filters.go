package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const SearchID = "main.columns.list.search"

var filterTitles = map[model.Field]string{
	model.FilterProject:      "Project",
	model.FilterStatus:       "Status",
	model.FilterOwner:        "Owner",
	model.FilterLabel:        "Label (comma-separated, any match)",
	model.FilterRelationship: "Relationship",
}

func filterLayer(m model.Model) tern.Element {
	if !m.FilterForm.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.FilterFields)+1)
	for _, field := range model.FilterFields {
		if !m.FilterFieldShown(field.Name) {
			continue
		}
		tone := ui.ToneNeutral
		title := filterTitles[field.Name]
		prompt := title + ": "
		if field.Name == m.FilterForm.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		value := m.FilterForm.Values.Value(field.Name)
		if !field.Text {
			children = append(children, choiceControl(m, model.CardFilter, field.Name, title, field.Name == m.FilterForm.Field, "", ""))
			continue
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: value, Cursor: caretAt(m.FilterForm.Caret, field.Name, value), Mode: "insert", Prompt: []ui.Span{{T: prompt}}, Placeholder: ui.T("Any")},
		})
	}
	if m.FilterForm.Saving {
		children = append(children, toneText("pending", ui.TonePending, "Saving default filter…"))
	}
	if m.FilterForm.Err != nil {
		children = append(children, toneText("error", ui.ToneError, m.FilterForm.Err.Error()))
	}
	children = append(children, buttonRow(cardButton("apply", model.ModeFilterForm, model.ApplyFilters, "Apply filters", true), cardButton("default", model.ModeFilterForm, model.SaveFilterDefault, "Save as default", false), cardButton("cancel", model.ModeFilterForm, model.CancelFilters, "Cancel", false)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardFilter)}, Head: ui.T("Filter"), Meta: ui.T(m.Collection.Title), Icon: ui.IconFunnel, Modal: true, Size: ui.SizeLG}, children))
}
