package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/data/ompext"
)

// AgentResult is what omp's `--mode json` output says about a run (ADR-0009). omp prints one JSON object per line;
// the worker reads the first `session` event (the session id), each assistant `message_end` (usage, summed) and the
// last terminal `agent_end` (the final assistant message). Unknown events and keys are ignored, garbage lines skipped.
type AgentResult struct {
	SessionID string `json:"session_id,omitempty"`
	// FinalText is the text of the last assistant message of the last terminal agent_end, which is the run's summary.
	FinalText string `json:"final_text,omitempty"`
	// StopReason is that message's stopReason: "stop" for a finished turn; "error" and "aborted" otherwise.
	StopReason string `json:"stop_reason,omitempty"`
	ErrorText  string `json:"error_text,omitempty"`
	// ErrorStatus is that message's errorStatus (the provider's HTTP status), 0 when it had none.
	ErrorStatus int `json:"error_status,omitempty"`
	// Terminal is true once an agent_end with isTerminal absent or true arrived. No agent_end is a failure whatever the exit code.
	Terminal     bool   `json:"terminal,omitempty"`
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	// CostUSD is the sum of the cost omp reported (usage.cost.total) for the attempt; nil when it reported none.
	CostUSD    *float64 `json:"cost_usd,omitempty"`
	StderrTail string   `json:"stderr_tail,omitempty"`
}

type agentParser struct {
	res       AgentResult
	in, out   int64
	sawUsage  bool
	cost      float64
	sawCost   bool
	onSession func(id string)
}

type ompMessage struct {
	Role         string `json:"role"`
	StopReason   string `json:"stopReason"`
	ErrorMessage string `json:"errorMessage"`
	ErrorStatus  int    `json:"errorStatus"`
	// Content stays undecoded: only the message that decides the result has its text read.
	Content contentView `json:"content"`
	Usage   *struct {
		Input      int64 `json:"input"`
		Output     int64 `json:"output"`
		CacheRead  int64 `json:"cacheRead"`
		CacheWrite int64 `json:"cacheWrite"`
		Cost       *struct {
			Total *float64 `json:"total"`
		} `json:"cost"`
	} `json:"usage"`
}

// contentView is a message's content as it lies in the line being parsed, not copied, so it is valid only until
// the parser's Line returns.
type contentView []byte

func (c *contentView) UnmarshalJSON(b []byte) error {
	*c = b
	return nil
}

// text is the message's text parts joined; ok is false when the content is not an array of parts (an assistant
// message that does not decode).
func (m ompMessage) text() (text string, ok bool) {
	if len(m.Content) == 0 {
		return "", true
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return "", false
	}
	var b strings.Builder
	for _, c := range parts {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String(), true
}

func (p *agentParser) Line(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return
	}
	// Messages stay raw until the role is known: omp's other roles (a `custom` message's content is a string) need not
	// fit ompMessage, and one of them must not cost the whole agent_end.
	var event struct {
		Type       string            `json:"type"`
		ID         string            `json:"id"`
		Message    json.RawMessage   `json:"message"`
		Messages   []json.RawMessage `json:"messages"`
		IsTerminal *bool             `json:"isTerminal"`
	}
	if json.Unmarshal(line, &event) != nil {
		return
	}
	switch event.Type {
	case "session":
		p.session(event.ID)
	case "message_end":
		if m, ok := assistantMessage(event.Message); ok {
			p.messageEnd(m)
		}
	case "agent_end":
		if !finishesTurn(event.IsTerminal) {
			return
		}
		p.res.Terminal = true
		for i := len(event.Messages) - 1; i >= 0; i-- {
			var head struct {
				Role string `json:"role"`
			}
			if json.Unmarshal(event.Messages[i], &head) != nil || head.Role != "assistant" {
				continue
			}
			m, _ := assistantMessage(event.Messages[i])
			p.agentEnd(m)
			break
		}
	}
}

// finishesTurn is ADR-0009's rule for an agent_end: it ends the run unless its isTerminal is present and false (omp
// goes on). endsRetry reads the same field from the end of an agent_end line too long to parse; the two must agree.
func finishesTurn(isTerminal *bool) bool { return isTerminal == nil || *isTerminal }

func (p *agentParser) session(id string) {
	if p.res.SessionID == "" && id != "" {
		p.res.SessionID = id
		if p.onSession != nil {
			p.onSession(id)
		}
	}
}

func (p *agentParser) messageEnd(m ompMessage) {
	if m.Role != "assistant" {
		return
	}
	text, ok := m.text()
	if !ok {
		return
	}
	if u := m.Usage; u != nil {
		p.sawUsage = true
		p.in += u.Input + u.CacheRead + u.CacheWrite // omp's `input` leaves cached prompt tokens out
		p.out += u.Output
		if u.Cost != nil && u.Cost.Total != nil {
			p.sawCost = true
			p.cost += *u.Cost.Total
		}
	}
	p.final(m, text)
}

