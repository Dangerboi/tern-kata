package driver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// Two Projects: work (uid p, where the Pane starts) and infra (uid q). The ticket "far" lives in infra and is related to work's "task".

func infraCard(uid string, labels ...string) model.Issue {
	issue := boardCard(uid, labels...)
	issue.ProjectName, issue.ProjectUID = "infra", new("q")
	return issue
}

func twoProjects() []model.CatalogProject {
	return []model.CatalogProject{
		{Project: model.Project{ID: 7, UID: "p", Name: "work"}},
		{Project: model.Project{ID: 8, UID: "q", Name: "infra"}},
	}
}

// farLink is the related link from work's task to infra's far.
func farLink() model.Link {
	return model.Link{ID: 1, Type: "related", FromIssueUID: "task", FromQualifiedID: "work#task", FromStatus: "open", ToIssueUID: "far", ToQualifiedID: "infra#far", ToStatus: "open"}
}

// answeredRead answers the snapshot read in cmds with the snapshot, and returns the state after it.
func answeredRead(t *testing.T, s driver.State, cmds []model.Cmd, snapshot model.Snapshot) driver.State {
	t.Helper()
	loaded, ok := snapshotFor(cmds)
	if !ok {
		t.Fatalf("no snapshot read in %#v", cmds)
	}
	snapshot.Catalog = twoProjects()
	loaded.Snapshot = snapshot
	return step(t, s, loaded).State
}

// inWork answers work's snapshot read (task, with the link to far), and inInfra infra's (far, with the same link).
func inWork(t *testing.T, s driver.State, cmds []model.Cmd) driver.State {
	t.Helper()
	return answeredRead(t, s, cmds, model.Snapshot{Issues: []model.Issue{boardCard("task")}, Links: []model.Link{farLink()}})
}

func inInfra(t *testing.T, s driver.State, cmds []model.Cmd) driver.State {
	t.Helper()
	return answeredRead(t, s, cmds, model.Snapshot{Issues: []model.Issue{infraCard("far")}, Links: []model.Link{farLink()}})
}

func readsProject(cmds []model.Cmd, uid string) bool {
	return slices.ContainsFunc(cmds, func(c model.Cmd) bool {
		read, ok := c.(model.ReadSnapshot)
		return ok && read.Query.ProjectUID != nil && *read.Query.ProjectUID == uid
	})
}

// workBoard is the Board of work with task selected and its related link to infra's far known.
func workBoard(t *testing.T) driver.State {
	t.Helper()
	s := boardPane(t, boardCard("task"))
	out := step(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: nextEvent(), Type: "issue.linked"})
	return step(t, inWork(t, out.State, out.Cmds), model.SelectMsg{UID: "task"}).State
}

func topBarText(t *testing.T, s driver.State) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "main.topbar.left.heading" {
			return wireText(n)
		}
	}
	t.Fatal("no top bar heading drawn")
	return ""
}

func TestFollowingARelatedIdOfAnotherProjectOnTheBoardSwitchesTheProjectAndEscComesBack(t *testing.T) {
	s := workBoard(t)
	out := step(t, s, click("", refClick(t, s, "task", "far")))
	if !readsProject(out.Cmds, "q") {
		t.Fatalf("the follow reads no snapshot of infra: %#v", out.Cmds)
	}
	s = inInfra(t, out.State, out.Cmds)
	s = step(t, s, model.LanesLoaded{ProjectUID: "q"}).State
	m := s.Model
	if m.Collection != model.ProjectCollection("q", "infra") || !m.BoardShown() || m.SelectedUID != "far" || m.Focus != model.ColumnList {
		t.Fatalf("collection %v board %v selected %q focus %v; want infra's Board with far selected", m.Collection, m.BoardShown(), m.SelectedUID, m.Focus)
	}
	if !strings.Contains(topBarText(t, s), "Project: infra") {
		t.Errorf("top bar %s does not name infra", topBarText(t, s))
	}

	back := step(t, s, key("escape"))
	if !readsProject(back.Cmds, "p") {
		t.Fatalf("esc reads no snapshot of work: %#v", back.Cmds)
	}
	s = inWork(t, back.State, back.Cmds)
	s = step(t, s, model.LanesLoaded{ProjectUID: "p"}).State
	if m := s.Model; m.Collection != model.ProjectCollection("p", "work") || !m.BoardShown() || m.SelectedUID != "task" || m.Focus != model.ColumnList {
		t.Errorf("after esc: collection %v board %v selected %q focus %v; want work's Board with task selected", m.Collection, m.BoardShown(), m.SelectedUID, m.Focus)
	}
	if !strings.Contains(topBarText(t, s), "Project: work") {
		t.Errorf("top bar %s does not name work after esc", topBarText(t, s))
	}
}

