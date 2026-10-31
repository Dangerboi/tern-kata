package view

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The click-action vocabulary: what a clickable node tells Tern to send back, and how the Pane driver
// reads it. A click is `name` or `name=value`; Tern splits it at the first `=` into an Action input's
// act and value. Every action here decodes to the message its matching key produces, and a chord
// click is that key: it goes through the key path.
//
// The Luau window hook keeps its own copy of the Issue list id and the select-issue and agent-states actions;
// agent-states carries the JSON agent list as its value and is not a click.
const (
	keyAct           = "key"
	jumpAct          = "jump"
	activityTabAct   = "activity-tab"
	detailItemAct    = "detail-item"
	chooseAct        = "choose"
	cardAct          = "card"
	refAct           = "ref"
	jobAct           = "job"
	runAct           = "run"
	attentionAct     = "attention"
	openJobAct       = "open-job"
	jobOpenAct       = "job-open"
	runOpenAct       = "run-open"
	attentionOpenAct = "attention-open"
	workflowOpenAct  = "workflow-open"
	agentAct         = "agent"
	menuPickAct      = "pick"
)

// FocusDetailClick, RetryClick and MenuClick carry no value.
const (
	FocusDetailClick = string(model.FocusDetail)
	RetryClick       = string(model.RetryAction)
	MenuClick        = string(model.OpenPicker)
	// NoticeLinkClick opens the Issue the Notice names.
	NoticeLinkClick = "notice-link"
)

func click(act, value string) string { return act + "=" + value }

// KeyClick presses chord.
func KeyClick(chord string) string { return click(keyAct, chord) }

// JumpClick opens the referenced Issue.
func JumpClick(uid string) string { return click(jumpAct, uid) }

// ToggleClick folds or unfolds the Issue row, and OpenRowClick is its double click.
func ToggleClick(uid string) string  { return click(string(model.ToggleExpansion), uid) }
func OpenRowClick(uid string) string { return click(string(model.OpenRow), uid) }

// AgentClick focuses the agent block, given as decimal text.
func AgentClick(block string) string { return click(agentAct, block) }

// CardClick selects the Board card of the Issue.
func CardClick(uid string) string { return click(cardAct, uid) }

// MenuPickClick is the id of a context menu row of the Issue with uid: select it, then press chord.
func MenuPickClick(uid, chord string) string { return click(menuPickAct, uid+":"+chord) }

// JobClick selects the job's card on the Jobs Page.
func JobClick(uid string) string { return click(jobAct, uid) }

// RunClick selects the Run's card on the Runs Page.
func RunClick(uid string) string { return click(runAct, uid) }

// AttentionClick selects the request on the Issue with uid on the Attention Page.
func AttentionClick(uid string) string { return click(attentionAct, uid) }

// OpenJobClick opens the Cron job on the Jobs Page.
func OpenJobClick(uid string) string { return click(openJobAct, uid) }

// JobOpenClick is the double click of a Jobs Page card: select it and press Enter. RunOpenClick, AttentionOpenClick and
// WorkflowOpenClick do the same on the Runs, Attention and Workflows Pages.
func JobOpenClick(uid string) string       { return click(jobOpenAct, uid) }
func RunOpenClick(uid string) string       { return click(runOpenAct, uid) }
func AttentionOpenClick(uid string) string { return click(attentionOpenAct, uid) }
func WorkflowOpenClick(uid string) string  { return click(workflowOpenAct, uid) }

// RefClick selects the Board card of a related Issue, revealing it in its Lane, or opens its detail when the Board does not draw it.
func RefClick(uid string) string { return click(refAct, uid) }

// ActivityTabClick shows the Activity tab.
func ActivityTabClick(tab model.ActivityTab) string {
	return click(activityTabAct, strconv.Itoa(int(tab)))
}

// DetailItemClick puts the detail's row cursor on the row with key in the tab.
func DetailItemClick(tab model.ActivityTab, key string) string {
	return click(detailItemAct, strconv.Itoa(int(tab))+":"+key)
}

// ChooseClick picks the choice at index of the open card's choice field called name.
func ChooseClick(name model.Field, index int) string {
	return click(chooseAct, string(name)+":"+strconv.Itoa(index))
}

func parseTab(text string) (model.ActivityTab, bool) {
	tab, err := strconv.Atoi(text)
	if err != nil || tab < 0 || tab >= int(model.TabCount) {
		return 0, false
	}
	return model.ActivityTab(tab), true
}

