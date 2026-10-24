package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// siblingsPane is a wide Pane on a parent p with children a to e, in creation order, and the given blocks links (blocker,
// blocked), with p expanded.
func siblingsPane(t *testing.T, blockLinks ...[2]string) driver.State {
	t.Helper()
	snap := model.Snapshot{Issues: []model.Issue{boardCard("p")}}
	for i, uid := range []string{"a", "b", "c", "d", "e"} {
		child := boardCard(uid)
		child.UpdatedAt = child.UpdatedAt.Add(-time.Duration(i) * time.Hour)
		snap.Issues = append(snap.Issues, child)
		snap.Links = append(snap.Links, model.Link{ID: int64(i + 1), Type: "parent", FromIssueUID: uid, ToIssueUID: "p"})
	}
	for i, b := range blockLinks {
		snap.Links = append(snap.Links, blocks(int64(10+i), b[0], "open", b[1]))
	}
	s := loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"}, snap)
	return feed(t, s, key("right")).State
}

// drawnOrder is the Issues' uids in the order the Issue list draws their rows, such as "abcde".
func drawnOrder(t *testing.T, s driver.State, uids ...string) string {
	t.Helper()
	ids := map[string]string{}
	for _, uid := range uids {
		ids[rowOf(t, s.Model, view.IssueListID, uid)] = uid
	}
	var order strings.Builder
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if uid, ok := ids[n.ID]; ok {
			order.WriteString(uid)
		}
	}
	return order.String()
}

func TestGraphOrderTogglesBetweenCreationAndBlockingOrder(t *testing.T) {
	for _, tt := range []struct {
		name        string
		links       [][2]string
		topological string
	}{
		{"unlinked siblings keep creation order", nil, "abcde"},
		{"a chain puts blockers first", [][2]string{{"e", "d"}, {"d", "c"}, {"c", "b"}}, "aedcb"},
		{"a diamond", [][2]string{{"d", "b"}, {"d", "c"}, {"b", "a"}, {"c", "a"}}, "dbcae"},
		{"a cycle follows the rest in creation order", [][2]string{{"b", "c"}, {"c", "b"}, {"e", "a"}}, "deabc"},
		{"links outside the siblings are ignored", [][2]string{{"p", "a"}, {"a", "zzz"}}, "abcde"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := siblingsPane(t, tt.links...)
			all := []string{"a", "b", "c", "d", "e"}
			if got := drawnOrder(t, s, all...); got != "abcde" {
				t.Fatalf("creation order drawn as %q", got)
			}
			s = feed(t, s, key("o")).State
			if got := drawnOrder(t, s, all...); got != tt.topological || s.Model.Notice != "graph order: topological" {
				t.Fatalf("after o: %q, notice %q, want %q", got, s.Model.Notice, tt.topological)
			}
			s = feed(t, s, key("o")).State
			if got := drawnOrder(t, s, all...); got != "abcde" || s.Model.Notice != "graph order: temporal" {
				t.Fatalf("after o again: %q, notice %q", got, s.Model.Notice)
			}
		})
	}
}

func TestGraphOrderDoesNothingOutsideTheNestedList(t *testing.T) {
	links := [][2]string{{"e", "a"}}
	all := []string{"a", "b", "c", "d", "e"}
	flat := feed(t, siblingsPane(t, links...), key("v"), key("o")).State
	if flat.Model.GraphOrder || flat.Model.Notice != "" {
		t.Fatalf("flat list: order %v, notice %q", flat.Model.GraphOrder, flat.Model.Notice)
	}
	nested := siblingsPane(t, links...)
	nested.Model.Focus = model.ColumnDetail
	detail := feed(t, nested, key("o")).State
	if detail.Model.GraphOrder || detail.Model.Notice != "" || drawnOrder(t, detail, all...) != "abcde" {
		t.Fatalf("detail column: order %v, notice %q, drawn %q", detail.Model.GraphOrder, detail.Model.Notice, drawnOrder(t, detail, all...))
	}
}
