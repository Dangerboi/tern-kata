package model

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

// AgentRecordPrefix starts the metadata key of an Agent record: the prefix, the Host id of the Pane that started the agent, a
// hyphen and the agent's block id.
const AgentRecordPrefix = "tern-kata.agent."

// AgentRecord is what a Pane publishes on an agent's Issue for the agent it started (ADR-0018), so Panes on other hosts find it.
// It is static: written once at start, removed once at the end, with no state and no heartbeat.
type AgentRecord struct {
	// Address is the agent's Agent address; "" for an agent without one (NoAddress says why).
	Address string `json:"address"`
	// Host is the display name of the host (agent.toml's host, default the machine's host name); HostID is what identifies it.
	Host   string `json:"host"`
	HostID string `json:"host_id"`
	Block  string `json:"block"`
	// Name is the agent's teammate name, Actor the actor the Pane started it as.
	Name      string         `json:"name"`
	Actor     string         `json:"actor"`
	StartedAt time.Time      `json:"started_at"`
	NoAddress AgentNoAddress `json:"no_address"`
	// MessageFrom is the actors, besides Actor, whose messages the agent acts on.
	MessageFrom []string `json:"message_from"`
}

func AgentRecordKey(hostID, block string) string { return AgentRecordPrefix + hostID + "-" + block }

// ParseAgentRecordKey splits an Agent record's key into the Host id (which holds no hyphen) and the block id.
func ParseAgentRecordKey(key string) (hostID, block string, ok bool) {
	rest, ok := strings.CutPrefix(key, AgentRecordPrefix)
	if !ok {
		return "", "", false
	}
	hostID, block, ok = strings.Cut(rest, "-")
	return hostID, block, ok && hostID != "" && block != ""
}

// ReadAgentRecord reads the record stored under key with the JSON value raw. The key must be a record's, and the value must name
// the same Host id and block as the key does; one that does not read is an error.
func ReadAgentRecord(key, raw string) (AgentRecord, error) {
	hostID, block, ok := ParseAgentRecordKey(key)
	if !ok {
		return AgentRecord{}, errors.New("the key is not an Agent record")
	}
	var r AgentRecord
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return AgentRecord{}, err
	}
	if r.HostID != hostID || r.Block != block {
		return AgentRecord{}, errors.New("the Agent record names another host or block than its key")
	}
	return r, nil
}

// AgentRecords reads the Agent records in an Issue's metadata as a snapshot carries it, oldest start first; a value that does not
// read as a record is left out.
func AgentRecords(metadata map[string]any) []AgentRecord {
	var records []AgentRecord
	for key, value := range metadata {
		if !strings.HasPrefix(key, AgentRecordPrefix) {
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if r, err := ReadAgentRecord(key, string(raw)); err == nil {
			records = append(records, r)
		}
	}
	slices.SortFunc(records, func(a, b AgentRecord) int {
		if c := a.StartedAt.Compare(b.StartedAt); c != 0 {
			return c
		}
		return strings.Compare(AgentRecordKey(a.HostID, a.Block), AgentRecordKey(b.HostID, b.Block))
	})
	return records
}
