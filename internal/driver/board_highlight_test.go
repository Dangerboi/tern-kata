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

// highlight is the tags and the tone of uid's card as drawn: the head's spans and the card's own tone.
func highlight(t *testing.T, s driver.State, uid string) (tags []string, tone string) {
	t.Helper()
	n := node(t, wireNodes(t, view.Render(s.Model, now)), view.CardID(s.Model, uid))
	if head, ok := n.P["head"].([]any); ok {
		for _, span := range head {
			if text := strings.TrimSpace(span.(map[string]any)["t"].(string)); text != "" {
				tags = append(tags, text)
			}
		}
	}
	tone, _ = n.P["tone"].(string)
	return tags, tone
}

func TestEachRelationshipTagsTheOtherCardInItsTone(t *testing.T) {
	s := highlightBoard(t)
	for _, c := range []struct{ uid, tag, tone string }{
		{"par", "parent", "accent"},
		{"kid", "child", "info"},
		{"blocker", "blocks this", "error"},
		{"blocked", "blocked by this", "warning"},
		{"rel", "related", "neutral"},
	} {
		tags, tone := highlight(t, s, c.uid)
		if len(tags) != 1 || tags[0] != c.tag || tone != c.tone {
			t.Errorf("%s: tags %q tone %q, want [%s] %s", c.uid, tags, tone, c.tag, c.tone)
		}
	}
	if tags, tone := highlight(t, s, "stranger"); len(tags) != 0 || tone != "neutral" {
		t.Errorf("an unrelated card has tags %q tone %q", tags, tone)
	}
}

func TestSeveralRelationshipsShowEveryTagAndRingByPrecedence(t *testing.T) {
	tags, tone := highlight(t, highlightBoard(t), "both")
	if want := []string{"blocks this", "child"}; strings.Join(tags, "|") != strings.Join(want, "|") || tone != "error" {
		t.Errorf("tags %q tone %q, want %q error", tags, tone, want)
	}
}

func TestTheSelectedCardKeepsItsRingAndDoneCardsStayMuted(t *testing.T) {
	s := highlightBoard(t)
	if tags, _ := highlight(t, s, "sel"); len(tags) != 0 {
		t.Errorf("the selected card is tagged %q", tags)
	}
	n := node(t, wireNodes(t, view.Render(s.Model, now)), view.CardID(s.Model, "sel"))
	if n.P["selected"] != true {
		t.Errorf("the selected card lost its selected ring")
	}
	tags, tone := highlight(t, s, "old")
	if len(tags) != 1 || tags[0] != "related" || tone != "muted" {
		t.Errorf("a related Done card: tags %q tone %q, want [related] muted", tags, tone)
	}
}

func TestTheHighlightFollowsTheSelectionAndNeedsOne(t *testing.T) {
	s := step(t, highlightBoard(t), model.SelectMsg{UID: "par"}).State
	if tags, tone := highlight(t, s, "sel"); len(tags) != 1 || tags[0] != "child" || tone != "info" {
		t.Errorf("with the parent selected, sel has tags %q tone %q, want [child] info", tags, tone)
	}
	if tags, _ := highlight(t, s, "kid"); len(tags) != 0 {
		t.Errorf("the old selection's relative kept tags %q", tags)
	}
	s.Model.SelectedUID = ""
	for _, uid := range []string{"sel", "par", "blocker"} {
		if tags, tone := highlight(t, s, uid); len(tags) != 0 || tone != "neutral" {
			t.Errorf("without a selection %s has tags %q tone %q", uid, tags, tone)
		}
	}
}

func TestTheLaneViewDrawsNoHighlight(t *testing.T) {
	s := highlightBoard(t)
	s.Model.Board.Mode = model.BoardLane
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if _, ok := n.P["head"]; ok {
			t.Errorf("the Lane view draws a head on %s", n.ID)
		}
	}
}
