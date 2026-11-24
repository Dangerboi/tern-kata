package model

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
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

// OrderChange is what the user did to one Lane's custom order, as a value the Board shows at once and the store applies to the
// order it holds. Shown are the Lane's cards as the user saw them, top first: those the stored order does not name go first, as
// the Lane shows them. So a change to a Lane the store holds no order for starts one from what the user saw, unless OnlyOrdered
// is set, which makes the change do nothing then (a drop into a Lane another Pane reset must not bring its order back). Remove
// are uids taken out. Move, when set, is put right before Anchor, or right after it when After, or last when Anchor is empty or
// not in the order. Reset drops the Lane's order and ignores the rest.
type OrderChange struct {
	Shown        []string
	Move, Anchor string
	After        bool
	Remove       []string
	Reset        bool
	OnlyOrdered  bool
}

func (c OrderChange) clone() OrderChange {
	c.Shown, c.Remove = slices.Clone(c.Shown), slices.Clone(c.Remove)
	return c
}

// shows says the save belongs to the source the Board shows: the Project it asked about and the Lane list source it reads.
func (b BoardState) shows(save SaveLaneOrder) bool {
	return save.ProjectUID == b.asked && save.Shared == b.Shared
}

// outIndex is the place in pending of the oldest change of the Project, which is the one out; -1 when the Project has none.
func (b BoardState) outIndex(projectUID string) int {
	return slices.IndexFunc(b.pending, func(s SaveLaneOrder) bool { return s.ProjectUID == projectUID })
}

// settled says the Board shows Issues where they are: no Move and no snapshot read is out, either of which may show an Issue
// where it was.
func (m Model) settled() bool { return m.Moving == "" && !m.Refreshing }

// Apply is a copy of o with the change made to the Lane's order; o itself is never written. An order left empty is dropped. An
// OnlyOrdered change to a Lane with no order changes nothing.
func (o LaneOrders) Apply(lane string, c OrderChange) LaneOrders {
	if c.Reset {
		return o.With(lane, nil)
	}
	if c.OnlyOrdered && len(o[lane]) == 0 {
		return o
	}
	order := slices.Clone(o[lane])
	var first []string
	for _, uid := range c.Shown {
		if !slices.Contains(order, uid) && !slices.Contains(first, uid) {
			first = append(first, uid)
		}
	}
	order = append(first, order...)
	order = slices.DeleteFunc(order, func(uid string) bool { return slices.Contains(c.Remove, uid) })
	if c.Move != "" {
		order = placeInOrder(order, c.Move, c.Anchor, c.After)
	}
	return o.With(lane, order)
}

// SameLane says o and other hold the same order for the Lane.
func (o LaneOrders) SameLane(other LaneOrders, lane string) bool {
	return slices.Equal(o[lane], other[lane])
}

// placeInOrder is order with uid put right before anchor, or right after it when after; last when anchor is not in it.
func placeInOrder(order []string, uid, anchor string, after bool) []string {
	placed := slices.DeleteFunc(slices.Clone(order), func(u string) bool { return u == uid })
	at := slices.Index(placed, anchor)
	switch {
	case at < 0:
		return append(placed, uid)
	case after:
		at++
	}
	return slices.Insert(placed, at, uid)
}

// LaneOrderable says the Lane can have a custom order: every Lane of the Lane list and No lane, whose order is stored under
// its empty key (LaneNone), which no Lane label can be. Done is not orderable (newest closed first).
func LaneOrderable(lane string) bool { return lane != LaneDone }

// SaveLaneOrder asks for a change to one Lane's custom card order to be applied where the Lane list in use lives: the Project's
// LaneOrderKey metadata when Shared, else the Project's table in the Lane list file. The store applies it to the order it holds
// and writes nothing when that leaves the Lane's order as it is. Seq numbers the save among the Pane's, as the answer's Save
// carries it back, so it is matched to its change and never to another that asks the same. Prune says the Pane sent it to drop
// departed uids, not the user.
type SaveLaneOrder struct {
	Workspace, Actor, ProjectUID, ProjectName string
	ProjectID                                 int64
	Shared                                    bool
	Lane                                      string
	Change                                    OrderChange
	Seq                                       int
	Prune                                     bool
}

