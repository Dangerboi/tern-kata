package view_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func decodeClick(id, click string) model.Msg {
	act, value, _ := strings.Cut(click, "=")
	return view.DecodeAction(id, act, value)
}

func TestClicksDecodeToTheirMessages(t *testing.T) {
	for _, tt := range []struct {
		id, click string
		want      model.Msg
	}{
		{"", view.KeyClick("ctrl+o"), model.KeyMsg{Chord: "ctrl+o"}},
		{"", view.JumpClick("peer"), model.JumpIssueMsg{UID: "peer"}},
		{"", view.FocusDetailClick, model.FocusColumnMsg{Column: model.ColumnDetail}},
		{"", view.ActivityTabClick(model.TabLinks), model.ActivityTabMsg{Tab: model.TabLinks}},
		{"", view.DetailItemClick(model.TabEvents, "e:1"), model.DetailItemMsg{Tab: model.TabEvents, Key: "e:1"}},
		{"", view.ToggleClick("parent"), model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}},
		{"", view.OpenRowClick("leaf"), model.ActionMsg{Action: model.OpenRow, Value: "leaf"}},
		{"", view.MenuClick, model.ActionMsg{Action: model.OpenPicker}},
		{view.RetryID, view.RetryClick, model.Retry{}},
	} {
		if got := decodeClick(tt.id, tt.click); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q decodes to %#v, want %#v", tt.click, got, tt.want)
		}
	}
}

func TestClicksThePaneDoesNotUseDecodeToNothing(t *testing.T) {
	for _, tt := range []struct{ id, click string }{
		{"", "unknown=1"},
		{"", "activity-tab=9"},
		{"", "activity-tab=x"},
		{"", "detail-item=1"},
		{"", "detail-item=9:row"},
		{"elsewhere", view.RetryClick},
	} {
		if got := decodeClick(tt.id, tt.click); got != nil {
			t.Errorf("%q on %q decodes to %#v", tt.click, tt.id, got)
		}
	}
}

func TestAgentStatesDecodeKeepsBlockIdsExact(t *testing.T) {
	const big = "144115188075855871" // 2^57 - 1: not a float64
	for _, tt := range []struct {
		name, value string
		want        model.Msg
	}{
		{"string pane", `[{"pane":"` + big + `","state":"working","last_message":"hi"}]`,
			model.AgentStatesMsg{States: []model.AgentState{{Block: big, State: "working", LastMessage: "hi"}}}},
		{"number pane", `[{"pane":` + big + `,"state":"idle"}]`,
			model.AgentStatesMsg{States: []model.AgentState{{Block: big, State: "idle"}}}},
		{"exponent pane", `[{"pane":1e3,"state":"exited"}]`,
			model.AgentStatesMsg{States: []model.AgentState{{Block: "1000", State: "exited"}}}},
		{"empty list", `[]`, model.AgentStatesMsg{States: []model.AgentState{}}},
		{"not JSON", `[{`, nil},
		{"not a list", `{"pane":"1"}`, nil},
		{"pane missing", `[{"state":"idle"}]`, nil},
		{"pane fractional", `[{"pane":1.5,"state":"idle"}]`, nil},
	} {
		if got := view.DecodeAction("", string(model.AgentStatesAction), tt.value); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

func TestTheAgentClickDecodesToTheBlockExactly(t *testing.T) {
	const big = "144115188075855871"
	act, value, _ := strings.Cut(view.AgentClick(big), "=")
	if got := view.DecodeAction("", act, value); !reflect.DeepEqual(got, model.AgentJumpMsg{Block: big}) {
		t.Errorf("got %#v", got)
	}
}
