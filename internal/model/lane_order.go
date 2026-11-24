package model

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// LaneOrderKey is the Project metadata key a Project shares its Lanes' custom card orders under, beside its shared Lane list:
// {"<lane label>": ["<issue uid>", ...]}, top card first. It is kept apart from SharedLanesKey so a drag's write changes only
// this key and never the shared list itself; it still raises the Project's revision, which a Lanes card save guards on.
const LaneOrderKey = "tern.lane_order"

// LaneOrders are a Project's custom card orders by Lane label, each the Issue uids top first; No lane's order is under the
// empty key. A Lane with no entry follows the list's order.
type LaneOrders map[string][]string

// With is a copy of o with the Lane's order set to order, or dropped when order is empty. o itself is never written: the
// Model and the cache share it.
func (o LaneOrders) With(lane string, order []string) LaneOrders {
	out := make(LaneOrders, len(o)+1)
	maps.Copy(out, o)
	if len(order) == 0 {
		delete(out, lane)
	} else {
		out[lane] = slices.Clone(order)
	}
	return out
}

// LaneOrderable says the Lane can have a custom order: every Lane of the Lane list and No lane, whose order is stored under
// its empty key (LaneNone), which no Lane label can be. Done is not orderable (newest closed first).
func LaneOrderable(lane string) bool { return lane != LaneDone }

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
// The empty key is No lane's order. Any other key CheckLabel would change is refused, as it names no Lane. An uid repeated in a
// Lane keeps its first place, and a Lane with no uids has no order. No value is no orders.
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
		if tidy, err := CheckLabel(label); label != LaneNone && (err != nil || tidy != label) {
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

// orderBlock is why the custom order cannot be written now, nil when it can: the Pane is disconnected, a shared order needs the
// Project as read with its list and an archived Project refuses metadata changes, and the Lane list file is never overwritten
// while it could not be read.
func (m Model) orderBlock() error {
	switch {
	case m.Stale:
		return ErrDisconnected
	case m.Board.Shared && m.Board.projectID == 0:
		reason := "the Project could not be read, so its shared custom order cannot be written"
		if m.Board.sharedErr != nil {
			reason += ": " + Reason(m.Board.sharedErr)
		}
		return errors.New(reason)
	case m.Board.Shared && m.Board.archived:
		return errors.New("an archived Project's shared custom order cannot be changed. Restore the Project first")
	case !m.Board.Shared && m.Board.fileErr != nil:
		return fmt.Errorf("the Lane list file could not be read (%s), so it is not overwritten", Reason(m.Board.fileErr))
	}
	return nil
}

// saveOrder shows the Lane's new order at once (none drops it) and asks for it to be written. When the order cannot be written
// the order stays as it was, with a Notice. The gateway runs commands side by side, so one write is out at a time: a save asked
// for meanwhile waits, and replaces a waiting one of the same Lane. The write makes every Lane list read asked for before it
// stale, so none puts the old order back.
func (m Model) saveOrder(lane string, order []string) (Model, []Cmd) {
	if err := m.orderBlock(); err != nil {
		return m.fail(Cannot("save", "the custom order", err)), nil
	}
	m.Board.reads.Supersede()
	m.Board.order = m.Board.order.With(lane, order)
	save := SaveLaneOrder{Workspace: m.Workspace, Actor: m.Actor, ProjectUID: m.Collection.ProjectUID, ProjectName: m.Collection.Title,
		ProjectID: m.Board.projectID, Shared: m.Board.Shared, Lane: lane, Order: slices.Clone(order)}
	if !m.Board.saving {
		m.Board.saving = true
		return m, []Cmd{save}
	}
	waiting := slices.Clone(m.Board.waiting)
	if i := slices.IndexFunc(waiting, func(w SaveLaneOrder) bool { return w.Lane == lane }); i >= 0 {
		waiting[i] = save
	} else {
		waiting = append(waiting, save)
	}
	m.Board.waiting = waiting
	return m, nil
}

// resetLaneOrder drops the Lane's custom order.
func resetLaneOrder(m Model, msg ResetLaneOrder) (Model, []Cmd) {
	if !m.OnBoard() || !m.LaneOrderCustom(msg.Lane) {
		return m, nil
	}
	return m.saveOrder(msg.Lane, nil)
}

// leftLane says the Issue uid, which the Lane's order names, is one the Pane holds that is closed or in another Lane now. A uid
// of an Issue the Pane does not hold (a filter hides it) is not left.
func (m Model) leftLane(uid, lane string, lanes []string) bool {
	i := slices.IndexFunc(m.Issues, func(issue Issue) bool { return issue.UID == uid })
	return i >= 0 && (IsClosed(m.Issues[i]) || LaneOf(m.Issues[i], lanes) != lane)
}

// pruneOrders saves each Lane's order without the Issues that left the Lane, so one that comes back shows first as a card new
// to the Lane does. It waits for a settled Board (no Move or read out, which may show an Issue where it was) and writes only
// when something is stale, so the answer it causes finds nothing more to do. It is silent where a save would be refused.
func (m Model) pruneOrders() (Model, []Cmd) {
	if !m.Board.loaded || m.Board.asked == "" || m.Board.asked != m.Collection.ProjectUID || m.Moving != "" || m.Refreshing || m.orderBlock() != nil {
		return m, nil
	}
	lanes := m.LaneList()
	var cmds []Cmd
	for _, lane := range append([]string{LaneNone}, lanes...) {
		order := m.Board.order[lane]
		kept := slices.DeleteFunc(slices.Clone(order), func(uid string) bool { return m.leftLane(uid, lane, lanes) })
		if len(kept) == len(order) {
			continue
		}
		var save []Cmd
		m, save = m.saveOrder(lane, kept)
		cmds = append(cmds, save...)
	}
	return m, cmds
}

// laneOrderSaved applies the answer to a write of a Lane's order and sends the next waiting write. The orders it carries are
// what the store holds, so they are shown, a failed write included, unless another write is out (its answer is the one to show)
// or the Lane list changed source meanwhile. A failed write that tells no orders reads the Lane list again, so the Board shows
// what is stored, and a Notice says why it failed.
func laneOrderSaved(m Model, msg LaneOrderSaved) (Model, []Cmd) {
	if msg.Save.ProjectUID != m.Collection.ProjectUID {
		return m, nil
	}
	m.Board.saving = false
	m.Board.reads.Supersede()
	var cmds []Cmd
	for len(m.Board.waiting) > 0 && !m.Board.saving {
		next := m.Board.waiting[0]
		m.Board.waiting = slices.Clone(m.Board.waiting[1:])
		if next.Shared == m.Board.Shared {
			m.Board.saving = true
			cmds = append(cmds, next)
		}
	}
	if msg.Err != nil {
		m = m.fail(Cannot("save", "the custom order", msg.Err))
	}
	if m.Board.saving || msg.Save.Shared != m.Board.Shared {
		return m, cmds
	}
	switch {
	case msg.Current != nil:
		m.Board.order = msg.Current
	case msg.Err != nil:
		var reload []Cmd
		m, reload = m.lanesRefresh()
		cmds = append(cmds, reload...)
	}
	return m, cmds
}
