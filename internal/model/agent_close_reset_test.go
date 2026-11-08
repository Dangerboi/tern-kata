package model_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// failedStatusRead is a Pane whose exit request holds an Issue outside the Collection and whose status read for it has failed.
func failedStatusRead(t *testing.T) model.Model {
	t.Helper()
	m, _ := relay(t, exitedPane("open"), st("11", "exited", ""))
	m, _ = pruned(m, "11")
	m.Issues = nil
	m, cmds := model.Update(m, model.StreamEvent{})
	if len(reads(cmds)) != 1 {
		t.Fatalf("cmds %v", cmds)
	}
	m, _ = model.Update(m, model.AgentIssueRead{UID: "u1", Err: errors.New("daemon down")})
	if _, cmds = model.Update(m, model.StreamEvent{}); len(reads(cmds)) != 0 {
		t.Fatalf("read again before anything reset it: %v", cmds)
	}
	return m
}

// switchDaemon asks to connect this Pane to another daemon and returns the Pane with the answer still to feed.
func switchDaemon(t *testing.T, m model.Model) (model.Model, model.SwitchDaemon) {
	t.Helper()
	m, cmds := model.Update(m, model.KeyMsg{Chord: "shift+d"})
	var load model.LoadDaemons
	for _, c := range cmds {
		if l, ok := c.(model.LoadDaemons); ok {
			load = l
		}
	}
	m, _ = model.Update(m, model.DaemonsLoaded{Request: load.Request, Entries: []model.DaemonEntry{{Name: "other"}}})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "enter"})
	for _, c := range cmds {
		if s, ok := c.(model.SwitchDaemon); ok {
			return m, s
		}
	}
	t.Fatalf("no SwitchDaemon asked: %v", cmds)
	return m, model.SwitchDaemon{}
}

func TestSwitchingDaemonLetsAFailedStatusReadBeAskedAgain(t *testing.T) {
	m, ask := switchDaemon(t, failedStatusRead(t))
	m, _ = model.Update(m, model.DaemonSwitched{Request: ask.Request, Name: "other"})
	if _, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration}); len(reads(cmds)) != 1 {
		t.Fatalf("after the switch: cmds %v", cmds)
	}
}

func TestAFailedSwitchKeepsTheFailedStatusRead(t *testing.T) {
	m, ask := switchDaemon(t, failedStatusRead(t))
	m, _ = model.Update(m, model.DaemonSwitched{Request: ask.Request, Name: "other", Err: errors.New("down")})
	if _, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration}); len(reads(cmds)) != 0 {
		t.Fatalf("after the failed switch: cmds %v", cmds)
	}
}

func TestAReconnectedEventStreamLetsAFailedStatusReadBeAskedAgain(t *testing.T) {
	m := failedStatusRead(t)
	m.StreamGeneration = 2
	m, _ = model.Update(m, model.StreamOpened{Generation: 1})
	if _, cmds := model.Update(m, model.StreamEvent{}); len(reads(cmds)) != 0 {
		t.Fatalf("a stale stream's open reset it: %v", cmds)
	}
	m, _ = model.Update(m, model.StreamOpened{Generation: 2})
	if _, cmds := model.Update(m, model.StreamEvent{}); len(reads(cmds)) != 1 {
		t.Fatalf("after the reconnect: cmds %v", cmds)
	}
}
