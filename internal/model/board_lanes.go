package model

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The Lane list is read whenever a Project Collection becomes current: the Project's shared list (its tern.lanes metadata)
// when it holds a valid one, else tern-kata's user config, else the default. A file that cannot be read is never
// overwritten.

// LoadLanes asks for the Lane list of the Project with ProjectUID, on the daemon of Workspace. Request numbers the asks: an
// answer to an older one is dropped.
type LoadLanes struct {
	Workspace, ProjectUID string
	Request               int64
}

func (LoadLanes) cmd() {}

// LanesLoaded answers LoadLanes. No Lanes means the default list. Shared says Lanes and the limits are the Project's tern.lanes,
// else they are the file's. Err means the file could not be read or parsed, whichever list is used.
type LanesLoaded struct {
	ProjectUID          string
	Request             int64
	Lanes               []string
	DoneDays, DoneCards int
	Shared              bool
	// ProjectID and Revision are the Project as the daemon listed it with the list; ProjectID is 0 when it was not read.
	ProjectID, Revision int64
	// Archived says the Project is archived: kata still lists it with its list but refuses any change to its metadata.
	Archived  bool
	SharedErr error
	Err       error
}

func (LanesLoaded) msg() {}

// SaveLanes asks for Lanes to be written as the Project's Lane list; ProjectName is written beside it for readers of the file.
// No Lanes restores the default list; DoneDays and DoneCards, zero for the default, are written beside it. With every part
// default the Project's table is removed.
type SaveLanes struct {
	ProjectUID, ProjectName string
	Lanes                   []string
	DoneDays, DoneCards     int
}

func (SaveLanes) cmd() {}

type LanesSaved struct {
	ProjectUID          string
	Lanes               []string
	DoneDays, DoneCards int
	Err                 error
}

func (LanesSaved) msg() {}

// ShareLanes asks for Lanes and Done's limits to be written as the Project's tern.lanes metadata, as Actor, guarded on Revision:
// the Project revision the Lanes card opened with. Stop clears the key instead, so the Project shares no list.
type ShareLanes struct {
	Workspace, Actor, ProjectUID string
	ProjectID, Revision          int64
	Lanes                        []string
	DoneDays, DoneCards          int
	Stop                         bool
}

func (ShareLanes) cmd() {}

// LanesShared answers ShareLanes. Revision is the Project's revision after the write. Conflict says the Project changed since
// the card opened: Current is the Project's shared list as it stands now, at Current.Revision.
type LanesShared struct {
	Share    ShareLanes
	Revision int64
	Conflict bool
	Current  LanesLoaded
	Err      error
}

func (LanesShared) msg() {}

func (s ShareLanes) same(o ShareLanes) bool {
	return s.ProjectUID == o.ProjectUID && s.Revision == o.Revision && s.Stop == o.Stop && s.DoneDays == o.DoneDays &&
		s.DoneCards == o.DoneCards && slices.Equal(s.Lanes, o.Lanes)
}

// shareLaneList is how the Lanes card shares or stops sharing: one guarded write of the Project's tern.lanes. It is refused
// when the Project was not read with the list, since the write needs its id and revision, and when it is archived, which kata
// refuses.
func (m Model) shareLaneList(lanes []string, doneDays, doneCards int, stop bool) (Model, []Cmd) {
	form := &m.LanesForm
	if form.refuseStale(m, "sharing the Lane list") {
		return m, nil
	}
	if form.ProjectID == 0 {
		form.Err = errors.New("The Project could not be read, so its shared Lane list cannot be written.")
		if m.Board.sharedErr != nil {
			form.Err = fmt.Errorf("The Project could not be read, so its shared Lane list cannot be written: %w", m.Board.sharedErr)
		}
		return m, nil
	}
	if m.Board.archived {
		form.Err = errors.New("An archived Project's shared Lane list can't be changed. Restore the Project first, or save to This machine.")
		return m, nil
	}
	share := ShareLanes{Workspace: m.Workspace, Actor: m.Actor, ProjectUID: m.Collection.ProjectUID, ProjectID: form.ProjectID,
		Revision: form.Revision, Lanes: slices.Clone(lanes), DoneDays: doneDays, DoneCards: doneCards, Stop: stop}
	form.Saving, form.Err, form.SentShare = true, nil, share
	return m, []Cmd{share}
}

