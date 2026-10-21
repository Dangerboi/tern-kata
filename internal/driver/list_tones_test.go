package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// listCell is the style of the span of Issue uid's list row whose text contains text, with the detail folded so the owner shows.
func listCell(t *testing.T, s driver.State, uid, text string) string {
	t.Helper()
	row := node(t, wireNodes(t, view.Render(s.Model, now)), view.IssueID(uid))
	for _, part := range []string{"label", "value"} {
		spans, _ := row.P[part].([]any)
		for _, sp := range spans {
			span, _ := sp.(map[string]any)
			if got, _ := span["t"].(string); strings.Contains(got, text) {
				style, _ := span["s"].(string)
				return style
			}
		}
	}
	t.Fatalf("row %s has no span with %q: %v", uid, text, row.P)
	return ""
}

func TestListIdAndOwnerTakeTheBoardsColours(t *testing.T) {
	custom := boardCard("cust", "in-review")
	closed := closedAt("done", time.October, 7, "needs-info")
	issues := []model.Issue{ownedBy(boardCard("triage", "needs-triage"), "ada"), ownedBy(custom, "bo"), ownedBy(closed, "cy"), ownedBy(boardCard("none"), "di")}
	s := step(t, projectPane(t, 160, issues...), key("shift+l")).State
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"needs-triage", "in-review"}}).State
	for _, c := range []struct{ uid, owner, id, want string }{
		{"triage", "ada", "triage", "warning"},
		{"cust", "bo", "cust", "accent"},
		{"done", "cy", "done", "success"},
		{"none", "di", "none", "muted"},
	} {
		if got := listCell(t, s, c.uid, c.id); got != c.want {
			t.Errorf("%s id style = %q, want %q", c.uid, got, c.want)
		}
		if got := listCell(t, s, c.uid, c.owner); got != "accent" {
			t.Errorf("%s owner style = %q, want accent, the nearest text colour to the user tone", c.uid, got)
		}
		if got := listCell(t, s, c.uid, "ago"); got != "muted" {
			t.Errorf("%s updated style = %q, want muted", c.uid, got)
		}
	}
}

func TestOtherCollectionsAskForNoLanes(t *testing.T) {
	s := projectPane(t, 160, boardCard("a"))
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"in-review"}}).State
	for _, c := range model.FixedCollections() {
		if c.IsProject() {
			continue
		}
		out := step(t, s, model.CollectionSelected{Collection: c})
		if got := loadCmds(out.Cmds); len(got) != 0 {
			t.Errorf("%s asked %v, want no load", c.Title, got)
		}
		if got := out.State.Model.LaneList(); len(got) == 1 {
			t.Errorf("%s keeps the Project's Lane list %v, want the default", c.Title, got)
		}
	}
}
