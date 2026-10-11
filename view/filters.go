package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const SearchID = "main.columns.list.search"

var filterKeys = [...]string{"project", "status", "owner", "label", "relationship"}

// FilterFieldID is the id of the Filter form's field index.
func FilterFieldID(index int) string { return "layer.filters." + filterKeys[index] }

func FilterFieldIndex(id string) (int, bool) {
	for index := range filterKeys {
		if id == FilterFieldID(index) {
			return index, true
		}
	}
	return 0, false
}

func filterLayer(m model.Model) tern.Element {
	if !m.FilterForm.Open {
		return nil
	}
	values := m.FilterForm.Values
	project := "Any Project in this Collection"
	if values.ProjectUID != "" {
		project = m.ProjectFilterName(values.ProjectUID)
	}
	status := "all"
	if values.Status != "" {
		status = values.Status
	}
	relationship := "Any"
	switch values.Relationship {
	case "parent":
		relationship = "Has parent"
	case "child":
		relationship = "Has child"
	case "blocks":
		relationship = "Blocks"
	case "blocked_by":
		relationship = "Blocked by"
	case "related":
		relationship = "Related"
	}
	fields := []struct {
		key, title, text string
		readonly         bool
	}{
		{"project", "Project", project, true},
		{"status", "Status", status, true},
		{"owner", "Owner", values.Owner, false},
		{"label", "Label (comma-separated, any match)", values.Label, false},
		{"relationship", "Relationship", relationship, true},
	}
	children := make([]tern.Element, 0, len(fields))
	for index, field := range fields {
		tone := ui.ToneNeutral
		prompt := field.title + ": "
		if index == m.FilterForm.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: field.key, Tone: tone, Aria: field.title},
			Field:  ui.Field{Text: field.text, Mode: "insert", Readonly: field.readonly, Prompt: []ui.Span{{T: prompt}}, Placeholder: "Any"},
		})
	}
	children = append(children, buttonRow(keyButton("apply", "Apply filters", "ctrl+o", true), keyButton("cancel", "Cancel", "escape", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "filters"}, Head: ui.T("Filter " + m.Collection.Title), Modal: true, Size: ui.SizeMD, Children: children})
}
