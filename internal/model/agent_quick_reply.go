package model

// Quick replies are three canned messages to an agent, one key each: `r` on the Attention Page or the Agent card opens a picker
// where 1, 2 or 3 sends one. A pick goes through the Message card's own send, so its off-states, limit, stale check and question
// before replacing an unread message are the typed message's.

const (
	// ModeAgentQuickReply is the Message card as the picker of the three quick replies.
	ModeAgentQuickReply Mode = "agent-quick-reply"

	// QuickReplyAgent is `r` on the Attention Page; AgentCardQuickReply is `r` on the Agent card.
	QuickReplyAgent      Action = "quick-reply-agent"
	AgentCardQuickReply  Action = "agent-card-quick-reply"
	AgentQuickReplyOne   Action = "agent-quick-reply-1"
	AgentQuickReplyTwo   Action = "agent-quick-reply-2"
	AgentQuickReplyThree Action = "agent-quick-reply-3"
)

// The texts of the quick replies. QuickReplyWrapUp is also what an end-of-day sweep tells the agents.
const (
	QuickReplyContinue    = "continue"
	QuickReplyWrapUp      = "wrap up: commit what you have and comment your status"
	QuickReplyCommitClose = "commit and close if the acceptance criteria hold"
)

// QuickReply is one canned message: the key that sends it, the picker's short name for it and its text.
type QuickReply struct {
	Key    string
	Name   string
	Text   string
	Action Action
}

// QuickReplies are the picker's three replies, in key order.
var QuickReplies = []QuickReply{
	{Key: "1", Name: "Continue", Text: QuickReplyContinue, Action: AgentQuickReplyOne},
	{Key: "2", Name: "Wrap up", Text: QuickReplyWrapUp, Action: AgentQuickReplyTwo},
	{Key: "3", Name: "Commit and close", Text: QuickReplyCommitClose, Action: AgentQuickReplyThree},
}

// openAgentQuickReply is `r` on the Attention Page: the picker for the agent behind the selected request, or the reason there is
// none in a Notice, as `m` gives.
func openAgentQuickReply(m Model) (Model, []Cmd) {
	return openAgentMessageFor(m, true)
}

// quickReplyOf is the quick reply a picker action sends.
func quickReplyOf(action Action) (QuickReply, bool) {
	for _, q := range QuickReplies {
		if q.Action == action {
			return q, true
		}
	}
	return QuickReply{}, false
}
