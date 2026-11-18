package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// link is a link of the given type from one Issue to another.
func link(id int64, kind, from, to string) model.Link {
	return model.Link{ID: id, Type: kind, FromIssueUID: from, FromQualifiedID: "work#" + from, FromStatus: "open", ToIssueUID: to, ToQualifiedID: "work#" + to, ToStatus: "open"}
}

// highlightBoard is a Board of cards around "sel": its parent, child, blocker, blocked and related Issue, one Issue that both
// blocks it and is its child, one closed related Issue, and a stranger; sel is selected.
func highlightBoard(t *testing.T) driver.State {
	t.Helper()
	issues := []model.Issue{boardCard("sel"), boardCard("par"), boardCard("kid"), boardCard("blocker"), boardCard("blocked"), boardCard("rel"),
		boardCard("both"), closedAt("old", time.October, 7, "ready-for-human"), boardCard("stranger")}
	links := []model.Link{
		link(1, "parent", "sel", "par"), link(2, "parent", "kid", "sel"), link(3, "blocks", "blocker", "sel"), link(4, "blocks", "sel", "blocked"),
		link(5, "related", "sel", "rel"), link(6, "parent", "both", "sel"), link(7, "blocks", "both", "sel"), link(8, "related", "old", "sel"),
	}
	return step(t, linkedBoard(t, links, issues...), model.SelectMsg{UID: "sel"}).State
}

// highlight is the tag line and the tone of uid's card as drawn: the muted line under the title that names every relationship to
// the selected Issue, joined by commas, with the style it is drawn in ("" when there is none), and the card's own ring tone.
func highlight(t *testing.T, s driver.State, uid string) (line, style, tone string) {
	t.Helper()
	nodes := wireNodes(t, view.Render(s.Model, now))
	card := rowOf(t, s.Model, view.BoardID, uid)
	n := node(t, nodes, card)
	if _, ok := n.P["head"]; ok {
		t.Errorf("%s draws a head, the tags are a line under the title", uid)
	}
	for _, c := range nodes {
		if strings.HasPrefix(c.ID, card+".tag-") {
			line, style = spanText(t, c), spanStyle(t, c)
		}
		if strings.HasPrefix(c.ID, card+".id.tag-") {
			t.Errorf("%s draws a tag badge in the id row", uid)
		}
	}
	tone, _ = n.P["tone"].(string)
	return line, style, tone
}

func TestEachRelationshipTagsTheOtherCardInWords(t *testing.T) {
	s := highlightBoard(t)
	for _, c := range []struct{ uid, line, style, tone string }{
		{"par", "parent of selected", "muted", "neutral"},
		{"kid", "child of selected", "muted", "neutral"},
		{"blocker", "blocks selected", "warning", "warning"},
		{"blocked", "blocked by selected", "warning", "warning"},
		{"rel", "related to selected", "muted", "neutral"},
	} {
		line, style, tone := highlight(t, s, c.uid)
		if line != c.line || style != c.style || tone != c.tone {
			t.Errorf("%s: line %q style %q tone %q, want %q %s %s", c.uid, line, style, tone, c.line, c.style, c.tone)
		}
	}
	if line, _, tone := highlight(t, s, "stranger"); line != "" || tone != "neutral" {
		t.Errorf("an unrelated card has tag line %q tone %q", line, tone)
	}
}

func TestSeveralRelationshipsShareOneLineAndRingByPrecedence(t *testing.T) {
	line, style, tone := highlight(t, highlightBoard(t), "both")
	if line != "blocks selected, child of selected" || style != "warning" || tone != "warning" {
		t.Errorf("line %q style %q tone %q, want both words, warning, warning", line, style, tone)
	}
}

func TestTheSelectedCardKeepsItsRingAndDoneCardsStayMuted(t *testing.T) {
	s := highlightBoard(t)
	if line, _, _ := highlight(t, s, "sel"); line != "" {
		t.Errorf("the selected card is tagged %q", line)
	}
	n := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.BoardID, "sel"))
	if n.P["selected"] != true {
		t.Errorf("the selected card lost its selected ring")
	}
	line, _, tone := highlight(t, s, "old")
	if line != "related to selected" || tone != "muted" {
		t.Errorf("a related Done card: line %q tone %q, want related to selected, muted", line, tone)
	}
}

func TestTheHighlightFollowsTheSelectionAndNeedsOne(t *testing.T) {
	s := step(t, highlightBoard(t), model.SelectMsg{UID: "par"}).State
	if line, _, tone := highlight(t, s, "sel"); line != "child of selected" || tone != "neutral" {
		t.Errorf("with the parent selected, sel has line %q tone %q, want child of selected, neutral", line, tone)
	}
	if line, _, _ := highlight(t, s, "kid"); line != "" {
		t.Errorf("the old selection's relative kept line %q", line)
	}
	s.Model.SelectedUID = ""
	for _, uid := range []string{"sel", "par", "blocker"} {
		if line, _, tone := highlight(t, s, uid); line != "" || tone != "neutral" {
			t.Errorf("without a selection %s has line %q tone %q", uid, line, tone)
		}
	}
}

func TestTheLaneViewDrawsNoHighlight(t *testing.T) {
	s := highlightBoard(t)
	s.Model.Board.Mode = model.BoardLane
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if _, ok := n.P["head"]; ok || strings.Contains(n.ID, ".tag-") {
			t.Errorf("the Lane view draws a highlight on %s", n.ID)
		}
	}
}
