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

// filterChoiceLabel is how the Filter form shows value of the choice field called name.
func filterChoiceLabel(m model.Model, name model.Field, value string) string {
	switch name {
	case model.FilterProject:
		if value != "" {
			return m.ProjectFilterName(value)
		}
		return "Any Project in this Collection"
	case model.FilterStatus:
		return model.FilterChoiceLabel(model.FilterStatuses, value)
	}
	return model.FilterChoiceLabel(model.FilterRelationships, value)
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
			choices := m.FilterChoices(field.Name)
			labels := make([]string, len(choices))
			for i, choice := range choices {
				labels[i] = filterChoiceLabel(m, field.Name, choice)
			}
			children = append(children, choiceField(field.Name, tone, title, prompt, labels, func(i int) bool { return choices[i] == value }, -1))
			continue
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: value, Cursor: caretAt(m.FilterForm.Caret, field.Name, value), Mode: "insert", Prompt: []ui.Span{{T: prompt}}, Placeholder: "Any"},
		})
	}
	if m.FilterForm.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Saving default filter…"})
	}
	if m.FilterForm.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.FilterForm.Err.Error()})
	}
	children = append(children, buttonRow(cardButton("apply", model.ModeFilterForm, model.ApplyFilters, "Apply filters", true), cardButton("default", model.ModeFilterForm, model.SaveFilterDefault, "Save as default", false), cardButton("cancel", model.ModeFilterForm, model.CancelFilters, "Cancel", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardFilter)}, Head: ui.T("Filter " + m.Collection.Title), Modal: true, Size: ui.SizeMD, Children: children})
}