// agentEnd takes the last assistant turn of an agent_end as the run's result. That turn decides the run even when
// its content does not decode: falling back to an earlier turn's stop would pass a failed run, so the zero message
// clears what message_end recorded.
func (p *agentParser) agentEnd(m ompMessage) {
	text, ok := m.text()
	if !ok {
		m, text = ompMessage{}, ""
	}
	p.final(m, text)
}

// assistantMessage decodes raw as an assistant message; any other role, or a shape that does not decode, is not one
// (the zero message comes back with false).
func assistantMessage(raw json.RawMessage) (ompMessage, bool) {
	var m ompMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m.Role != "assistant" {
		return ompMessage{}, false
	}
	return m, true
}

func (p *agentParser) final(m ompMessage, text string) {
	// The run folder's output file keeps the whole message; the result is journalled, so it stays under the summary bound.
	p.res.FinalText = truncate(text, maxSummaryBytes)
	p.res.StopReason = m.StopReason
	p.res.ErrorText = truncate(m.ErrorMessage, maxSummaryBytes)
	p.res.ErrorStatus = m.ErrorStatus
}

// Result is what the output said so far; tokens and cost stay nil when omp reported none.
func (p *agentParser) Result() AgentResult {
	r := p.res
	if p.sawUsage {
		in, out := p.in, p.out
		r.InputTokens, r.OutputTokens = &in, &out
	}
	if p.sawCost {
		cost := p.cost
		r.CostUSD = &cost
	}
	return r
}

func sessionFolder(runDir string) string { return filepath.Join(runDir, "session") }