// DecodeAction is the message for an Action input from node id, nil for one the Pane does not use.
func DecodeAction(id, act, value string) model.Msg {
	switch act {
	case keyAct:
		return model.KeyMsg{Chord: value}
	case string(model.AgentStatesAction):
		return decodeAgentStates(value)
	case string(model.SelectIssueAction), string(model.ToggleExpansion), string(model.OpenRow), MenuClick:
		return model.ActionMsg{Action: model.Action(act), Value: value}
	case FocusDetailClick:
		return model.FocusColumnMsg{Column: model.ColumnDetail}
	case RetryClick:
		if id == RetryID {
			return model.Retry{}
		}
	case NoticeLinkClick:
		return model.OpenNoticeLinkMsg{}
	case activityTabAct:
		if tab, ok := parseTab(value); ok {
			return model.ActivityTabMsg{Tab: tab}
		}
	case detailItemAct:
		tabText, key, found := strings.Cut(value, ":")
		if tab, ok := parseTab(tabText); found && ok {
			return model.DetailItemMsg{Tab: tab, Key: key}
		}
	case chooseAct:
		name, index, found := strings.Cut(value, ":")
		if i, err := strconv.Atoi(index); found && err == nil {
			return model.ChooseMsg{Field: model.Field(name), Index: i}
		}
	case jumpAct:
		return model.JumpIssueMsg{UID: value}
	case agentAct:
		return model.AgentJumpMsg{Block: value}
	case cardAct:
		return model.SelectMsg{UID: value}
	case menuPickAct:
		if uid, chord, found := strings.Cut(value, ":"); found {
			return model.MenuPickMsg{UID: uid, Chord: chord}
		}
	case refAct:
		return model.ShowCardMsg{UID: value}
	case runAct:
		return model.RunSelectMsg{UID: value}
	case attentionAct:
		return model.AttentionSelectMsg{UID: value}
	case openJobAct:
		return model.OpenJobMsg{UID: value}
	case jobAct:
		return model.JobSelectMsg{UID: value}
	case jobOpenAct:
		return model.JobOpenMsg{UID: value}
	case runOpenAct:
		return model.RunOpenMsg{UID: value}
	case attentionOpenAct:
		return model.AttentionOpenMsg{UID: value}
	case workflowOpenAct:
		return model.WorkflowOpenMsg{UID: value}
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

// IsBoardCardID reports whether id is under the Board's node: a Lane column, its card list or a card.
func IsBoardCardID(id string) bool { return strings.HasPrefix(id, BoardID+".") }

// CollectionItemID and PickerItemID are the ids of the Menu sidebar's and the Menu popup's entry key.
func CollectionItemID(key string) string { return itemID(CollectionsID, key) }
func PickerItemID(key string) string     { return itemID(PickerListID, key) }

// DecodeSelect is the message for a Select input on list id choosing item, nil for a list the Pane does not use.
func DecodeSelect(id, item string) model.Msg {
	switch id {
	case IssueListID:
		return model.SelectMsg{UID: itemKey(IssueListID, item)}
	case WorkflowListID:
		return model.WorkflowSelectMsg{UID: itemKey(WorkflowListID, item)}
	case CollectionsID:
		return model.CollectionPointerSelected{Key: itemKey(CollectionsID, item)}
	case PickerListID:
		return model.PickerSelected{Key: itemKey(PickerListID, item)}
	case AgentListID:
		return model.AgentCardSelected{Block: itemKey(AgentListID, item)}
	case MoveListID:
		return model.MoveTargetSelected{Key: itemKey(MoveListID, item)}
	case ChildrenTableID:
		return model.JumpIssueMsg{UID: item}
	}
	if field, ok := controlField(id); ok {
		return decodeChoice(field, item)
	}
	return nil
}

// controlField is the choice field whose control node has id: a card field's id and the control's key.
func controlField(id string) (model.Field, bool) {
	fieldID, ok := strings.CutSuffix(id, "."+choiceControlKey)
	if !ok {
		return "", false
	}
	if _, field, found := FieldFromID(fieldID); found && field != "" {
		return field, true
	}
	return "", false
}

// decodeChoice is the choice a tab or an option sent by its id: the index it names in the order the card shows its choices,
// which is the last part of an id given as the whole node id.
func decodeChoice(field model.Field, item string) model.Msg {
	if index, err := strconv.Atoi(item[strings.LastIndex(item, ".")+1:]); err == nil {
		return model.ChooseMsg{Field: field, Index: index}
	}
	return nil
}

// DecodeChange is the message for Tern's new value of node id: a day picked on a card's calendar, the option a select picked
// (as the index it was given) or the state a switch flipped to. It is nil for any other node.
func DecodeChange(id string, value any) model.Msg {
	if day, ok := value.(string); ok {
		for _, card := range model.Cards() {
			if id == CalendarID(card) {
				return model.DatePickMsg{Day: day}
			}
		}
	}
	field, ok := controlField(id)
	if !ok {
		return nil
	}
	switch value := value.(type) {
	case string:
		return decodeChoice(field, value)
	case bool:
		return model.SwitchMsg{Field: field, On: value}
	}
	return nil
}

// DecodeToggle is the message for the pointer opening or closing the menu of the select at node id, nil for any other node.
func DecodeToggle(id string, open bool) model.Msg {
	if field, ok := controlField(id); ok {
		return model.MenuToggleMsg{Field: field, Open: open}
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

// decodeAgentStates reads the window half's JSON list of {pane, state, last_message, command, hidden}, nil when it is malformed; command and hidden are
// absent from an older window half, and a missing hidden leaves the agent's place unknown.
// pane is decimal text, or a JSON number, kept exact: block ids can exceed what a float holds.
func decodeAgentStates(value string) model.Msg {
	var list []struct {
		Pane        json.RawMessage `json:"pane"`
		State       string          `json:"state"`
		LastMessage string          `json:"last_message"`
		Command     string          `json:"command"`
		Hidden      *bool           `json:"hidden"`
	}
	if err := json.Unmarshal([]byte(value), &list); err != nil {
		return nil
	}
	states := make([]model.AgentState, 0, len(list))
	for _, item := range list {
		block, ok := blockID(item.Pane)
		if !ok {
			return nil
		}
		where := model.SeenUnknown
		switch {
		case item.Hidden == nil:
		case *item.Hidden:
			where = model.SeenHidden
		default:
			where = model.SeenShown
		}
		states = append(states, model.AgentState{Block: block, State: model.AgentStateWord(item.State), LastMessage: item.LastMessage, Command: item.Command, Hidden: where})
	}
	return model.AgentStatesMsg{States: states}
}

func blockID(raw json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, text != ""
	}
	number := string(raw)
	if _, err := strconv.ParseUint(number, 10, 64); err == nil {
		return number, true
	}
	// An exponent or fraction form of a whole number.
	f, _, err := big.ParseFloat(number, 10, 128, big.ToNearestEven)
	if err != nil || !f.IsInt() || f.Sign() < 0 {
		return "", false
	}
	i, _ := f.Int(nil)
	return i.String(), true
}