func TestEscGoesBackOneProjectPerHop(t *testing.T) {
	s := workBoard(t)
	out := step(t, s, click("", refClick(t, s, "task", "far")))
	s = step(t, inInfra(t, out.State, out.Cmds), model.LanesLoaded{ProjectUID: "q"}).State
	// far's own card lists task as related: following it goes back to work as a second hop.
	out = step(t, s, click("", refClick(t, s, "far", "task")))
	s = step(t, inWork(t, out.State, out.Cmds), model.LanesLoaded{ProjectUID: "p"}).State
	if m := s.Model; m.Collection.ProjectUID != "p" || m.SelectedUID != "task" {
		t.Fatalf("second hop: collection %v selected %q", m.Collection, m.SelectedUID)
	}
	out = step(t, s, key("escape"))
	s = step(t, inInfra(t, out.State, out.Cmds), model.LanesLoaded{ProjectUID: "q"}).State
	if m := s.Model; m.Collection.ProjectUID != "q" || m.SelectedUID != "far" || !m.BoardShown() {
		t.Fatalf("first esc: collection %v selected %q", m.Collection, m.SelectedUID)
	}
	out = step(t, s, key("escape"))
	s = step(t, inWork(t, out.State, out.Cmds), model.LanesLoaded{ProjectUID: "p"}).State
	if m := s.Model; m.Collection.ProjectUID != "p" || m.SelectedUID != "task" || !m.BoardShown() {
		t.Errorf("second esc: collection %v selected %q", m.Collection, m.SelectedUID)
	}
}

func TestEscAfterAFollowFromTheLaneViewRestoresTheLaneView(t *testing.T) {
	s := step(t, workBoard(t), key("enter")).State
	if s.Model.Board.Mode != model.BoardLane {
		t.Fatal("enter did not open the Lane view")
	}
	out := step(t, s, model.ShowCardMsg{UID: "far"})
	if m := out.State.Model; m.Collection.ProjectUID != "q" {
		t.Fatalf("collection %v; the follow did not switch", m.Collection)
	}
	s = step(t, inInfra(t, out.State, out.Cmds), model.LanesLoaded{ProjectUID: "q"}).State
	if m := s.Model; m.SelectedUID != "far" || m.Board.Mode != model.BoardLane {
		t.Fatalf("selected %q mode %v; want far in the Lane view", m.SelectedUID, m.Board.Mode)
	}
	out = step(t, s, key("escape"))
	s = step(t, inWork(t, out.State, out.Cmds), model.LanesLoaded{ProjectUID: "p"}).State
	if m := s.Model; m.Collection.ProjectUID != "p" || m.Board.Mode != model.BoardLane || m.SelectedUID != "task" || m.Focus != model.ColumnList {
		t.Errorf("after esc: collection %v mode %v selected %q focus %v; want work's Lane view on task", m.Collection, m.Board.Mode, m.SelectedUID, m.Focus)
	}
}

func TestFollowingATicketTheInfraBoardDoesNotDrawOpensItsDetailBesideTheLaneView(t *testing.T) {
	s := workBoard(t)
	out := step(t, s, click("", refClick(t, s, "task", "far")))
	// infra lists no far card: it is closed beyond Done.
	shut := infraCard("far")
	shut.Status = "closed"
	closed := answeredRead(t, out.State, out.Cmds, model.Snapshot{Issues: []model.Issue{shut}})
	s = step(t, closed, model.LanesLoaded{ProjectUID: "q"}).State
	if m := s.Model; m.Collection.ProjectUID != "q" || m.Board.Mode != model.BoardLane || m.SelectedUID != "far" || m.Focus != model.ColumnDetail {
		t.Errorf("collection %v mode %v selected %q focus %v; want infra's Lane view with far's detail focused", m.Collection, m.Board.Mode, m.SelectedUID, m.Focus)
	}
	back := step(t, s, key("escape"))
	s = step(t, inWork(t, back.State, back.Cmds), model.LanesLoaded{ProjectUID: "p"}).State
	if m := s.Model; m.Collection.ProjectUID != "p" || !m.BoardShown() || m.SelectedUID != "task" {
		t.Errorf("after esc: collection %v board %v selected %q; want work's Board with task selected", m.Collection, m.BoardShown(), m.SelectedUID)
	}
}

