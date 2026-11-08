package model

import "testing"

func TestIssueWorkAttentionIsNothingOnAClosedIssue(t *testing.T) {
	issue := Issue{Metadata: map[string]any{WorkAttentionKey: "stuck", WorkAttentionMsgKey: "tests hang"}}
	if w, ok := IssueWorkAttention(issue); !ok || w.Word() != "stuck" || w.Msg != "tests hang" {
		t.Fatalf("open Issue: %+v, %v", w, ok)
	}
	issue.Status = "closed"
	if w, ok := IssueWorkAttention(issue); ok {
		t.Errorf("closed Issue shows %+v", w)
	}
}
