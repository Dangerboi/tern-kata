package cronjob

import (
	"slices"
	"strings"
)

// reasonPrefix starts the first line of a Run summary that carries a machine-readable cause, by kata's convention (kata's cron
// reference, "Independent run evidence"): `reason: <code>`, with the detail on the lines after. Any status may carry one; kata
// stores it as text and checks nothing.
const reasonPrefix = "reason: "

// legacyHoldCodes are the hold reasons tern-kata's worker wrote as "<code>: <detail>" before it followed kata's convention.
// ReadReason still reads that shape for them, so earlier runs (and journals) keep their reason. The list never grows: a new
// code is only ever written in the convention's shape.
var legacyHoldCodes = []string{
	"admission_rejected", "create_rejected", "secret_unavailable", "launch_unproven", "interrupted", "no_receipt",
	"group_unproven", "invalid_prompt", "execution_error", "agent_login", "agent_quota",
}

func ReasonMessage(code, detail string) string {
	if detail == "" {
		return reasonPrefix + code
	}
	return reasonPrefix + code + "\n" + detail
}

// ReadReason splits a summary message into its reason code and its detail. It reads kata's convention and the
// "<code>: <detail>" shape tern-kata's worker wrote before it; any other message has no code and is all detail.
func ReadReason(message string) (code, detail string) {
	first, rest, _ := strings.Cut(message, "\n")
	if c, ok := strings.CutPrefix(strings.TrimRight(first, " \r"), reasonPrefix); ok && validReasonCode(c) {
		return c, rest
	}
	if c, d, ok := strings.Cut(message, ": "); ok && slices.Contains(legacyHoldCodes, c) {
		return c, d
	}
	return "", message
}

func (r Run) Reason() (code, detail string) { return ReadReason(r.Summary) }

// validReasonCode is the convention's code grammar: [a-z0-9_.-]{1,64}.
func validReasonCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, c := range []byte(code) {
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}
