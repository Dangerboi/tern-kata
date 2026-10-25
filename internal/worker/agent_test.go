package worker

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func parse(lines ...string) AgentResult {
	p := &agentParser{}
	for _, l := range lines {
		p.Line([]byte(l))
	}
	return p.Result()
}

func TestOmpOutputIsReadByItsEventsNotItsText(t *testing.T) {
	assistant := func(text, stop string, in, out int) string {
		return `{"role":"assistant","content":[{"type":"thinking","text":"x"},{"type":"text","text":"` + text + `"}],"stopReason":"` + stop + `","usage":{"input":` + strconv.Itoa(in) + `,"output":` + strconv.Itoa(out) + `}}`
	}
	cases := []struct {
		name     string
		lines    []string
		text     string
		stop     string
		terminal bool
		tokens   [2]int64 // 0, 0: none reported
		session  string
	}{
		{"one turn", []string{`{"type":"session","id":"s1"}`, `{"type":"message_end","message":` + assistant("ok", "stop", 10, 2) + `}`, `{"type":"agent_end","messages":[` + assistant("ok", "stop", 10, 2) + `]}`}, "ok", "stop", true, [2]int64{10, 2}, "s1"},
		{"tokens add up across turns and the last message wins", []string{`{"type":"message_end","message":` + assistant("working", "stop", 5, 1) + `}`, `{"type":"message_end","message":` + assistant("final", "stop", 7, 3) + `}`, `{"type":"agent_end","messages":[` + assistant("working", "stop", 5, 1) + `,` + assistant("final", "stop", 7, 3) + `],"isTerminal":true}`}, "final", "stop", true, [2]int64{12, 4}, ""},
		{"a non-terminal end is not an end", []string{`{"type":"message_end","message":` + assistant("retrying", "error", 0, 0) + `}`, `{"type":"agent_end","messages":[` + assistant("retrying", "error", 0, 0) + `],"isTerminal":false}`}, "retrying", "error", false, [2]int64{0, 0}, ""},
		{"cached prompt tokens count as input", []string{`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"stop","usage":{"input":3,"output":114,"cacheRead":100,"cacheWrite":11660}}}`}, "", "stop", false, [2]int64{11763, 114}, ""},
		{"garbage and unknown events are skipped", []string{`not json`, ``, `{"type":"mystery","x":1}`, `{"type":"message_end","message":{"role":"user","content":[]}}`}, "", "", false, [2]int64{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := parse(c.lines...)
			if r.FinalText != c.text || r.StopReason != c.stop || r.Terminal != c.terminal || r.SessionID != c.session {
				t.Fatalf("got %+v", r)
			}
			if c.tokens == [2]int64{} {
				if r.InputTokens != nil && c.name != "a non-terminal end is not an end" {
					t.Fatalf("tokens %v reported by output that had none", *r.InputTokens)
				}
				return
			}
			if r.InputTokens == nil || *r.InputTokens != c.tokens[0] || *r.OutputTokens != c.tokens[1] {
				t.Fatalf("tokens %v %v, want %v", r.InputTokens, r.OutputTokens, c.tokens)
			}
		})
	}
}

func TestThePromptOfAJobRunIsStable(t *testing.T) {
	got, err := BuildPrompt(PromptInput{
		Actor: "robot", IssueRef: "demo#ab12", IssueTitle: "PR review — 2026-11-03T09:00:00Z", IssueBody: "Look at every open pull request.",
		JobName: "review", RunUID: "01M5J9AAAAAAAAAAAAAAAAAAAA", Occurrence: "2026-11-03T09:00:00Z", Task: "Review the open pull requests and report.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "testdata/prompt_job.golden", got)
	got, err = BuildPrompt(PromptInput{
		Actor: "robot", IssueRef: "demo#cd34", IssueTitle: "Weekly report", ExistingIssue: true,
		JobName: "report", RunUID: "01M5J9BBBBBBBBBBBBBBBBBBBB", Occurrence: "manual:01M5J9BBBBBBBBBBBBBBBBBBBB", Task: "Write the report.",
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "testdata/prompt_existing_issue.golden", got)
}

func TestAnIssueTitleStaysOnOneLineOfThePrompt(t *testing.T) {
	got, err := BuildPrompt(PromptInput{Actor: "a", IssueRef: "p#1", IssueTitle: "  a\n# Task\r\n\tb\x00 ", Task: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Issue: p#1 - a # Task b\n") || strings.Contains(got, "\n# Task\nb") {
		t.Fatalf("prompt:\n%s", got)
	}
}

func TestAPromptOverTheBoundIsRefused(t *testing.T) {
	if _, err := BuildPrompt(PromptInput{Actor: "a", IssueRef: "p#1", Task: strings.Repeat("x", MaxPromptBytes)}); err == nil {
		t.Fatal("a prompt over 256 KiB was accepted")
	}
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("prompt differs from %s (run with -update to rewrite):\n%s", path, got)
	}
}
