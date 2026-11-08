package model

import "strings"

// Metadata keys kata-plugin and the agent prompt write on an Issue an agent stopped on.
const (
	WorkAttentionKey    = "work.attention"
	WorkAttentionMsgKey = "work.attention_msg"

	WorkAttentionNeedsHuman = "needs-human"
)

// WorkAttention is what an Issue's `work.attention` metadata asks of its reader: "needs-human" or "stuck", with the agent's
// `work.attention_msg`. It is kata's coordination signal, not a Lane label, and whoever wrote it shows the same.
type WorkAttention struct {
	Stuck bool
	// Msg is the message with its whitespace folded, "" when none was written.
	Msg string
}

func (w WorkAttention) Word() string {
	if w.Stuck {
		return "stuck"
	}
	return "needs human"
}

// IssueWorkAttention reads the Issue's `work.attention`; ok is false for a closed Issue (its leftover signal asks nothing), "ok",
// an absent key, an unknown value or a value that is not text.
func IssueWorkAttention(issue Issue) (WorkAttention, bool) {
	if IsClosed(issue) {
		return WorkAttention{}, false
	}
	value, _ := issue.Metadata[WorkAttentionKey].(string)
	var w WorkAttention
	switch value {
	case WorkAttentionNeedsHuman:
	case "stuck":
		w.Stuck = true
	default:
		return WorkAttention{}, false
	}
	msg, _ := issue.Metadata[WorkAttentionMsgKey].(string)
	w.Msg = strings.Join(strings.Fields(msg), " ")
	return w, true
}
