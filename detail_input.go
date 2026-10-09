package main

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func detailAction(input *tern.Action) model.Msg {
	if input.Act == string(model.SelectIssueAction) {
		return model.ActionMsg{Action: model.SelectIssueAction, Value: input.Value}
	}
	switch input.Act {
	case "key":
		return model.ActionMsg{Action: "key", Value: input.Value}
	case string(model.ToggleExpansion):
		return model.ActionMsg{Action: model.ToggleExpansion, Value: input.Value}
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
