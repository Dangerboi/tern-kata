package model

import (
	"errors"
	"slices"
	"testing"
)

func failedReadPane() Model {
	m := Model{Workspace: "/w", Issues: nil}
	m.AgentClose = agentCloseState{
		reading:    []string{"r"},
		unreadable: []string{"u1"},
		known:      map[string]string{"k": "open"},
	}
	return m
}

func TestSwitchingDaemonForgetsTheFailedStatusReadsAndWhatWasKnown(t *testing.T) {
	m := failedReadPane()
	request := m.Daemons.switches.Ask()
	m, _ = Update(m, DaemonSwitched{Request: request, Name: "other"})
	if len(m.AgentClose.unreadable) != 0 || len(m.AgentClose.reading) != 0 || len(m.AgentClose.known) != 0 {
		t.Fatalf("after the switch: %+v", m.AgentClose)
	}
}

func TestAFailedSwitchKeepsTheFailedStatusReads(t *testing.T) {
	m := failedReadPane()
	request := m.Daemons.switches.Ask()
	m, _ = Update(m, DaemonSwitched{Request: request, Name: "other", Err: errors.New("down")})
	if !slices.Equal(m.AgentClose.unreadable, []string{"u1"}) {
		t.Fatalf("after the failed switch: %+v", m.AgentClose)
	}
}

func TestAReconnectedEventStreamForgetsTheFailedStatusReadsOnly(t *testing.T) {
	m := failedReadPane()
	m.StreamGeneration = 2
	m, _ = Update(m, StreamOpened{Generation: 1})
	if !slices.Equal(m.AgentClose.unreadable, []string{"u1"}) {
		t.Fatalf("a stale stream's open reset them: %+v", m.AgentClose)
	}
	m, _ = Update(m, StreamOpened{Generation: 2})
	if len(m.AgentClose.unreadable) != 0 {
		t.Fatalf("after the reconnect: %+v", m.AgentClose)
	}
	if !slices.Equal(m.AgentClose.reading, []string{"r"}) || m.AgentClose.known["k"] != "open" {
		t.Fatalf("the reconnect dropped more than the failed reads: %+v", m.AgentClose)
	}
}
