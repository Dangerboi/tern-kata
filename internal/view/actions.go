package view

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The click-action vocabulary: what a clickable node tells Tern to send back, and how the Pane driver
// reads it. A click is `name` or `name=value`; Tern splits it at the first `=` into an Action input's
// act and value. Every action here decodes to the message its matching key produces, and a chord
// click is that key: it goes through the key path.
//
// The Luau window hook keeps its own copy of the Issue list id and the select-issue action.
const (
	keyAct         = "key"
	jumpAct        = "jump"
	activityTabAct = "activity-tab"
	detailItemAct  = "detail-item"
)

// FocusDetailClick and RetryClick carry no value.
const (
	FocusDetailClick = string(model.FocusDetail)
	RetryClick       = string(model.RetryAction)
)

func click(act, value string) string { return act + "=" + value }

// KeyClick presses chord.
func KeyClick(chord string) string { return click(keyAct, chord) }

// JumpClick opens the referenced Issue.
func JumpClick(uid string) string { return click(jumpAct, uid) }

// ToggleClick folds or unfolds the Issue row, and OpenRowClick is its double click.
func ToggleClick(uid string) string  { return click(string(model.ToggleExpansion), uid) }
func OpenRowClick(uid string) string { return click(string(model.OpenRow), uid) }

// ActivityTabClick shows the Activity tab.
func ActivityTabClick(tab model.ActivityTab) string {
	return click(activityTabAct, strconv.Itoa(int(tab)))
}

// DetailItemClick puts the detail's row cursor on the row with key in the tab.
func DetailItemClick(tab model.ActivityTab, key string) string {
	return click(detailItemAct, strconv.Itoa(int(tab))+":"+key)
}

func parseTab(text string) (model.ActivityTab, bool) {
	tab, err := strconv.Atoi(text)
	return model.ActivityTab(tab), err == nil && tab >= 0 && tab < int(model.TabCount)
}

// DecodeAction is the message for an Action input from node id, nil for one the Pane does not use.
func DecodeAction(id, act, value string) model.Msg {
	switch act {
	case keyAct:
		return model.KeyMsg{Chord: value}
	case string(model.SelectIssueAction), string(model.ToggleExpansion), string(model.OpenRow):
		return model.ActionMsg{Action: model.Action(act), Value: value}
	case FocusDetailClick:
		return model.FocusColumnMsg{Column: model.ColumnDetail}
	case RetryClick:
		if id == RetryID {
			return model.Retry{}
		}
	case activityTabAct:
		if tab, ok := parseTab(value); ok {
			return model.ActivityTabMsg{Tab: tab}
		}
	case detailItemAct:
		tabText, key, found := strings.Cut(value, ":")
		if tab, ok := parseTab(tabText); found && ok {
			return model.DetailItemMsg{Tab: tab, Key: key}
		}
	case jumpAct:
		return model.JumpIssueMsg{UID: value}
	}
	return nil
}

// The items of a list are its children, named list id + "." + key.
func itemID(list, key string) string   { return list + "." + key }
func itemKey(list, item string) string { return strings.TrimPrefix(item, list+".") }

// IssueID is the id of the Issue list's row for uid.
func IssueID(uid string) string { return itemID(IssueListID, uid) }

// IsIssueID reports whether id is a row of the Issue list.
func IsIssueID(id string) bool { return strings.HasPrefix(id, itemID(IssueListID, "")) }

// CollectionItemID and PickerItemID are the ids of the Menu sidebar's and the Menu popup's entry key.
func CollectionItemID(key string) string { return itemID(CollectionsID, key) }
func PickerItemID(key string) string     { return itemID(PickerListID, key) }

// DecodeSelect is the message for a Select input on list id choosing item, nil for a list the Pane does not use.
func DecodeSelect(id, item string) model.Msg {
	switch id {
	case IssueListID:
		return model.SelectMsg{UID: itemKey(IssueListID, item)}
	case CollectionsID:
		return model.CollectionPointerSelected{Key: itemKey(CollectionsID, item)}
	case PickerListID:
		return model.PickerSelected{Key: itemKey(PickerListID, item)}
	}
	return nil
}

// DecodeFocus is the message for Tern's focus on node id: focus that field of the open card, nil for any other node.
func DecodeFocus(id string) model.Msg {
	if _, field, ok := FieldFromID(id); ok && field != "" {
		return model.FocusFieldMsg{Field: field}
	}
	return nil
}

// FieldID is the node that holds card's field called name; a card without fields has one input, named "".
func FieldID(card model.Card, name model.Field) string {
	switch card {
	case model.CardCreate:
		return CreateFieldID(name)
	case model.CardDraft:
		return DraftEditorID
	case model.CardField:
		return FieldInputID
	case model.CardClose:
		return CloseFieldID(name)
	case model.CardProject:
		return ProjectInputID
	case model.CardFilter:
		return FilterFieldID(name)
	case model.CardPicker:
		return PickerFilterID
	}
	return ""
}

// FieldFromID is the card and field whose node has id; the field is "" for the one input of a card without fields.
func FieldFromID(id string) (model.Card, model.Field, bool) {
	for _, card := range model.Cards() {
		fields, _ := model.CardFields(card)
		for _, field := range fields {
			if id == FieldID(card, field.Name) {
				return card, field.Name, true
			}
		}
		if len(fields) == 0 && id == FieldID(card, "") {
			return card, "", true
		}
	}
	return model.NoCard, "", false
}
