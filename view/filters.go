package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"strings"
)

const SearchID = "main.columns.list.search"

func FilterFieldIndex(id string) (int, bool) {
	for index, key := range [...]string{"project", "status", "owner", "label", "relationship"} {
		if id == "layer.filters."+key {
			return index, true
		}
	}
	return 0, false
}

func InputFocusID(m model.Model) string {
	if m.Create.Open {
		if m.Create.Saving || m.Create.DiscardConfirm {
			return ""
		}
		return "layer.create." + createKeys[m.Create.Field]
	}
	if _, active := m.ActiveDraft(); active {
		if m.EditorMode() == model.ModeEditor {
			return DraftEditorID
		}
		return ""
	}
	if m.FieldForm.Open {
		if !m.FieldForm.Saving {
			return FieldInputID
		}
		return ""
	}
	if m.CloseForm.Open {
		return CloseInputFocusID(m)
	}
	if m.FilterForm.Open {
		switch m.FilterForm.Field {
		case 2:
			return "layer.filters.owner"
		case 3:
			return "layer.filters.label"
		}
		return ""
	}
	if m.ProjectForm.Open {
		if !m.ProjectForm.Saving {
			return ProjectInputID
		}
		return ""
	}
	if m.Picker.Open {
		return PickerFilterID
	}
	if !m.HelpOpen && m.Search.Open && !m.Search.Results && m.Err == nil && !m.FirstLoad() && m.Focus == model.ColumnList {
		return SearchID
	}
	return ""
}

func IsTransientTarget(id string) bool {
	if _, ok := CreateFieldIndex(id); ok {
		return true
	}
	if id == FieldInputID || id == SearchID || id == DraftEditorID || id == PickerFilterID || id == ProjectInputID {
		return true
	}
	if _, ok := FilterFieldIndex(id); ok {
		return true
	}
	if field, ok := strings.CutPrefix(id, "layer.close.field"); ok {
		return len(field) == 1 && field[0] >= '0' && field[0] <= '6'
	}
	return strings.HasPrefix(id, IssueListID+".")
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
	children = append(children, ui.Row{Common: ui.Common{Key: "actions"}, Gap: ui.GapMD, Children: ui.Nodes(
		el.Button{Common: ui.Common{Key: "apply", Actions: &ui.Actions{Click: string(model.ApplyFilters)}}, Text: "Apply filters"},
		el.Button{Common: ui.Common{Key: "cancel", Actions: &ui.Actions{Click: string(model.CancelFilters)}}, Text: "Cancel"},
	)})
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "filters"}, Head: ui.T("Filter " + m.Collection.Title), Modal: true, Size: ui.SizeMD, Children: children})
}
