package model

import "slices"

// The choice sets of the form cards whose field is a choice, not typed text: each reads the held value from the card's state
// and says how the card draws it. popup_card.go registers them as the cards' choices.

// DraftReplyKind is the Draft editor's choice of what a Reply says, a field of no table: the editor is one body, so the kind has
// no focus of its own and the key that cycles it stays CycleReplyKind.
const DraftReplyKind Field = "reply-kind"

// priorityValues are the values a Create card's Priority holds, in the order its choices are shown: none, then P0 to P4.
var priorityValues = []string{"", "0", "1", "2", "3", "4"}

var priorityLabels = []string{"None", "P0", "P1", "P2", "P3", "P4"}

// priorityIndex is the index of a held priority among priorityValues; a typed "-" (kata's word for none) is none.
func priorityIndex(value string) int {
	if value == "-" {
		return 0
	}
	return slices.Index(priorityValues, value)
}

func createChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != CreatePriority {
		return ChoiceSet{}, false
	}
	return newChoices(ControlSelect, priorityLabels, priorityIndex(m.Create.Priority)), true
}

// closeReasons are the reasons a close takes, in the order the Reason control shows them.
var closeReasons = []CloseReasonKind{ReasonDone, ReasonWontfix, ReasonDuplicate, ReasonSuperseded, ReasonAuditNoChange}

// CloseReasonLabel is the word the Reason control shows for a reason: kata's code is never shown, and a code from a newer kata
// is its own word.
func CloseReasonLabel(reason string) string { return CloseReasonWords(reason) }

// closeChoiceSet is the Reason control: segmented tabs while the card is wide, a select once it is narrower than the folded
// legend.
func closeChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != CloseReason {
		return ChoiceSet{}, false
	}
	labels := make([]string, len(closeReasons))
	for i, reason := range closeReasons {
		labels[i] = CloseReasonLabel(string(reason))
	}
	control := ControlTabs
	if m.NarrowerThan(FoldedLegendCols) {
		control = ControlSelect
	}
	return newChoices(control, labels, slices.Index(closeReasons, CloseReasonKind(m.CloseForm.Values.Reason))), true
}

// tokenScopes are the scopes a token takes, in the order the Scope control shows them.
var tokenScopes = []string{TokenScopeAccount, TokenScopeIssue}

func tokenChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != TokenScope {
		return ChoiceSet{}, false
	}
	return newChoices(ControlTabs, []string{"Account", "Issue"}, slices.Index(tokenScopes, m.TokenForm.Values.Scope)), true
}

// replyKindLabels are the words of the four ReplyKinds, in the order of ReplyKinds.
var replyKindLabels = []string{"Reply", "Confirm", "Refute", "Supersede"}

func draftChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	draft, ok := m.ActiveDraft()
	if name != DraftReplyKind || !ok || draft.ReplyTo == "" {
		return ChoiceSet{}, false
	}
	return newChoices(ControlTabs, replyKindLabels, slices.Index(ReplyKinds, draft.ReplyKind)), true
}

// chooseReplyKind gives the Draft the kind at index of ReplyKinds, the way the pointer picks a tab.
func chooseReplyKind(m Model, index int) Model {
	draft, ok := m.ActiveDraft()
	if !ok || draft.ReplyTo == "" || index < 0 || index >= len(ReplyKinds) {
		return m
	}
	draft.ReplyKind, draft.Err = ReplyKinds[index], nil
	return putDraft(m, m.EditorUID, draft)
}
