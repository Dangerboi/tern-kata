package driver_test

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// drawnPieces are the texts a node and everything under it draws, one per text, span or label.
func drawnPieces(n tern.WireNode) []string {
	var pieces []string
	var collect func(v any)
	collect = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for _, key := range []string{"t", "text", "label"} {
				if s, ok := v[key].(string); ok {
					pieces = append(pieces, strings.TrimSpace(s))
				}
			}
			for key, child := range v {
				if key != "actions" {
					collect(child)
				}
			}
		case []any:
			for _, child := range v {
				collect(child)
			}
		}
	}
	collect(n.P)
	for _, child := range n.C {
		pieces = append(pieces, drawnPieces(child)...)
	}
	return pieces
}

// drawnText is the text a node and everything under it draws.
func drawnText(n tern.WireNode) string { return strings.Join(drawnPieces(n), " ") }

// rowsShowing are the ids of the nodes under list that draw every one of texts and have no child that does: the row of an
// Issue, a card of the Board, a Job, a Run or a Workflow, found by what the view shows of it.
func rowsShowing(t *testing.T, m model.Model, list string, texts ...string) []string {
	t.Helper()
	return rowsMatching(t, m, list, func(n tern.WireNode) bool {
		text := drawnText(n)
		for _, want := range texts {
			if !strings.Contains(text, want) {
				return false
			}
		}
		return true
	})
}

// rowsNamed are the ids of the outermost nodes under list that draw name as a whole text of their own: the row, not the
// parts of it that carry the name.
func rowsNamed(t *testing.T, m model.Model, list, name string) []string {
	t.Helper()
	var all []string
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if strings.HasPrefix(n.ID, list+".") && slices.Contains(drawnPieces(n), name) {
			all = append(all, n.ID)
		}
	}
	return slices.DeleteFunc(all, func(id string) bool {
		return slices.ContainsFunc(all, func(outer string) bool { return strings.HasPrefix(id, outer+".") })
	})
}

// rowsMatching are the ids of the nodes under list that holds accepts and none of whose children it accepts.
func rowsMatching(t *testing.T, m model.Model, list string, holds func(tern.WireNode) bool) []string {
	t.Helper()
	var found []string
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if !strings.HasPrefix(n.ID, list+".") || !holds(n) || slices.ContainsFunc(n.C, holds) {
			continue
		}
		found = append(found, n.ID)
	}
	return found
}

// rowShowing is the id of the one node under list that draws all of texts, failing the test when none or several do.
func rowShowing(t *testing.T, m model.Model, list string, texts ...string) string {
	t.Helper()
	found := rowsShowing(t, m, list, texts...)
	if len(found) != 1 {
		t.Fatalf("nodes under %s showing %q = %v, want exactly one", list, texts, found)
	}
	return found[0]
}

// showsIssue holds for a node that draws the short id of issue as a text of its own (after any fold marker) and a text containing its title.
func showsIssue(issue model.Issue) func(tern.WireNode) bool {
	return func(n tern.WireNode) bool {
		pieces := drawnPieces(n)
		// A row's id cell may lead with a fold marker or an indent.
		bare := func(p string) string {
			return strings.TrimLeftFunc(p, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		}
		return slices.ContainsFunc(pieces, func(p string) bool { return bare(p) == issue.ShortID }) && slices.ContainsFunc(pieces, func(p string) bool { return strings.Contains(p, issue.Title) })
	}
}

// issueRows are the ids of the nodes under list that show issue.
func issueRows(t *testing.T, m model.Model, list string, issue model.Issue) []string {
	t.Helper()
	return rowsMatching(t, m, list, showsIssue(issue))
}

// issueRow is the id of the row or card list draws for issue: the node that shows its short id and its title.
func issueRow(t *testing.T, m model.Model, list string, issue model.Issue) string {
	t.Helper()
	found := issueRows(t, m, list, issue)
	if len(found) != 1 {
		t.Fatalf("nodes under %s showing Issue %s %q = %v, want exactly one", list, issue.ShortID, issue.Title, found)
	}
	return found[0]
}

// rowOf is the id of the row or card list draws for the Issue uid, found by that Issue's short id and title.
func rowOf(t *testing.T, m model.Model, list, uid string) string {
	t.Helper()
	for _, issue := range m.Issues {
		if issue.UID == uid {
			return issueRow(t, m, list, issue)
		}
	}
	t.Fatalf("the model holds no Issue %s", uid)
	return ""
}

// rowOfDrawn reports whether list draws a row or card for the Issue uid.
func rowOfDrawn(t *testing.T, m model.Model, list, uid string) bool {
	t.Helper()
	for _, issue := range m.Issues {
		if issue.UID == uid {
			return len(issueRows(t, m, list, issue)) > 0
		}
	}
	return false
}

// namedRow is the id of the one row under list that draws name as a whole text of its own.
func namedRow(t *testing.T, m model.Model, list, name string) string {
	t.Helper()
	found := rowsNamed(t, m, list, name)
	if len(found) != 1 {
		t.Fatalf("nodes under %s named %q = %v, want exactly one", list, name, found)
	}
	return found[0]
}

// namedRowDrawn reports whether the view draws a row under list named name.
func namedRowDrawn(t *testing.T, m model.Model, list, name string) bool {
	t.Helper()
	return len(rowsNamed(t, m, list, name)) > 0
}

// jobRow is the id of the card the Jobs Page draws for the job uid, found by its name.
func jobRow(t *testing.T, m model.Model, uid string) string {
	t.Helper()
	for _, job := range m.Jobs.Jobs {
		if job.UID == uid {
			return namedRow(t, m, view.JobListID, job.Name)
		}
	}
	t.Fatalf("the Jobs Page holds no job %s", uid)
	return ""
}

// workflowRow is the id of the row the Workflows Page draws for the workflow uid, found by its name.
func workflowRow(t *testing.T, m model.Model, uid string) string {
	t.Helper()
	for _, flow := range m.Workflows.List {
		if flow.UID == uid {
			return namedRow(t, m, view.WorkflowListID, flow.Name)
		}
	}
	t.Fatalf("the Workflows Page holds no workflow %s", uid)
	return ""
}

// workflowRowDrawn reports whether the Workflows Page draws a row for the workflow uid.
func workflowRowDrawn(t *testing.T, m model.Model, uid string) bool {
	t.Helper()
	for _, flow := range m.Workflows.List {
		if flow.UID == uid {
			return namedRowDrawn(t, m, view.WorkflowListID, flow.Name)
		}
	}
	return false
}

// runRow is the id of the card the Runs Page draws for the Run uid: the card at the Run's place in the list, as Runs of one
// job draw alike.
func runRow(t *testing.T, m model.Model, uid string) string {
	t.Helper()
	at := slices.IndexFunc(m.Runs.Runs, func(run model.Run) bool { return run.UID == uid })
	if at < 0 {
		t.Fatalf("the Runs Page holds no Run %s", uid)
	}
	for _, n := range wireNodes(t, view.Render(m, now)) {
		if n.ID == view.RunListID {
			if at >= len(n.C) {
				t.Fatalf("the Runs Page draws %d cards, none for Run %s", len(n.C), uid)
			}
			return n.C[at].ID
		}
	}
	t.Fatalf("the Runs Page draws no list")
	return ""
}
