package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func attentionIssue(uid string, meta map[string]any) model.Issue {
	issue := agentIssue(uid)
	issue.Metadata = meta
	return issue
}

// attentionSpan is the text and style of the span of a row's label that names the work attention.
func attentionSpan(t *testing.T, issue model.Issue, pane string) (text, style string) {
	t.Helper()
	s := agentPanes(t, issue)[pane]
	row := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.IssueListID, issue.UID))
	spans, _ := row.P["label"].([]any)
	for _, sp := range spans {
		span, _ := sp.(map[string]any)
		got, _ := span["t"].(string)
		if strings.Contains(got, "needs human") || strings.Contains(got, "stuck") {
			style, _ := span["s"].(string)
			return strings.TrimSpace(got), style
		}
	}
	return "", ""
}

func TestTheWorkAttentionShowsWithItsMessageOnListAndLaneRows(t *testing.T) {
	for _, pane := range []string{"list", "lane"} {
		for _, c := range []struct {
			value, text, style string
		}{
			{"needs-human", "needs human: the API key is missing", "warning"},
			{"stuck", "stuck: the API key is missing", "error"},
		} {
			issue := attentionIssue("t1", map[string]any{"work.attention": c.value, "work.attention_msg": "the API key is missing"})
			if text, style := attentionSpan(t, issue, pane); text != c.text || style != c.style {
				t.Errorf("%s %s: %q in %q, want %q in %q", pane, c.value, text, style, c.text, c.style)
			}
		}
		for name, meta := range map[string]map[string]any{
			"ok":      {"work.attention": "ok", "work.attention_msg": "fine"},
			"absent":  {"work.attention_msg": "orphan"},
			"unknown": {"work.attention": "bored", "work.attention_msg": "x"},
			"number":  {"work.attention": 3},
			"none":    nil,
		} {
			if text, _ := attentionSpan(t, attentionIssue("t1", meta), pane); text != "" {
				t.Errorf("%s %s: shows %q", pane, name, text)
			}
		}
	}
}

func TestTheWorkAttentionWithoutAMessageShowsTheWordAlone(t *testing.T) {
	issue := attentionIssue("t1", map[string]any{"work.attention": "needs-human"})
	if text, _ := attentionSpan(t, issue, "list"); text != "needs human" {
		t.Errorf("text %q", text)
	}
}

func TestTheBoardCardShowsTheWorkAttention(t *testing.T) {
	issue := attentionIssue("t1", map[string]any{"work.attention": "stuck", "work.attention_msg": "tests hang"})
	s := agentPanes(t, issue)["board"]
	nodes := wireNodes(t, view.Render(s.Model, now))
	badge := node(t, nodes, rowOf(t, s.Model, view.BoardID, "t1")+".attention")
	if badge.P["text"] != "stuck: tests hang" || badge.P["tone"] != "error" {
		t.Errorf("badge %v", badge.P)
	}
	ok := agentPanes(t, attentionIssue("t1", map[string]any{"work.attention": "ok"}))["board"]
	for _, n := range wireNodes(t, view.Render(ok.Model, now)) {
		if n.ID == rowOf(t, ok.Model, view.BoardID, "t1")+".attention" {
			t.Error("ok shows an attention badge")
		}
	}
}

func TestTheIssueDetailHasAWorkAttentionPropertyWithTheWholeMessage(t *testing.T) {
	long := strings.Repeat("word ", 30)
	issue := attentionIssue("t1", map[string]any{"work.attention": "needs-human", "work.attention_msg": long})
	s := step(t, projectPane(t, 160, issue), model.DetailLoaded{UID: "t1", Selected: &model.IssueDetailData{Issue: &issue}}).State
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasSuffix(n.ID, ".properties.work-attention.v") {
			if n.P["text"] != "needs human: "+strings.TrimSpace(long) || n.P["tone"] != "warning" {
				t.Errorf("badge %v", n.P)
			}
			return
		}
	}
	t.Fatal("the detail has no Work attention property")
}

func TestAClosedIssuesLeftoverWorkAttentionShowsNotOnTheDetail(t *testing.T) {
	issue := attentionIssue("t1", map[string]any{"work.attention": "stuck", "work.attention_msg": "tests hang"})
	issue.Status = "closed"
	s := step(t, projectPane(t, 160, issue), model.DetailLoaded{UID: "t1", Selected: &model.IssueDetailData{Issue: &issue}}).State
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.Contains(n.ID, ".properties.work-attention") {
			t.Errorf("detail: a closed Issue has the property %s", n.ID)
		}
	}
}
