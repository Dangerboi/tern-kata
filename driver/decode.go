package driver

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// decode turns one Tern input into the message the model understands, nil for
// an input the Pane does not use.
func decode(input tern.Input) model.Msg {
	switch input := input.(type) {
	case tern.Key:
		text := ""
		if input.Name == "paste" || (!input.Ctrl && !input.Alt && !input.Meta) {
			text = input.Text
		}
		return model.KeyMsg{Chord: input.String(), Text: text}
	case *tern.Edit:
		return model.EditMsg{ID: input.ID, From: input.From, To: input.To, Text: input.Text, Cursor: input.Cursor, Len: input.Len}
	case *tern.Undo:
		return model.UndoMsg{ID: input.ID}
	case *tern.Focus:
		var msg model.Msg
		if field, ok := view.FilterFieldIndex(input.ID); ok {
			msg = model.FilterFieldMsg{Field: field}
		}
		if field, ok := view.CreateFieldIndex(input.ID); ok {
			msg = model.CreateFieldMsg{Field: field}
		}
		return msg
	case *tern.Select:
		switch input.ID {
		case view.IssueListID:
			return model.SelectMsg{UID: strings.TrimPrefix(input.Item, view.IssueListID+".")}
		case view.CollectionsID:
			return model.CollectionPointerSelected{Key: strings.TrimPrefix(input.Item, view.CollectionsID+".")}
		case view.PickerListID:
			return model.PickerSelected{Key: strings.TrimPrefix(input.Item, view.PickerListID+".")}
		}
	case *tern.Action:
		if input.ID == view.RetryID && input.Act == string(model.RetryAction) {
			return model.Retry{}
		}
		return decodeAction(input)
	case *tern.Resize:
		return model.ResizeMsg{Cols: input.Cols}
	}
	return nil
}

func decodeAction(input *tern.Action) model.Msg {
	switch input.Act {
	case string(model.SelectIssueAction):
		return model.ActionMsg{Action: model.SelectIssueAction, Value: input.Value}
	case "key":
		return model.ActionMsg{Action: "key", Value: input.Value}
	case string(model.ToggleExpansion), string(model.OpenRow):
		return model.ActionMsg{Action: model.Action(input.Act), Value: input.Value}
	case "focus-detail":
		return model.FocusColumnMsg{Column: model.ColumnDetail}
	case "activity-tab":
		tab, err := strconv.Atoi(input.Value)
		if err == nil && tab >= 0 && tab < int(model.TabCount) {
			return model.ActivityTabMsg{Tab: model.ActivityTab(tab)}
		}
	case "detail-item":
		tabText, key, ok := strings.Cut(input.Value, ":")
		tab, err := strconv.Atoi(tabText)
		if ok && err == nil && tab >= 0 && tab < int(model.TabCount) {
			return model.DetailItemMsg{Tab: model.ActivityTab(tab), Key: key}
		}
	case "jump":
		return model.JumpIssueMsg{UID: input.Value}
	}
	return nil
}