// lanesLoad is the load a Step calls for: one command when a Project Collection becomes current, whether or not the Board is on,
// because the list colours Issues by Lane too, and one more each time Board mode turns on, so a change made in another Pane or by
// hand shows up. Other Collections use the default Lane list and load nothing. A new Project's Lanes are dropped at once; a reload
// for the same Project keeps showing the list already held until it answers.
func (m Model) lanesLoad(boardWasOn bool) (Model, []Cmd) {
	uid := ""
	if m.Collection.IsProject() {
		uid = m.Collection.ProjectUID
	}
	if uid == m.Board.asked {
		if uid == "" || boardWasOn || !m.OnBoard() {
			return m, nil
		}
		m.Board.loaded = false
		return m, []Cmd{m.loadLanesCmd(uid)}
	}
	m.Board = BoardState{Mode: m.Board.Mode, Lane: m.Board.Lane, asked: uid, reads: m.Board.reads}
	if uid == "" {
		return m, nil
	}
	return m, []Cmd{m.loadLanesCmd(uid)}
}

// lanesRefresh reads the current Project's Lane list again after another client changed a Project's metadata, which may be its
// shared list. The list already held stays shown until the read answers.
func (m Model) lanesRefresh() (Model, []Cmd) {
	if m.Board.asked == "" {
		return m, nil
	}
	return m, []Cmd{m.loadLanesCmd(m.Board.asked)}
}

func (m *Model) loadLanesCmd(uid string) LoadLanes {
	return LoadLanes{Workspace: m.Workspace, ProjectUID: uid, Request: m.Board.reads.Ask()}
}

// lanesLoaded applies the answer to the newest read. An older answer, sent before a save took effect, changes nothing and, when
// that save wanted a fresh read, asks for it.
func lanesLoaded(m Model, msg LanesLoaded) (Model, []Cmd) {
	current, again := m.Board.reads.Answer(msg.Request)
	if again && m.Board.asked != "" {
		m.Board.loaded = false
		return m, []Cmd{m.loadLanesCmd(m.Board.asked)}
	}
	if !current || msg.ProjectUID != m.Collection.ProjectUID {
		return m, nil
	}
	m = applyLanesLoaded(m, msg)
	if m.lanesWanted && m.OnBoard() {
		m = openLanesForm(m)
	}
	return m, nil
}

func applyLanesLoaded(m Model, msg LanesLoaded) Model {
	m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = msg.Lanes, msg.DoneDays, msg.DoneCards
	if m.Board.DoneDays > maxDoneDays {
		m.Board.DoneDays = 0
	}
	if m.Board.DoneCards > maxDoneCards {
		m.Board.DoneCards = 0
	}
	m.Board.Shared, m.Board.projectID, m.Board.revision, m.Board.sharedErr = msg.Shared, msg.ProjectID, msg.Revision, msg.SharedErr
	m.Board.archived = msg.Archived
	m.Board.fileErr, m.Board.loaded = msg.Err, true
	var notices []string
	if msg.SharedErr == nil {
		m.Board.reported = ""
	} else if text := msg.SharedErr.Error(); text != m.Board.reported {
		m.Board.reported = text
		notices = append(notices, fmt.Sprintf("Shared Lane list not used: %v.", msg.SharedErr))
	}
	if msg.Err != nil && !msg.Shared {
		m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = nil, 0, 0
		notices = append(notices, fmt.Sprintf("Lane list file unreadable, showing the default Lanes: %v", msg.Err))
	}
	if len(notices) > 0 {
		m.Notice = strings.Join(notices, " ")
	}
	if m.waitingForFollow() {
		return m
	}
	m = m.landFollow().keepLane()
	if m.Focus == ColumnDetail && m.OpenedIssueUID() != "" && m.SelectedUID == m.OpenedIssueUID() {
		return m // the Issue opened beside the Lane view stays shown whichever Lane it is in
	}
	return m.WithVisibleSelection()
}

