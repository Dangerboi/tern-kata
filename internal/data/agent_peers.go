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

// saveAgentStates writes the relay's states to agent-states.json, unless the file already holds them.
func (g *Gateway) saveAgentStates(cmd model.SaveAgentStates) model.AgentStatesSaved {
	dirs, err := g.workerDirs()
	if err != nil || dirs.State == "" {
		if err == nil {
			err = fmt.Errorf("tern-kata's state folder is not known")
		}
		return model.AgentStatesSaved{Err: err}
	}
	doc := agentStatesDoc{States: make([]agentStateRecord, len(cmd.States))}
	for i, s := range cmd.States {
		doc.States[i] = agentStateRecord{Block: s.Block, State: s.State}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return model.AgentStatesSaved{Err: err}
	}
	raw = append(raw, '\n')
	path := filepath.Join(dirs.State, AgentStatesFile)
	token, err := guardedfile.Read(path)
	if err != nil || token.Text() == string(raw) {
		return model.AgentStatesSaved{Err: err}
	}
	err = guardedfile.Update(path, 0o700, func(guardedfile.Token) ([]byte, error) { return raw, nil })
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
