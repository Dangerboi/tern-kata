package model

import (
	"fmt"
	"slices"
)

// The Lane list is read from tern-kata's user config whenever a Project Collection becomes current. A file
// that cannot be read leaves the default Lane list and is never overwritten.

// LoadLanes asks for the saved Lane list of the Project with ProjectUID.
type LoadLanes struct{ ProjectUID string }

func (LoadLanes) cmd() {}

// LanesLoaded answers LoadLanes. No Lanes means the file or the Project has none saved; Err means the file could not be read or parsed.
type LanesLoaded struct {
	ProjectUID string
	Lanes      []string
	// DoneDays and DoneCards are the Project's Done limits; zero means the default.
	DoneDays, DoneCards int
	Err                 error
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

// LanesSaved answers SaveLanes.
type LanesSaved struct {
	ProjectUID          string
	Lanes               []string
	DoneDays, DoneCards int
	Err                 error
}

func (LanesSaved) msg() {}

// SaveLaneList is how the Lanes card saves: it asks to write lanes (already normalised) as the Project's Lane list, with its Done limits. It is
// refused with the file's error while the file could not be read, so a hand-edited file is never overwritten.
func (m Model) SaveLaneList(lanes []string, doneDays, doneCards int) (Model, []Cmd) {
	if m.Board.fileErr != nil {
		m.Notice = fmt.Sprintf("Lanes not saved: %v", m.Board.fileErr)
		return m, nil
	}
	return m, []Cmd{SaveLanes{ProjectUID: m.Collection.ProjectUID, ProjectName: m.Collection.Title, Lanes: slices.Clone(lanes), DoneDays: doneDays, DoneCards: doneCards}}
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
		if uid == "" || boardWasOn || !m.Board.On() {
			return m, nil
		}
		m.Board.loaded = false
		return m, []Cmd{LoadLanes{ProjectUID: uid}}
	}
	m.Board.asked, m.Board.loaded = uid, false
	m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards, m.Board.fileErr = nil, 0, 0, nil
	if uid == "" {
		return m, nil
	}
	return m, []Cmd{LoadLanes{ProjectUID: uid}}
}

func lanesLoaded(m Model, msg LanesLoaded) Model {
	if msg.ProjectUID != m.Collection.ProjectUID {
		return m
	}
	m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = msg.Lanes, msg.DoneDays, msg.DoneCards
	m.Board.fileErr, m.Board.loaded = msg.Err, true
	if msg.Err != nil {
		m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = nil, 0, 0
		m.Notice = fmt.Sprintf("Lane list file unreadable, showing the default Lanes: %v", msg.Err)
	}
	return m.keepLane().WithVisibleSelection()
}

func lanesSaved(m Model, msg LanesSaved) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Lanes not saved: %v", msg.Err)
		return m
	}
	if msg.ProjectUID == m.Collection.ProjectUID {
		m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = msg.Lanes, msg.DoneDays, msg.DoneCards
	}
	return m.keepLane().WithVisibleSelection()
}

// keepLane re-picks the Lane view's Lane when a new Lane list dropped it.
func (m Model) keepLane() Model {
	if m.Board.Mode != BoardLane || slices.ContainsFunc(m.BoardColumns(), func(c BoardColumn) bool { return c.Lane == m.Board.Lane }) {
		return m
	}
	return m.laneView()
}
