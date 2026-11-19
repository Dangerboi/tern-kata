package model

import "slices"

// The choice sets of the form cards whose field is a choice, not typed text: each reads the held value from the card's state
// and says how the card draws it. popup_card.go registers them as the cards' choices.

// DraftReplyKind is the Draft editor's choice of what a Reply says, a field of no table: the editor is one body, so the kind has
// no focus of its own and the key that cycles it stays CycleReplyKind.
const DraftReplyKind Field = "reply-kind"

// priorityValues are the values a Create card's Priority holds, in the order its choices are shown: none, then P0 to P4.
var priorityValues = []string{"", "0", "1", "2", "3", "4"}

// The Priority picker is a segmented row, not a select: six short words fit in a row, and a select's menu is drawn inside the
// card body, which clips it to the few options its height allows.
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
	return newChoices(ControlTabs, priorityLabels, priorityIndex(m.Create.Priority)), true
}

// FieldPriority is the Priority choice of the Set priority card, the same picker as the Create card's.
const FieldPriority Field = "priority"

// fieldChoiceSet is the choice of the single-field card: the Priority picker of Set priority, none for every other form.
func fieldChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if !m.FieldForm.Open || m.FieldForm.Action != PriorityAction || name != FieldPriority {
		return ChoiceSet{}, false
	}
	return newChoices(ControlTabs, priorityLabels, priorityIndex(m.FieldForm.Value)), true
}

// chooseFieldPriority holds the priority at index of the picker; none is kata's "-".
func chooseFieldPriority(m Model, index int) Model {
	if index < 0 || index >= len(priorityValues) {
		return m
	}
	m.FieldForm.Value = cmpOr(priorityValues[index], "-")
	m.FieldForm.Err = nil
	return m
}

// stepFieldChoice moves the Set priority picker to its next or previous choice.
func stepFieldChoice(m Model, back bool) Model {
	return chooseFieldPriority(m, cycleIndex(max(priorityIndex(m.FieldForm.Value), 0), len(priorityValues), back))
}

func cmpOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// closeReasons are the reasons a close takes, in the order the Reason control shows them.
var closeReasons = []CloseReasonKind{ReasonDone, ReasonWontfix, ReasonDuplicate, ReasonSuperseded, ReasonAuditNoChange}

// CloseReasonLabel is the word the Reason control shows for a reason: kata's code is never shown, and a code from a newer kata
// is its own word.
func CloseReasonLabel(reason string) string { return CloseReasonWords(reason) }

// closeReasonTabsCols is the narrowest Pane that fits the five reasons in one segmented row; below it the Reason is a select.
const closeReasonTabsCols = 56

// closeChoiceSet is the Reason control: segmented tabs while the five reasons fit a row, a select below that.
func closeChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != CloseReason {
		return ChoiceSet{}, false
	}
	labels := make([]string, len(closeReasons))
	for i, reason := range closeReasons {
		labels[i] = CloseReasonLabel(string(reason))
	}
	control := ControlTabs
	if m.NarrowerThan(closeReasonTabsCols) {
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
