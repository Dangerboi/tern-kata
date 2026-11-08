package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// AgentSessionSuffix names the file beside an agent's prompt file that holds the path of its omp session file; tern-kata's omp
// extension writes it on session start, on every session switch or branch and on every agent start (its SESSION_PATH_SUFFIX), and removePrompt removes it.
const AgentSessionSuffix = ".session"

const agentSessionTail = 1 << 20

const (
	agentSessionNoPath = "no session yet: omp has not named its session file"
	agentSessionNoFile = "no session file yet"
	agentSessionUnread = "session file unreadable"
)

func (g *Gateway) waitAgentSessions(ctx context.Context, cmd model.WaitAgentSessions) model.AgentSessionTick {
	g.pause(ctx, model.AgentSessionEvery)
	return model.AgentSessionTick(cmd)
}

func (g *Gateway) readAgentSessions(ctx context.Context, cmd model.ReadAgentSessions) model.AgentSessionsRead {
	out := model.AgentSessionsRead{Request: cmd.Request, Sessions: make([]model.AgentSessionFacts, 0, len(cmd.Agents))}
	for _, a := range cmd.Agents {
		if ctx.Err() != nil {
			break
		}
		s := readAgentSession(a)
		s.Block = a.Block
		out.Sessions = append(out.Sessions, s)
	}
	return out
}

func readAgentSession(a model.AgentSessionAsk) model.AgentSessionFacts {
	if a.PromptFile == "" {
		return model.AgentSessionFacts{Reason: agentSessionNoPath}
	}
	raw, err := os.ReadFile(a.PromptFile + AgentSessionSuffix)
	path := strings.TrimSpace(string(raw))
	if err != nil || path == "" {
		return model.AgentSessionFacts{Reason: agentSessionNoPath}
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.AgentSessionFacts{Reason: agentSessionNoFile}
		}
		return model.AgentSessionFacts{Reason: agentSessionUnread}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return model.AgentSessionFacts{Reason: agentSessionUnread}
	}
	s, err := parseSessionTail(f, info.Size(), agentSessionTail)
	if err != nil {
		return model.AgentSessionFacts{Reason: agentSessionUnread}
	}
	return s
}

// parseSessionTail reads at most the last limit bytes of the omp session JSONL in r, size bytes long, and derives what the Agent
// card shows. omp writes one JSON entry per line; an assistant `message` entry carries the model, `usage.cost.total` (summed over
// the assistant messages, as the worker sums usage, ADR-0009) and its `toolCall` content parts, and a `toolResult` message
// answers one by `toolCallId`. The current tool is the last toolCall with no toolResult after it. When the file is longer than
// limit the cut first line is dropped and the session is Partial: ToolCalls and Cost then count only the tail.
func parseSessionTail(r io.ReaderAt, size, limit int64) (model.AgentSessionFacts, error) {
	start := max(size-limit, 0)
	buf := make([]byte, size-start)
	if _, err := r.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return model.AgentSessionFacts{}, err
	}
	var s model.AgentSessionFacts
	if start > 0 {
		s.Partial = true
		if nl := bytes.IndexByte(buf, '\n'); nl >= 0 {
			buf = buf[nl+1:]
		} else {
			buf = nil
		}
	}
	pending := map[string]string{}
	var order []string
	for len(buf) > 0 {
		line := buf
		if nl := bytes.IndexByte(buf, '\n'); nl >= 0 {
			line, buf = buf[:nl], buf[nl+1:]
		} else {
			buf = nil
		}
		foldSessionLine(&s, bytes.TrimSpace(line), pending, &order)
	}
	for i := len(order) - 1; i >= 0; i-- {
		if name, open := pending[order[i]]; open {
			s.CurrentTool = name
			break
		}
	}
	return s, nil
}

type sessionEntry struct {
	Type    string `json:"type"`
	Message *struct {
		Role       string          `json:"role"`
		Model      string          `json:"model"`
		ToolCallID string          `json:"toolCallId"`
		Content    json.RawMessage `json:"content"`
		Usage      *struct {
			Cost *struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	} `json:"message"`
}

// foldSessionLine folds one session line into s. pending maps the id of each toolCall with no result yet to its tool's name and
// order lists the ids in call order. Lines that are not a message, or do not decode, change nothing.
func foldSessionLine(s *model.AgentSessionFacts, line []byte, pending map[string]string, order *[]string) {
	if len(line) == 0 || line[0] != '{' {
		return
	}
	var e sessionEntry
	if json.Unmarshal(line, &e) != nil || e.Type != "message" || e.Message == nil {
		return
	}
	m := e.Message
	switch m.Role {
	case "toolResult":
		delete(pending, m.ToolCallID)
	case "assistant":
		if m.Model != "" {
			s.Model = m.Model
		}
		if m.Usage != nil && m.Usage.Cost != nil {
			s.Cost += m.Usage.Cost.Total
			s.HasCost = true
		}
		var parts []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(m.Content, &parts) != nil {
			return
		}
		for _, p := range parts {
			if p.Type == "toolCall" {
				s.ToolCalls++
				pending[p.ID] = p.Name
				*order = append(*order, p.ID)
			}
		}
	}
}
