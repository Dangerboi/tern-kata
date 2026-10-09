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
		section, err := strconv.Atoi(input.Value)
		if err == nil && section >= 0 && section <= int(model.TabLinks) {
			return model.ActivityTabMsg{Tab: model.ActivityTab(section)}
		}
	case "detail-item":
		sectionText, key, ok := strings.Cut(input.Value, ":")
		section, err := strconv.Atoi(sectionText)
		if ok && err == nil && section >= 0 && section <= int(model.TabLinks) {
			return model.DetailItemMsg{Tab: model.ActivityTab(section), Key: key}
		}
	case "jump":
		return model.JumpIssueMsg{UID: input.Value}
	}
	return nil
}