// args is the omp command line for a run (ADR-0009): headless JSON, the run's session folder, the profile's model
// and thinking level, its extensions flags, an unattended approval mode unless the profile sets its own, the profile's extra arguments,
// tern-kata's omp extension when extension names it, and the prompt as an @file (one positional is one turn; the file keeps the
// prompt out of argv's size limit).
// Checked against omp 18.12: -p --mode json need no TTY, stdin must be /dev/null, and without an approval mode a
// headless run that needs approval has no one to ask, so the default is `--approval-mode yolo`.
func agentArgs(p Profile, runDir, promptFile, resume, extension string) []string {
	args := []string{"-p", "--mode", "json", "--session-dir", sessionFolder(runDir)}
	if resume != "" {
		args = append(args, "--resume", resume)
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Thinking != "" {
		args = append(args, "--thinking", p.Thinking)
	}
	if !slices.ContainsFunc(p.Args, func(a string) bool {
		return a == "--approval-mode" || strings.HasPrefix(a, "--approval-mode=") || a == "--auto-approve"
	}) {
		args = append(args, "--approval-mode", "yolo")
	}
	args = append(args, p.Extensions.args()...)
	args = append(args, p.Args...)
	if extension != "" {
		args = append(args, ompext.Argv(extension, p.Args, p.Extensions.Mode == extensionsList)...)
	}
	return append(args, "@"+promptFile)
}

// MaxPromptBytes bounds the expanded prompt (ADR-0009).
const MaxPromptBytes = 256 << 10

type PromptInput struct {
	Actor         string
	IssueRef      string // qualified: <project>#<short id>
	IssueTitle    string
	IssueBody     string
	ExistingIssue bool
	JobName       string
	RunUID        string
	Occurrence    string
	Task          string
	// Attempt is the step's attempt, 1 for the first. Resumed is set from the second on: the previous attempt's
	// turn may have partly run, and the new prompt says so.
	Attempt    int
	NewCommits string
	Resumed    *ResumeNote
	// Step is set for a workflow's prompt step; a prompt job's single step needs none.
	Step *StepInput
}

// StepInput is where a prompt step sits in its workflow, and why it runs again when a failed attempt came before it.
type StepInput struct {
	Workflow string
	About    string
	Key      string
	Number   int // 1-based
	Count    int
	Failure  string
}

type ResumeNote struct {
	Hold   string // the hold reason that was lifted
	Detail string // the hold's summary after its reason
}

// BuildPrompt is the prompt of a prompt-job run, in ADR-0009's order: the run's Issue, the job and run identity, the
// task text, then the contract. It is tested as text (testdata/prompt_job.golden).
func BuildPrompt(in PromptInput) (string, error) {
	var b strings.Builder
	b.WriteString("# This run\n\n")
	fmt.Fprintf(&b, "Issue: %s", in.IssueRef)
	// A title can hold newlines that would fake a prompt section, so it goes on the line as one run of words.
	if title := strings.Join(strings.FieldsFunc(in.IssueTitle, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " "); title != "" {
		fmt.Fprintf(&b, " - %s", title)
	}
	b.WriteString("\n")
	if in.ExistingIssue {
		b.WriteString("This is an existing Issue that the job reports to.\n")
	}
	fmt.Fprintf(&b, "Job: %s\nRun: %s\nOccurrence: %s\n", in.JobName, in.RunUID, in.Occurrence)
	if st := in.Step; st != nil {
		fmt.Fprintf(&b, "Workflow: %s\n", st.Workflow)
		if about := strings.TrimSpace(st.About); about != "" {
			fmt.Fprintf(&b, "About: %s\n", truncate(about, 2000))
		}
		fmt.Fprintf(&b, "Step: %d of %d (%s), attempt %d\n", st.Number, st.Count, st.Key, max(in.Attempt, 1))
	} else {
		fmt.Fprintf(&b, "Step: 1, attempt %d\n", max(in.Attempt, 1))
	}
	if body := strings.TrimSpace(in.IssueBody); body != "" && !in.ExistingIssue {
		fmt.Fprintf(&b, "\nIssue body:\n\n%s\n", body)
	}
	if in.Resumed != nil {
		b.WriteString("\n# Resumed run\n\n")
		fmt.Fprintf(&b, "This run was held (%s) and a person resumed it. Your previous turn in this session was cut short and may have partly run: a command, an edit or a kata write may have happened without you seeing its result. Before redoing anything, check the checkout and this run's Issue (its comments, labels and status) for work already done, and carry on from there rather than starting over. The task and the run contract below still apply in full.\n", in.Resumed.Hold)
		if d := strings.TrimSpace(in.Resumed.Detail); d != "" {
			fmt.Fprintf(&b, "\nWhy it was held: %s\n", truncate(d, 2000))
		}
	}
	if st := in.Step; st != nil && st.Failure != "" {
		b.WriteString("\n# Retry\n\n")
		fmt.Fprintf(&b, "This is attempt %d of this step: the previous attempt failed. Before redoing anything, check the checkout and this run's Issue (its comments, labels and status) for work that attempt left partly done, and carry on from there. Earlier steps of this workflow ran in other processes; what they did is in the checkout and the Issue.\n\nWhy it failed: %s\n", max(in.Attempt, 2), truncate(strings.TrimSpace(st.Failure), 2000))
	}
	if list := strings.TrimSpace(in.NewCommits); list != "" {
		b.WriteString("\n# New commits\n\nThis job runs when its checkout has new commits. These came in since the last run of this job that succeeded, newest first (abbreviated id and subject):\n\n")
		b.WriteString(list)
		b.WriteString("\n")
	}
	b.WriteString("\n# Task\n\n")
	b.WriteString(strings.TrimSpace(in.Task))
	b.WriteString("\n\n# Run contract\n\n")
	fmt.Fprintf(&b, "You are acting as %s. This run's Issue is %s.\n\n", in.Actor, in.IssueRef)
	fmt.Fprintf(&b, "- Comment what you did and how you checked it: `kata comment %s -m \"<what you did and how you verified it>\"`.\n", in.IssueRef)
	if in.ExistingIssue {
		b.WriteString("- This Issue is not yours to close: comment only. Never close, label or edit it.\n")
	} else {
		fmt.Fprintf(&b, "- When the work is done and verified, close the Issue with evidence: `kata close %s --done -m \"<what was done and how it was verified>\"` plus `--commit <sha>`, `--pr <url>`, `--test \"<command>\"` or `--reviewed <path>`. When nothing needed changing, use `kata close %s --audit-no-change -m \"<what you checked>\" --evidence \"no-change-audit:<what was checked>\"`.\n", in.IssueRef, in.IssueRef)
		b.WriteString("- Do not close the Issue when the work is not done or not verified.\n")
	}
	b.WriteString("- If the task failed or needs a person, say so plainly in your final message. Never pad or invent evidence.\n")
	b.WriteString("- Your final message is recorded as this run's summary: make it say what happened.\n")
	b.WriteString("- Other runs may be working in this checkout at the same time: don't switch branches, reset, stash or clean it unless your task says to. If your task needs a tree of its own, make a git worktree.\n")
	out := b.String()
	if len(out) > MaxPromptBytes {
		return "", fmt.Errorf("the prompt is %d bytes, over the %d limit", len(out), MaxPromptBytes)
	}
	if !utf8.ValidString(out) || strings.ContainsRune(out, 0) {
		return "", fmt.Errorf("the prompt is not valid UTF-8 text without NUL bytes")
	}
	return out, nil
}

// maxSummaryBytes is kata's summary bound, leaving room for the reason kept first.
const maxSummaryBytes = 60 << 10

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