func (SaveLaneOrder) cmd() {}

// LaneOrderSaved answers SaveLaneOrder. Current is every Lane's order as the store holds it after the change, or as read again
// after a failed one (nil when it could not be read). Err says why the change failed.
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

// orderBlock is why the custom order cannot be written now, nil when it can: the Pane is disconnected, the Project's Lane list
// has not answered yet (a reload of a list already read does not count), the Project could not be read (so it is not known
// whether it shares its list, and an order written to the file would vanish once it reads), a shared order needs the Project as
// read with its list, an archived Project refuses metadata changes, and the Lane list file is never overwritten while it could
// not be read. A Project kata does not list has no read error and keeps using the file.
func (m Model) orderBlock() error {
	switch {
	case m.Stale:
		return ErrDisconnected
	case !m.Board.lanesAnswered:
		return errors.New("the Board is still loading its Lanes")
	case m.Board.projectID == 0 && (m.Board.Shared || m.Board.sharedErr != nil):
		reason := "the Project could not be read, so where its custom order lives is not known"
		if m.Board.sharedErr != nil {
			reason = Reason(m.Board.sharedErr) + ", so where its custom order lives is not known"
		}
		return errors.New(reason)
	case m.Board.Shared && m.Board.archived:
		return errors.New("an archived Project's shared custom order cannot be changed. Restore the Project first")
	case !m.Board.Shared && m.Board.fileErr != nil:
		return fmt.Errorf("the Lane list file could not be read (%s), so it is not overwritten", Reason(m.Board.fileErr))
	}
	return nil
}

// saveOrder is a change the user made to the Lane's custom order: it is shown at once and sent to the store, which applies it to
// the order it holds. On a settled Board, uids the Lane's order names that left the Lane are taken out with it; otherwise they
// are left to pruneOrders, as the Board may show an Issue where it was. When the order cannot be written the Board stays as it
// was, with a Notice. A change to the Lane also lifts the bar on pruning it.
func (m Model) saveOrder(lane string, change OrderChange) (Model, []Cmd) {
	if err := m.orderBlock(); err != nil {
		return m.fail(Cannot("save", "the custom order", err)), nil
	}
	if _, barred := m.Board.pruneFailed[lane]; barred {
		m.Board.pruneFailed = maps.Clone(m.Board.pruneFailed)
		delete(m.Board.pruneFailed, lane)
	}
	if !change.Reset && m.settled() {
		lanes := m.LaneList()
		change.Remove = slices.Clone(change.Remove)
		for _, uid := range m.Board.order[lane] {
			if uid != change.Move && !slices.Contains(change.Remove, uid) && m.leftLane(uid, lane, lanes) {
				change.Remove = append(change.Remove, uid)
			}
		}
	}
	return m.sendOrder(lane, change, false)
}

// sendOrder keeps change among the pending ones, which the Board shows over the stored orders until the store answers, and
// sends it unless an earlier change of the Project is still out: one write per Project is out at a time, so the store applies
// them in the order they were made, and the rest go out as answers arrive. prune says the Pane made it, not the user.
func (m Model) sendOrder(lane string, change OrderChange, prune bool) (Model, []Cmd) {
	m.orderSeq++
	save := SaveLaneOrder{Workspace: m.Workspace, Actor: m.Actor, ProjectUID: m.Collection.ProjectUID, ProjectName: m.Collection.Title,
		ProjectID: m.Board.projectID, Shared: m.Board.Shared, Lane: lane, Change: change.clone(), Seq: m.orderSeq, Prune: prune}
	idle := m.Board.outIndex(save.ProjectUID) < 0
	m.Board.pending = append(slices.Clone(m.Board.pending), save)
	m.Board.order = m.Board.shownOrders()
	if !idle {
		return m, nil
	}
	return m, []Cmd{save}
}

// shownOrders is the stored orders with every change still pending laid over them, in the order they were sent. A change for
// another Project or another Lane list source is not shown.
func (b BoardState) shownOrders() LaneOrders {
	orders := b.stored
	for _, save := range b.pending {
		if b.shows(save) {
			orders = orders.Apply(save.Lane, save.Change)
		}
	}
	return orders
}