// issuesPane is work's Issues Page with task's detail loaded: its Links tab names infra's far.
func issuesPane(t *testing.T) driver.State {
	t.Helper()
	task := boardCard("task")
	detail := &model.IssueDetailData{Issue: &task, Links: []model.Link{farLink()}}
	s := loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"},
		model.Snapshot{Catalog: twoProjects(), Issues: []model.Issue{task, boardCard("near")}, Links: []model.Link{farLink()}},
		model.SelectMsg{UID: "task"}, model.DetailLoaded{UID: "task", Selected: detail})
	return s
}

func TestFollowingALinksRowOfAnotherProjectSwitchesTheProjectAndEscComesBack(t *testing.T) {
	s := issuesPane(t)
	out := step(t, s, model.JumpIssueMsg{UID: "far"})
	if !readsProject(out.Cmds, "q") {
		t.Fatalf("the jump reads no snapshot of infra: %#v", out.Cmds)
	}
	s = inInfra(t, out.State, out.Cmds)
	m := s.Model
	if m.Collection != model.ProjectCollection("q", "infra") || m.SelectedUID != "far" || m.Focus != model.ColumnDetail || m.OnBoard() {
		t.Fatalf("collection %v selected %q focus %v; want infra's Issues with far's detail focused", m.Collection, m.SelectedUID, m.Focus)
	}
	if !strings.Contains(topBarText(t, s), "Project: infra") {
		t.Errorf("top bar %s does not name infra", topBarText(t, s))
	}
	back := step(t, s, key("escape"))
	s = inWork(t, back.State, back.Cmds)
	if m := s.Model; m.Collection != model.ProjectCollection("p", "work") || m.SelectedUID != "task" {
		t.Errorf("after esc: collection %v selected %q; want work with task", m.Collection, m.SelectedUID)
	}
}

func TestFollowingAKataLinkOfAnotherProjectSwitchesTheProject(t *testing.T) {
	s := issuesPane(t)
	out := step(t, s, model.ActionMsg{Action: model.SelectIssueAction, Value: "infra/far"})
	far := infraCard("far")
	out = step(t, out.State, model.IssueOpened{Ref: "infra/far", Selected: &model.IssueDetailData{Issue: &far}})
	if !readsProject(out.Cmds, "q") {
		t.Fatalf("the kata link reads no snapshot of infra: %#v", out.Cmds)
	}
	s = inInfra(t, out.State, out.Cmds)
	if m := s.Model; m.Collection != model.ProjectCollection("q", "infra") || m.SelectedUID != "far" {
		t.Fatalf("collection %v selected %q; want infra with far", m.Collection, m.SelectedUID)
	}
	back := step(t, s, key("escape"))
	s = inWork(t, back.State, back.Cmds)
	if m := s.Model; m.Collection.ProjectUID != "p" || m.SelectedUID != "task" {
		t.Errorf("after esc: collection %v selected %q; want work with task", m.Collection, m.SelectedUID)
	}
}

func TestFollowingInTheSameProjectKeepsTheCollection(t *testing.T) {
	s := issuesPane(t)
	out := step(t, s, model.JumpIssueMsg{UID: "near"})
	if m := out.State.Model; m.Collection != model.ProjectCollection("p", "work") || m.SelectedUID != "near" || readsProject(out.Cmds, "q") {
		t.Errorf("collection %v selected %q cmds %#v; want the same Project with near selected", m.Collection, m.SelectedUID, out.Cmds)
	}
}

func TestFollowingFromAnAllOpenCollectionKeepsTheCollection(t *testing.T) {
	s := issuesPane(t)
	var allOpen model.Collection
	for _, c := range s.Model.Collections {
		if c.Title == "All Open" {
			allOpen = c
		}
	}
	out := step(t, s, model.CollectionSelected{Collection: allOpen})
	s = inWork(t, out.State, out.Cmds)
	out = step(t, s, model.JumpIssueMsg{UID: "far"})
	if m := out.State.Model; m.Collection != allOpen || m.SelectedUID != "far" || readsProject(out.Cmds, "q") {
		t.Errorf("collection %v selected %q cmds %#v; want All Open kept with far selected", m.Collection, m.SelectedUID, out.Cmds)
	}
}
