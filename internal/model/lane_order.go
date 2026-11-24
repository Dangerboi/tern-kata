package model

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// LaneOrderKey is the Project metadata key a Project shares its Lanes' custom card orders under, beside its shared Lane list:
// {"<lane label>": ["<issue uid>", ...]}, top card first. It is kept apart from SharedLanesKey so a drag writes only the
// orders and never conflicts with a Lanes card save.
const LaneOrderKey = "tern.lane_order"

// LaneOrders are a Project's custom card orders by Lane label, each the Issue uids top first. A Lane with no entry follows the
// list's order.
type LaneOrders map[string][]string

// SaveLaneOrder asks for one Lane's custom card order to be written where the Lane list in use lives: the Project's
// LaneOrderKey metadata when Shared, else the Project's table in the Lane list file. No Order drops the Lane's order.
type SaveLaneOrder struct {
	Workspace, Actor, ProjectUID, ProjectName string
	ProjectID                                 int64
	Shared                                    bool
	Lane                                      string
	Order                                     []string
}

func (SaveLaneOrder) cmd() {}

// LaneOrderSaved answers SaveLaneOrder. Current is every Lane's order as the store holds it after the write, or as read again
// after a failed one (nil when it could not be read). Err says why the write failed.
type LaneOrderSaved struct {
	Save    SaveLaneOrder
	Current LaneOrders
	Err     error
}

func (LaneOrderSaved) msg() {}

// ParseLaneOrders reads a decoded LaneOrderKey value, JSON or TOML: an object of Lane label to a list of non-empty Issue uids.
// A label CheckLabel would change is refused, as it names no Lane. An uid repeated in a Lane keeps its first place, and a Lane
// with no uids has no order. No value is no orders.
func ParseLaneOrders(value any) (LaneOrders, error) {
	if value == nil {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		if typed, isTyped := value.(map[string][]string); isTyped {
			object = make(map[string]any, len(typed))
			for label, uids := range typed {
				object[label] = uids
			}
		} else {
			return nil, errors.New("not an object of Lane labels")
		}
	}
	var orders LaneOrders
	for label, list := range object {
		if tidy, err := CheckLabel(label); err != nil || tidy != label {
			return nil, fmt.Errorf("%q is not a Lane label", label)
		}
		var items []any
		switch list := list.(type) {
		case []any:
			items = list
		case []string:
			for _, uid := range list {
				items = append(items, uid)
			}
		default:
			return nil, fmt.Errorf("Lane %q is not a list of Issue uids", label)
		}
		var uids []string
		for _, item := range items {
			uid, ok := item.(string)
			if !ok || uid == "" {
				return nil, fmt.Errorf("Lane %q holds something that is not an Issue uid", label)
			}
			if !slices.Contains(uids, uid) {
				uids = append(uids, uid)
			}
		}
		if len(uids) == 0 {
			continue
		}
		if orders == nil {
			orders = LaneOrders{}
		}
		orders[label] = uids
	}
	return orders, nil
}

// ResetLaneOrder asks for the Lane's custom card order to be dropped, so the Lane follows the list's order again.
type ResetLaneOrder struct {
	Lane string
}

func (ResetLaneOrder) msg() {}

// LaneOrderCustom says the Lane has a custom card order: its cards follow the order, not the list's.
func (m Model) LaneOrderCustom(lane string) bool { return len(m.Board.order[lane]) > 0 }

// saveOrder shows the Lane's new order at once (none drops it) and asks for it to be written. A shared order needs the Project
// as read with its list, and an archived Project refuses metadata changes: either way the order stays as it was, with a Notice.
func (m Model) saveOrder(lane string, order []string) (Model, []Cmd) {
	switch {
	case m.Board.Shared && m.Board.projectID == 0:
		reason := "the Project could not be read, so its shared Lane order cannot be written"
		if m.Board.sharedErr != nil {
			reason += ": " + Reason(m.Board.sharedErr)
		}
		return m.fail(Refused("save", "the Lane order", reason)), nil
	case m.Board.Shared && m.Board.archived:
		return m.fail(Refused("save", "the Lane order", "an archived Project's shared Lane order cannot be changed. Restore the Project first")), nil
	}
	orders := make(LaneOrders, len(m.Board.order)+1)
	maps.Copy(orders, m.Board.order)
	if len(order) == 0 {
		delete(orders, lane)
	} else {
		orders[lane] = order
	}
	m.Board.order = orders
	return m, []Cmd{SaveLaneOrder{Workspace: m.Workspace, Actor: m.Actor, ProjectUID: m.Collection.ProjectUID, ProjectName: m.Collection.Title,
		ProjectID: m.Board.projectID, Shared: m.Board.Shared, Lane: lane, Order: slices.Clone(order)}}
}

// resetLaneOrder drops the Lane's custom order.
func resetLaneOrder(m Model, msg ResetLaneOrder) (Model, []Cmd) {
	if !m.OnBoard() || !m.LaneOrderCustom(msg.Lane) {
		return m, nil
	}
	return m.saveOrder(msg.Lane, nil)
}

// laneOrderSaved applies the answer to a write of a Lane's order. The orders it carries are what the store holds, so they are
// shown, a failed write included; a Notice says why that one failed.
func laneOrderSaved(m Model, msg LaneOrderSaved) Model {
	if msg.Save.ProjectUID != m.Collection.ProjectUID {
		return m
	}
	if msg.Current != nil {
		m.Board.order = msg.Current
	}
	if msg.Err != nil {
		m = m.fail(Cannot("save", "the Lane order", msg.Err))
	}
	return m
}
