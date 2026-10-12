package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const SearchID = "main.columns.list.search"

// FilterFieldID is the id of the Filter form's field called name.
func FilterFieldID(name model.Field) string { return "layer.filters." + string(name) }

var filterTitles = map[model.Field]string{
	model.FilterProject:      "Project",
	model.FilterStatus:       "Status",
	model.FilterOwner:        "Owner",
	model.FilterLabel:        "Label (comma-separated, any match)",
	model.FilterRelationship: "Relationship",
}

// filterText is what the Filter form shows in the field called name.
func filterText(m model.Model, name model.Field) string {
	values := m.FilterForm.Values
	switch name {
	case model.FilterProject:
		if values.ProjectUID != "" {
			return m.ProjectFilterName(values.ProjectUID)
		}
		return "Any Project in this Collection"
	case model.FilterStatus:
		return model.FilterChoiceLabel(model.FilterStatuses, values.Status)
	case model.FilterOwner:
		return values.Owner
	case model.FilterLabel:
		return values.Label
	case model.FilterRelationship:
		return model.FilterChoiceLabel(model.FilterRelationships, values.Relationship)
	}
	return ""
}

func filterLayer(m model.Model) tern.Element {
	if !m.FilterForm.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.FilterFields)+1)
	for _, field := range model.FilterFields {
		tone := ui.ToneNeutral
		title := filterTitles[field.Name]
		prompt := title + ": "
		if field.Name == m.FilterForm.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: filterText(m, field.Name), Mode: "insert", Readonly: !field.Text, Prompt: []ui.Span{{T: prompt}}, Placeholder: "Any"},
		})
	}
	children = append(children, buttonRow(keyButton("apply", "Apply filters", "ctrl+o", true), keyButton("cancel", "Cancel", "escape", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "filters"}, Head: ui.T("Filter " + m.Collection.Title), Modal: true, Size: ui.SizeMD, Children: children})
}
