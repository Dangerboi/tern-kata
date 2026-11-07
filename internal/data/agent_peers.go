package data

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// AgentStatesFile is the latest relay of agent states in tern-kata's state folder, beside agents.json: a Pane writes it, and
// `tern-kata agents` (which runs without a Pane, so hears no relay) reads it.
const AgentStatesFile = "agent-states.json"

// agentStatesDoc is agent-states.json: one entry per agent block a relay has listed.
type agentStatesDoc struct {
	States []agentStateRecord `json:"states"`
}

type agentStateRecord struct {
	Block string               `json:"block"`
	State model.AgentStateWord `json:"state"`
}

// saveAgentStates merges the relay's states into agent-states.json by block. Another Pane's window may see agent blocks this
// one's does not, so blocks the relay does not list keep the state the file has for them while agents.json still maps them; the
// file stays bounded by that. The read-merge-write runs under guardedfile.Update, so Panes writing at once lose nothing and a
// reader never sees a half-written file.
func (g *Gateway) saveAgentStates(cmd model.SaveAgentStates) model.AgentStatesSaved {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentStatesSaved{Err: err}
	}
	entries, err := ReadAgents(file)
	if err != nil {
		return model.AgentStatesSaved{Err: err}
	}
	mapped := make(map[string]bool, len(entries))
	for _, e := range entries {
		mapped[e.Block] = true
	}
	path := filepath.Join(filepath.Dir(file), AgentStatesFile)
	err = guardedfile.Update(path, 0o700, func(token guardedfile.Token) ([]byte, error) {
		doc := agentStatesDoc{States: make([]agentStateRecord, 0, len(cmd.States))}
		listed := make(map[string]bool, len(cmd.States))
		for _, s := range cmd.States {
			listed[s.Block] = true
			doc.States = append(doc.States, agentStateRecord{Block: s.Block, State: s.State})
		}
		var old agentStatesDoc
		if token.Text() != "" && json.Unmarshal([]byte(token.Text()), &old) == nil {
			for _, r := range old.States {
				if !listed[r.Block] && mapped[r.Block] {
					doc.States = append(doc.States, r)
				}
			}
		}
		raw, err := json.MarshalIndent(doc, "", "  ")
		return append(raw, '\n'), err
	})
	return model.AgentStatesSaved{Err: err}
}

// ReadRunningAgents is what `tern-kata agents` and `tern-kata message` work from: the agents of agents.json that the latest
// relay has not shown exited, each with its state. With no relay file every agent reads with no state.
func (g *Gateway) ReadRunningAgents() ([]model.IssueAgent, error) {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return nil, err
	}
	entries, err := ReadAgents(file)
	if err != nil {
		return nil, err
	}
	states, err := readAgentStates(filepath.Join(filepath.Dir(file), AgentStatesFile))
	if err != nil {
		return nil, err
	}
	return model.RunningAgents(entries, states), nil
}

// readAgentStates reads agent-states.json; a missing file is no states.
func readAgentStates(path string) ([]model.AgentState, error) {
	token, err := guardedfile.Read(path)
	if err != nil || token.Text() == "" {
		return nil, err
	}
	var doc agentStatesDoc
	if err := json.Unmarshal([]byte(token.Text()), &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	states := make([]model.AgentState, len(doc.States))
	for i, r := range doc.States {
		states[i] = model.AgentState{Block: r.Block, State: r.State}
	}
	return states, nil
}
