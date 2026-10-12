package driver

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// textFocusID is the node that should hold Tern's caret, "" for none.
func textFocusID(m model.Model) string {
	switch m.TopCard() {
	case model.CardCreate:
		if m.Create.Saving || m.Create.DiscardConfirm {
			return ""
		}
		return view.CreateFieldID(m.Create.Field)
	case model.CardDraft:
		if draft, _ := m.ActiveDraft(); m.EditorMode() == model.ModeEditor && !draft.Saving {
			return view.DraftEditorID
		}
	case model.CardField:
		if !m.FieldForm.Saving {
			return view.FieldInputID
		}
	case model.CardClose:
		if !m.CloseForm.Saving {
			return view.CloseFieldID(m.CloseForm.Field)
		}
	case model.CardProject:
		if !m.ProjectForm.Saving {
			return view.ProjectInputID
		}
	case model.CardFilter:
		if m.FilterTextFieldFocused() {
			return view.FilterFieldID(m.FilterForm.Field)
		}
	case model.CardPicker:
		return view.PickerFilterID
	default:
		if m.Search.Open && !m.Search.Results && m.Err == nil && !m.FirstLoad() && m.Focus == model.ColumnList {
			return view.SearchID
		}
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
