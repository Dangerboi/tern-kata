package data

import (
	"fmt"
	"strings"
	"unicode"
)

// AgentPrompt is the first message of an interactive agent on an Issue: which Issue, where to read it, and what it leaves in
// kata. The body, comments and attachments are not in it, so no Issue text but the title passes through a file and none through
// a shell; the agent runs `kata show`.
func AgentPrompt(ref, title string) string {
	title = strings.Join(strings.FieldsFunc(title, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	var b strings.Builder
	fmt.Fprintf(&b, "# Your Issue\n\nIssue: %s - %s\nRun `kata show %s` for its body, comments and attachments before you start.\n\n", ref, title, ref)
	b.WriteString("# Contract\n\n")
	fmt.Fprintf(&b, "- Claim it first: `kata claim %s --ttl 24h`. If another actor owns the Issue, stop and ask the user; don't take it.\n", ref)
	fmt.Fprintf(&b, "- Comment what you did and how you checked it: `kata comment %s -m \"<what you did and how you verified it>\"`.\n", ref)
	fmt.Fprintf(&b, "- Only when the work is done and verified, close the Issue with evidence: `kata close %s --done -m \"<what was done and how it was verified>\"` plus `--commit <sha>`, `--pr <url>`, `--test \"<command>\"` or `--reviewed <path>`. Otherwise leave it open. Never pad or invent evidence.\n", ref)
	b.WriteString("- When you need a decision, ask the user and wait for the answer.\n")
	fmt.Fprintf(&b, "- If you stop without closing the Issue, comment why, then hand it to a human: `kata label rm %s ready-for-agent` (when it has that label) and `kata label add %s ready-for-human`.\n", ref, ref)
	return b.String()
}
