package driver

import (
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
)

// textFocusID is the node that should hold Tern's caret, "" for none.
func textFocusID(m model.Model) string {
	if m.Create.Open {
		if m.Create.Saving || m.Create.DiscardConfirm {
			return ""
		}
		return view.CreateFieldID(m.Create.Field)
	}
	if _, active := m.ActiveDraft(); active {
		if m.EditorMode() == model.ModeEditor {
			return view.DraftEditorID
		}
		return ""
	}
	if m.FieldForm.Open {
		if !m.FieldForm.Saving {
			return view.FieldInputID
		}
		return ""
	}
	if m.CloseForm.Open {
		if m.CloseForm.Pending {
			return ""
		}
		return view.CloseFieldID(m.CloseForm.Field)
	}
	if m.FilterForm.Open {
		switch m.FilterForm.Field {
		case 2, 3:
			return view.FilterFieldID(m.FilterForm.Field)
		}
		return ""
	}
	if m.ProjectForm.Open {
		if !m.ProjectForm.Saving {
			return view.ProjectInputID
		}
		return ""
	}
	if m.Picker.Open {
		return view.PickerFilterID
	}
	if !m.HelpOpen && m.Search.Open && !m.Search.Results && m.Err == nil && !m.FirstLoad() && m.Focus == model.ColumnList {
		return view.SearchID
	}
	return ""
}

// transientTarget reports whether id is a node that can disappear between a
// queued focus, reveal or scroll op and the next render, so that Tern's
// "unknown id" answer for it is expected.
func transientTarget(id string) bool {
	if _, ok := view.CreateFieldIndex(id); ok {
		return true
	}
	if id == view.FieldInputID || id == view.SearchID || id == view.DraftEditorID || id == view.PickerFilterID || id == view.ProjectInputID {
		return true
	}
	if _, ok := view.FilterFieldIndex(id); ok {
		return true
	}
	if _, ok := view.CloseFieldIndex(id); ok {
		return true
	}
	if view.IsIssueID(id) {
		return true
	}
	// The active tab's rows and its empty line go when the tab switches; the tab names and the activity container stay.
	row, ok := strings.CutPrefix(id, view.DetailActivityID+".")
	return ok && row != "tabs" && !strings.Contains(row, ".")
}
