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
// The Luau window hook keeps its own copy of the Issue list id and the select-issue, wrap-up-sweep and agent-states actions;
// agent-states carries the JSON agent list as its value and is not a click.
const (
	keyAct             = "key"
	buttonAct          = "press"
	jumpAct            = "jump"
	activityTabAct     = "activity-tab"
	detailItemAct      = "detail-item"
	chooseAct          = "choose"
	cardAct            = "card"
	refAct             = "ref"
	jobAct             = "job"
	runAct             = "run"
	attentionAct       = "attention"
	openJobAct         = "open-job"
	jobOpenAct         = "job-open"
	runOpenAct         = "run-open"
	attentionOpenAct   = "attention-open"
	workflowOpenAct    = "workflow-open"
	agentAct           = "agent"
	configAct          = "config"
	contextMenuPickAct = "pick"
)

const (
	FocusDetailClick = string(model.FocusDetail)
	RetryClick       = string(model.RetryAction)
	MenuClick        = string(model.OpenPicker)
	NoticeLinkClick  = "notice-link"
)

func click(act, value string) string { return act + "=" + value }

func KeyClick(chord string) string { return click(keyAct, chord) }

// ButtonClick is the click of a card's button: its chord pressed with the card as it stands, so no overlay of the card (its open
// calendar or dropdown) takes the chord in place of the button.
func ButtonClick(chord string) string { return click(buttonAct, chord) }

func JumpClick(uid string) string { return click(jumpAct, uid) }

func ToggleClick(uid string) string  { return click(string(model.ToggleExpansion), uid) }
func OpenRowClick(uid string) string { return click(string(model.OpenRow), uid) }

func AgentClick(block string) string { return click(agentAct, block) }

func CardClick(uid string) string { return click(cardAct, uid) }

func ContextMenuPickClick(uid string, mode model.Mode, chord string) string {
	return click(contextMenuPickAct, string(mode)+":"+uid+":"+chord)
}

func JobClick(uid string) string { return click(jobAct, uid) }

func RunClick(uid string) string { return click(runAct, uid) }

func AttentionClick(uid string) string { return click(attentionAct, uid) }

func OpenJobClick(uid string) string { return click(openJobAct, uid) }

func JobOpenClick(uid string) string       { return click(jobOpenAct, uid) }
func RunOpenClick(uid string) string       { return click(runOpenAct, uid) }
func AttentionOpenClick(uid string) string { return click(attentionOpenAct, uid) }
func WorkflowOpenClick(uid string) string  { return click(workflowOpenAct, uid) }

func ConfigActClick(key model.ConfigFileKey, action model.ConfigAction) string {
	return click(configAct, string(key)+":"+string(action))
}

// RefClick selects the Board card of a related Issue, revealing it in its Lane, or opens its detail when the Board does not draw it.
func RefClick(uid string) string { return click(refAct, uid) }

func ActivityTabClick(tab model.ActivityTab) string {
	return click(activityTabAct, strconv.Itoa(int(tab)))
}

func DetailItemClick(tab model.ActivityTab, key string) string {
	return click(detailItemAct, strconv.Itoa(int(tab))+":"+key)
}

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

var uidActs = map[string]func(value string) model.Msg{
	jumpAct:          func(v string) model.Msg { return model.JumpIssueMsg{UID: v} },
	agentAct:         func(v string) model.Msg { return model.AgentJumpMsg{Block: v} },
	cardAct:          func(v string) model.Msg { return model.SelectMsg{UID: v} },
	refAct:           func(v string) model.Msg { return model.ShowCardMsg{UID: v} },
	runAct:           func(v string) model.Msg { return model.RunSelectMsg{UID: v} },
	attentionAct:     func(v string) model.Msg { return model.AttentionSelectMsg{UID: v} },
	openJobAct:       func(v string) model.Msg { return model.OpenJobMsg{UID: v} },
	jobAct:           func(v string) model.Msg { return model.JobSelectMsg{UID: v} },
	jobOpenAct:       func(v string) model.Msg { return model.JobOpenMsg{UID: v} },
	runOpenAct:       func(v string) model.Msg { return model.RunOpenMsg{UID: v} },
	attentionOpenAct: func(v string) model.Msg { return model.AttentionOpenMsg{UID: v} },
	workflowOpenAct:  func(v string) model.Msg { return model.WorkflowOpenMsg{UID: v} },
}

func DecodeAction(id, act, value string) model.Msg {
	switch act {
	case keyAct:
		return model.KeyMsg{Chord: value}
	case buttonAct:
		return model.ButtonPressMsg{Chord: value}
	case string(model.AgentStatesAction):
		return decodeAgentStates(value)
	case string(model.SelectIssueAction), string(model.WrapUpSweepAction), string(model.ToggleExpansion), string(model.OpenRow), MenuClick:
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
	case contextMenuPickAct:
		if mode, rest, found := strings.Cut(value, ":"); found {
			if uid, chord, found := strings.Cut(rest, ":"); found {
				return model.ContextMenuPickMsg{UID: uid, Mode: model.Mode(mode), Chord: chord}
			}
		}
	case configAct:
		if key, action, found := strings.Cut(value, ":"); found {
			return model.ConfigActMsg{Key: model.ConfigFileKey(key), Action: model.ConfigAction(action)}
		}
	}
	if decode, ok := uidActs[act]; ok {
		return decode(value)
	}
	return nil
}

// The items of a list are its children, named list id + "." + key.
func itemID(list, key string) string   { return list + "." + key }
func itemKey(list, item string) string { return strings.TrimPrefix(item, list+".") }

func selectedItem(list, key string) string {
	if key == "" {
		return ""
	}
	return itemID(list, key)
}

func IssueID(uid string) string { return itemID(IssueListID, uid) }

func IsIssueID(id string) bool { return strings.HasPrefix(id, itemID(IssueListID, "")) }

func IsBoardCardID(id string) bool { return strings.HasPrefix(id, BoardID+".") }

func PickerItemID(key string) string { return itemID(PickerListID, key) }

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

func DecodeChange(id string, value any) model.Msg {
	if day, ok := value.(string); ok {
		for _, card := range model.Cards() {
			if id == CalendarID(card) {
				return model.CalendarDayMsg{Day: day}
			}
		}
	}
	if on, ok := value.(bool); ok && id == MoveSingleID {
		return model.MoveSingleMsg{On: on}
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

func DecodeToggle(id string, open bool) model.Msg {
	if field, ok := controlField(id); ok {
		return model.DropdownToggleMsg{Field: field, Open: open}
	}
	return nil
}

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
	f, _, err := big.ParseFloat(number, 10, 128, big.ToNearestEven)
	if err != nil || !f.IsInt() || f.Sign() < 0 {
		return "", false
	}
	i, _ := f.Int(nil)
	return i.String(), true
}