func (s SaveLanes) answeredBy(msg LanesSaved) bool {
	return msg.ProjectUID == s.ProjectUID && msg.DoneDays == s.DoneDays && msg.DoneCards == s.DoneCards && slices.Equal(msg.Lanes, s.Lanes)
}

// lanesSaved applies a write of the Lane list file. While the Project shares a list, that list stays in use and a Notice says so.
func lanesSaved(m Model, msg LanesSaved) Model {
	if m.LanesForm.Open && m.LanesForm.Saving && m.LanesForm.Sent.answeredBy(msg) {
		if msg.Err != nil {
			m.LanesForm.Saving, m.LanesForm.Err = false, fmt.Errorf("Lanes not saved: %w", msg.Err)
			return m
		}
		m.LanesForm = LanesFormState{}
	}
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Lanes not saved: %v", msg.Err)
		return m
	}
	if msg.ProjectUID != m.Collection.ProjectUID {
		return m
	}
	m.Board.reads.Supersede()
	if m.Board.Shared {
		m.Notice = "Lanes saved on this machine. The Project's shared list stays in use until sharing stops."
		return m
	}
	m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = msg.Lanes, msg.DoneDays, msg.DoneCards
	return m.keepLane().WithVisibleSelection()
}

// lanesShared applies a write of the Project's shared list. A conflict keeps the card and its text, shows the list the Project
// shares now and guards the next save on the revision read with it, so saving again replaces that list knowingly.
func lanesShared(m Model, msg LanesShared) (Model, []Cmd) {
	if m.LanesForm.Open && m.LanesForm.Saving && m.LanesForm.SentShare.same(msg.Share) {
		m.LanesForm.Saving = false
		switch {
		case msg.Conflict:
			m.LanesForm.Revision, m.LanesForm.Err = msg.Current.Revision, conflictError(msg.Current)
			return m, nil
		case msg.Err != nil:
			m.LanesForm.Err = fmt.Errorf("%s: %w", shareFailure(msg.Share), msg.Err)
			return m, nil
		}
		m.LanesForm = LanesFormState{}
	}
	switch {
	case msg.Conflict:
		m.Notice = shareFailure(msg.Share) + ": the Project changed meanwhile."
		return m, nil
	case msg.Err != nil:
		m.Notice = fmt.Sprintf("%s: %v", shareFailure(msg.Share), msg.Err)
		return m, nil
	case msg.Share.ProjectUID != m.Collection.ProjectUID:
		return m, nil
	}
	m.Board.reads.Supersede()
	if msg.Share.Stop {
		m.Notice = "The Project no longer shares a Lane list."
		return m.lanesRefresh()
	}
	m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = msg.Share.Lanes, msg.Share.DoneDays, msg.Share.DoneCards
	m.Board.Shared, m.Board.revision, m.Board.sharedErr, m.Board.reported = true, msg.Revision, nil, ""
	return m.keepLane().WithVisibleSelection(), nil
}

func shareFailure(s ShareLanes) string {
	if s.Stop {
		return "Sharing not stopped"
	}
	return "Lanes not shared"
}

func conflictError(current LanesLoaded) error {
	now := "nothing"
	switch {
	case current.SharedErr != nil:
		now = "a value that is not a Lane list"
	case current.Shared:
		lanes := current.Lanes
		if len(lanes) == 0 {
			lanes = DefaultLanes()
		}
		now = strings.Join(lanes, ", ")
		if current.DoneDays > 0 || current.DoneCards > 0 {
			days, cards := cmp.Or(current.DoneDays, DefaultDoneDays), cmp.Or(current.DoneCards, DefaultDoneCards)
			now += fmt.Sprintf(" (Done: %d days, %d cards)", days, cards)
		}
	}
	return fmt.Errorf("The Project changed since this card opened; it shares %s now. Save again to replace it.", now)
}

func (m Model) keepLane() Model {
	if !m.LaneViewShown() || slices.ContainsFunc(m.BoardColumns(), func(c BoardColumn) bool { return c.Lane == m.Board.Lane }) {
		return m
	}
	return m.laneView()
}
