package model

import "strconv"

// HoldWords is the sentence-case words of a hold or pause reason code. A code a newer kata sends comes back as it is.
func HoldWords(code string) string {
	switch code {
	case "agent_quota":
		return "Quota reached"
	case "agent_login":
		return "Login required"
	case "interrupted":
		return "Interrupted"
	case "launch_unproven":
		return "Launch not confirmed"
	case "no_receipt":
		return "No receipt from the agent"
	case "group_unproven":
		return "Process group not confirmed"
	case "invalid_prompt":
		return "Invalid prompt"
	case "execution_error":
		return "Execution error"
	case "admission_rejected":
		return "Admission rejected"
	case "create_rejected":
		return "Create rejected"
	case "secret_unavailable":
		return "Secret unavailable"
	}
	return code
}

// CloseReasonWords is the words of an Issue's close reason code; an unknown code comes back as it is.
func CloseReasonWords(code string) string {
	switch code {
	case "done":
		return "Done"
	case "wontfix":
		return "Won't fix"
	case "duplicate":
		return "Duplicate"
	case "superseded":
		return "Superseded"
	case "audit-no-change":
		return "No change"
	}
	return code
}

// RelationWords is the lowercase words of a link relation, which follows a muted lead; an unknown code comes back as it is.
func RelationWords(code string) string {
	switch code {
	case "blocks":
		return "blocks"
	case "blocked_by":
		return "blocked by"
	case "parent":
		return "parent"
	case "child":
		return "child"
	case "related":
		return "related"
	}
	return code
}

// Noun is the noun alone for n: one when n is 1, else many, which defaults to one+"s".
func Noun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	if many == "" {
		return one + "s"
	}
	return many
}

// Count is n with its noun: "1 daemon", "3 daemons", "0 daemons".
func Count(n int, one, many string) string { return strconv.Itoa(n) + " " + Noun(n, one, many) }

// RefName names an Issue the way the Pane's Collection sees it: "#short" inside the Project Collection of its own
// Project, "project#short" for an Issue of another Project and in Collections that span Projects.
func (m Model) RefName(projectName, shortID string) string {
	if projectName == "" || m.Collection.IsProject() && m.Collection.Title == projectName {
		return "#" + shortID
	}
	return projectName + "#" + shortID
}