// resetLaneOrder drops the Lane's custom order.
func resetLaneOrder(m Model, msg ResetLaneOrder) (Model, []Cmd) {
	if !m.OnBoard() || !m.LaneOrderCustom(msg.Lane) {
		return m, nil
	}
	return m.saveOrder(msg.Lane, OrderChange{Reset: true})
}

// leftLane says the Issue uid, which the Lane's order names, is one the Pane holds that is closed or in another Lane now. A uid
// of an Issue the Pane does not hold (a filter hides it) is not left.
func (m Model) leftLane(uid, lane string, lanes []string) bool {
	i := slices.IndexFunc(m.Issues, func(issue Issue) bool { return issue.UID == uid })
	return i >= 0 && (IsClosed(m.Issues[i]) || LaneOf(m.Issues[i], lanes) != lane)
}

// pruneOrders sends, for each Lane, the removal of the Issues that left the Lane from its order, so one that comes back shows
// first as a card new to the Lane does. A removal changes nothing the store already has, so Panes pruning the same departure
// write at most once and never replace another Pane's order. The removals queue behind each other, one write out at a time. It
// waits for a settled Board (no Move or read out, which may show an Issue where it was) and is silent: the user did nothing, so
// a refused or failed prune says nothing, and the same one is not sent again until the next Lane list read or a change the user
// makes to the Lane.
func (m Model) pruneOrders() (Model, []Cmd) {
	if m.Board.asked == "" || m.Board.asked != m.Collection.ProjectUID || !m.settled() || m.orderBlock() != nil {
		return m, nil
	}
	lanes := m.LaneList()
	var cmds []Cmd
	for _, lane := range append([]string{LaneNone}, lanes...) {
		gone := slices.DeleteFunc(slices.Clone(m.Board.order[lane]), func(uid string) bool { return !m.leftLane(uid, lane, lanes) })
		if len(gone) == 0 || m.Board.pruneFailed[lane] == strings.Join(gone, "\n") {
			continue
		}
		var save []Cmd
		m, save = m.sendOrder(lane, OrderChange{Remove: gone}, true)
		cmds = append(cmds, save...)
	}
	return m, cmds
}

// laneOrderSaved applies the answer to a change of a Lane's order: the change is no longer pending, the next change of its
// Project, if any, is sent, and the stored orders the answer carries are shown with the changes still pending laid over them, a
// failed one included. An answer of no pending change (from before a daemon switch, which never reuses a number) is dropped;
// one of another Project or another Lane list source leaves the shown order alone. A failed change says why in a Notice, except
// a prune, which is silent; one that tells no orders reads the Lane list again, so the Board shows what is stored.
func laneOrderSaved(m Model, msg LaneOrderSaved) (Model, []Cmd) {
	i := slices.IndexFunc(m.Board.pending, func(s SaveLaneOrder) bool { return s.Seq == msg.Save.Seq })
	if i < 0 {
		return m, nil
	}
	m.Board.pending = slices.Delete(slices.Clone(m.Board.pending), i, i+1)
	var next []Cmd
	if j := m.Board.outIndex(msg.Save.ProjectUID); j >= 0 {
		next = []Cmd{m.Board.pending[j]}
	}
	m, cmds := m.orderAnswered(msg)
	return m, append(cmds, next...)
}

// orderAnswered shows the answer to a change that is no longer pending; see laneOrderSaved.
func (m Model) orderAnswered(msg LaneOrderSaved) (Model, []Cmd) {
	prune := msg.Save.Prune
	if msg.Err != nil && !prune {
		m = m.fail(Cannot("save", "the custom order", msg.Err))
	}
	if !m.Board.shows(msg.Save) {
		return m, nil
	}
	if msg.Err == nil {
		m.Board.reads.Supersede()
	} else if prune {
		m.Board.pruneFailed = maps.Clone(m.Board.pruneFailed)
		if m.Board.pruneFailed == nil {
			m.Board.pruneFailed = map[string]string{}
		}
		m.Board.pruneFailed[msg.Save.Lane] = strings.Join(msg.Save.Change.Remove, "\n")
	}
	if msg.Current != nil {
		m.Board.stored = msg.Current
	}
	m.Board.order = m.Board.shownOrders()
	if msg.Current == nil && msg.Err != nil && !prune {
		return m.lanesRefresh()
	}
	return m, nil
}
