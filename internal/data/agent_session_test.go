package data

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func assistantLine(modelName string, cost float64, tools ...string) string {
	var parts []string
	for _, t := range tools {
		parts = append(parts, fmt.Sprintf(`{"type":"toolCall","id":%q,"name":%q,"arguments":{}}`, "id-"+t, strings.SplitN(t, ":", 2)[0]))
	}
	return fmt.Sprintf(`{"type":"message","id":"x","message":{"role":"assistant","model":%q,"content":[{"type":"thinking","thinking":"hm"}%s],"usage":{"input":1,"cost":{"total":%v}}}}`,
		modelName, prefixed(parts), cost)
}

func prefixed(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return "," + strings.Join(parts, ",")
}

func resultLine(tool string) string {
	return fmt.Sprintf(`{"type":"message","id":"y","message":{"role":"toolResult","toolCallId":%q,"content":[{"type":"text","text":"ok"}]}}`, "id-"+tool)
}

const userLine = `{"type":"message","id":"u","message":{"role":"user","content":"hello"}}`

func parse(t *testing.T, text string, limit int64) model.AgentSessionFacts {
	t.Helper()
	s, err := ParseSessionTail(bytes.NewReader([]byte(text)), int64(len(text)), limit)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionTailOfAWorkingSessionNamesTheToolWithNoResult(t *testing.T) {
	text := strings.Join([]string{
		`{"type":"session","id":"s"}`, userLine,
		assistantLine("claude-a", 0.25, "read", "grep"), resultLine("read"), resultLine("grep"),
		assistantLine("claude-b", 0.5, "bash"),
		`not json`,
	}, "\n") + "\n"
	got := parse(t, text, 1<<20)
	want := model.AgentSessionFacts{Model: "claude-b", ToolCalls: 3, CurrentTool: "bash", Cost: 0.75, HasCost: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSessionTailOfAnIdleSessionHasNoCurrentTool(t *testing.T) {
	text := strings.Join([]string{userLine, assistantLine("m", 0.1, "bash"), resultLine("bash"), assistantLine("m", 0.1)}, "\n")
	got := parse(t, text, 1<<20)
	want := model.AgentSessionFacts{Model: "m", ToolCalls: 1, Cost: 0.2, HasCost: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSessionTailWithoutUsageHasNoCost(t *testing.T) {
	text := `{"type":"message","message":{"role":"assistant","model":"m","content":[]}}` + "\n"
	if got := parse(t, text, 1<<20); got.HasCost || got.Model != "m" {
		t.Errorf("got %+v", got)
	}
}

func TestSessionTailBiggerThanTheBoundReadsOnlyTheEndAndSaysSo(t *testing.T) {
	var lines []string
	for i := range 200 {
		lines = append(lines, assistantLine("old", 1, fmt.Sprintf("t%d", i)), resultLine(fmt.Sprintf("t%d", i)))
	}
	lines = append(lines, assistantLine("new", 0.5, "bash"))
	text := strings.Join(lines, "\n") + "\n"
	limit := int64(len(lines[len(lines)-1])+len(lines[len(lines)-2])+len(lines[len(lines)-3])) + 10
	got := parse(t, text, limit)
	if !got.Partial || got.Model != "new" || got.CurrentTool != "bash" {
		t.Fatalf("got %+v", got)
	}
	if got.ToolCalls < 1 || got.ToolCalls > 3 || got.Cost > 3 {
		t.Errorf("counted past the bound: %+v", got)
	}
}

func TestSessionTailCutInsideTheOnlyLineHoldsNothing(t *testing.T) {
	text := assistantLine("m", 1, "bash")
	got := parse(t, text, 10)
	if !got.Partial || got.ToolCalls != 0 || got.Model != "" {
		t.Errorf("got %+v", got)
	}
}

func TestReadAgentSessionExplainsWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "prompt-1.md")
	for name, tt := range map[string]struct {
		ask  model.AgentSessionAsk
		path string
		want string
	}{
		"not omp":           {ask: model.AgentSessionAsk{PromptFile: prompt}, want: agentSessionNotOmp},
		"no path file":      {ask: model.AgentSessionAsk{PromptFile: prompt, Omp: true}, want: agentSessionNoPath},
		"no prompt file":    {ask: model.AgentSessionAsk{Omp: true}, want: agentSessionNoPath},
		"a missing session": {ask: model.AgentSessionAsk{PromptFile: prompt, Omp: true}, path: filepath.Join(dir, "gone.jsonl"), want: agentSessionNoFile},
	} {
		_ = os.Remove(prompt + AgentSessionSuffix)
		if tt.path != "" {
			if err := os.WriteFile(prompt+AgentSessionSuffix, []byte(tt.path+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got := readAgentSession(tt.ask); got.Reason != tt.want {
			t.Errorf("%s: reason %q, want %q", name, got.Reason, tt.want)
		}
	}
}

func TestReadAgentSessionReadsTheFileThePathFileNames(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "prompt-1.md")
	session := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(session, []byte(assistantLine("m", 0.3, "bash")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt+AgentSessionSuffix, []byte(session+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readAgentSession(model.AgentSessionAsk{PromptFile: prompt, Omp: true})
	if got.Reason != "" || got.Model != "m" || got.CurrentTool != "bash" || got.ToolCalls != 1 {
		t.Errorf("got %+v", got)
	}
	removePrompt(prompt)
	if _, err := os.Stat(prompt + AgentSessionSuffix); !os.IsNotExist(err) {
		t.Errorf("removePrompt left the session path file: %v", err)
	}
}
