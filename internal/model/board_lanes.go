package model

import (
	"fmt"
	"slices"
)

// The Lane list is read from tern-kata's user config when Board mode turns on and when the Project changes while it is on. A file
// that cannot be read leaves the default Lane list and is never overwritten.

// LoadLanes asks for the saved Lane list of the Project with ProjectUID.
type LoadLanes struct{ ProjectUID string }

func (LoadLanes) cmd() {}

// LanesLoaded answers LoadLanes. No Lanes means the file or the Project has none saved; Err means the file could not be read or parsed.
type LanesLoaded struct {
	ProjectUID string
	Lanes      []string
	Err        error
}

func (LanesLoaded) msg() {}

// SaveLanes asks for Lanes to be written as the Project's Lane list; ProjectName is written beside it for readers of the file.
// No Lanes removes the Project's list, which restores the default.
type SaveLanes struct {
	ProjectUID, ProjectName string
	Lanes                   []string
}

func (SaveLanes) cmd() {}

// LanesSaved answers SaveLanes.
type LanesSaved struct {
	ProjectUID string
	Lanes      []string
	Err        error
}

func (LanesSaved) msg() {}

// SaveLaneList is how the Lanes card saves: it asks to write lanes (already normalised) as the Project's Lane list. It is
// refused with the file's error while the file could not be read, so a hand-edited file is never overwritten.
func (m Model) SaveLaneList(lanes []string) (Model, []Cmd) {
	if m.Board.fileErr != nil {
		m.Notice = fmt.Sprintf("Lanes not saved: %v", m.Board.fileErr)
		return m, nil
	}
	return m, []Cmd{SaveLanes{ProjectUID: m.Collection.ProjectUID, ProjectName: m.Collection.Title, Lanes: slices.Clone(lanes)}}
}

// lanesLoad is the load a Step's change of Board mode or Project calls for: one command when the Board has just turned on or
// its Project has changed, and the old Project's Lanes are dropped at once.
func (m Model) lanesLoad(wasOn bool, was string) (Model, []Cmd) {
	if !m.Board.On() || (wasOn && was == m.Collection.ProjectUID) {
		return m, nil
	}
	m.Board.loaded = false
	if was != m.Collection.ProjectUID {
		m.Board.Lanes, m.Board.fileErr = nil, nil
	}
	return m, []Cmd{LoadLanes{ProjectUID: m.Collection.ProjectUID}}
}

func lanesLoaded(m Model, msg LanesLoaded) Model {
	if msg.ProjectUID != m.Collection.ProjectUID {
		return m
	}
	m.Board.Lanes, m.Board.fileErr, m.Board.loaded = msg.Lanes, msg.Err, true
	if msg.Err != nil {
		m.Board.Lanes = nil
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
		m.Board.Lanes = msg.Lanes
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
