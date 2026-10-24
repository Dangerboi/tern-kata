package driver_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

var moveCatalog = []model.CatalogProject{
	{Project: model.Project{ID: 7, UID: "p", Name: "work"}},
	{Project: model.Project{ID: 8, UID: "s", Name: "shop"}},
	{Project: model.Project{ID: 9, UID: "o", Name: "old", Archived: true}},
	{Project: model.Project{ID: 10, UID: "h", Name: "home"}},
}

// movable is a card of the Project "work" at revision 3.
func movable(uid string) model.Issue {
	issue := boardCard(uid)
	issue.ProjectID, issue.Revision = 7, 3
	return issue
}

// movePane is a Pane on "work" whose catalog also holds shop, home and an archived Project.
func movePane(t *testing.T, issues ...model.Issue) driver.State {
	t.Helper()
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Catalog: moveCatalog}, LocalDate: boardToday, TimeZone: "UTC"})
	return feed(t, s).State
}

// moveListed are the Projects the open Move card draws, in drawn order.
func moveListed(t *testing.T, s driver.State) []string {
	t.Helper()
	var names []string
	for _, p := range moveCatalog {
		if len(rowsNamed(t, s.Model, view.MoveListID, p.Project.Name)) > 0 {
			names = append(names, p.Project.Name)
		}
	}
	return names
}

func moveText(t *testing.T, s driver.State, leaf string) string {
	t.Helper()
	text, _ := nodeText(t, s, view.LayerID(model.CardMove)+"."+leaf)
	return text
}

// movedSnapshot is the daemon answering the snapshot read in cmds with issues, the catalog as before.
func movedSnapshot(t *testing.T, s driver.State, cmds []model.Cmd, issues ...model.Issue) driver.State {
	t.Helper()
	answer, ok := snapshotFor(cmds, issues...)
	if !ok {
		t.Fatalf("no snapshot read asked for: %#v", cmds)
	}
	answer.Snapshot.Catalog = moveCatalog
	return step(t, s, answer).State
}

func theMove(t *testing.T, cmds []model.Cmd) model.MoveToProjectCmd {
	t.Helper()
	var found []model.MoveToProjectCmd
	for _, cmd := range cmds {
		if move, ok := cmd.(model.MoveToProjectCmd); ok {
			found = append(found, move)
		}
	}
	if len(found) != 1 {
		t.Fatalf("cmds %#v, want exactly one move request", cmds)
	}
	return found[0]
}

// moving is the card after Enter on the Project called name: the dry run is out.
func moving(t *testing.T, s driver.State, name string) driver.Output {
	t.Helper()
	open := step(t, s, key("shift+m")).State
	for open.Model.MoveTargets()[open.Model.MoveProject.Cursor].Name != name {
		open = step(t, open, key("down")).State
	}
	return step(t, open, key("enter"))
}

func TestMOpensTheMoveCardInEveryModeListingOtherActiveProjects(t *testing.T) {
	issue := movable("a")
	list := movePane(t, issue, movable("b"))
	board := feed(t, list, key("b"), model.LanesLoaded{ProjectUID: "p"}).State
	lane := step(t, board, key("enter")).State
	detail := step(t, list, key("enter")).State
	for name, s := range map[string]driver.State{"list": list, "Board": board, "Lane view": lane, "detail": detail} {
		t.Run(name, func(t *testing.T) {
			open := step(t, s, key("shift+m"))
			m := open.State.Model
			if m.CurrentMode() != model.ModeMoveProject || m.TopCard() != model.CardMove || len(open.Cmds) != 0 {
				t.Fatalf("mode %s card %v cmds %v", m.CurrentMode(), m.TopCard(), open.Cmds)
			}
			if got := moveListed(t, open.State); !slices.Equal(got, []string{"shop", "home"}) {
				t.Errorf("lists %v, want shop and home: not the Issue's own Project and not the archived one", got)
			}
			if !strings.HasPrefix(open.State.Focus, view.LayerID(model.CardMove)) {
				t.Errorf("text focus %q is not on the Move card", open.State.Focus)
			}
			closed := step(t, open.State, key("escape"))
			if closed.State.Model.TopCard() != model.NoCard || len(closed.Cmds) != 0 {
				t.Fatalf("esc: card %v cmds %v", closed.State.Model.TopCard(), closed.Cmds)
			}
		})
	}
}

func TestMoveCardFiltersProjectsByTypedName(t *testing.T) {
	s := typed(t, step(t, movePane(t, movable("a")), key("shift+m")).State, "OM")
	if got := moveListed(t, s); !slices.Equal(got, []string{"home"}) {
		t.Errorf("lists %v after typing OM, want home", got)
	}
	s = typed(t, s, "zzz")
	if got := moveListed(t, s); len(got) != 0 {
		t.Errorf("lists %v for a name no Project has", got)
	}
	if out := step(t, s, key("enter")); len(out.Cmds) != 0 {
		t.Errorf("enter on an empty list sent %v", out.Cmds)
	}
}

func TestMoveCardPreviewsThenMovesGuardedOnTheRevisionItOpenedWith(t *testing.T) {
	s := movePane(t, movable("a"), movable("b"))
	preview := moving(t, s, "shop")
	want := model.MoveToProjectCmd{Workspace: "/repo", UID: "a", ProjectID: 7, ToProjectUID: "s", DryRun: true, Revision: 3}
	if got := theMove(t, preview.Cmds); got != want {
		t.Fatalf("preview %+v, want %+v", got, want)
	}
	if !preview.State.Model.MoveProject.Sending {
		t.Fatal("the card does not wait for kata")
	}
	if out := step(t, preview.State, key("enter")); len(out.Cmds) != 0 {
		t.Errorf("a key while the preview is out sent %v", out.Cmds)
	}

	previewed := step(t, preview.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 3}).State
	if previewed.Model.CurrentMode() != model.ModeMoveConfirm {
		t.Fatalf("mode %s after the preview, want the confirm mode", previewed.Model.CurrentMode())
	}
	if text := moveText(t, previewed, "preview"); !strings.Contains(text, "moves to shop") || strings.Contains(text, "as ") {
		t.Errorf("preview %q, want it to name the Project and, since kata assigns the id on moving, no id", text)
	}

	move := step(t, previewed, key("enter"))
	want.DryRun = false
	if got := theMove(t, move.Cmds); got != want {
		t.Fatalf("move %+v, want %+v", got, want)
	}
}

func TestMoveCardShowsKatasRefusalInThePreviewAndKeepsListing(t *testing.T) {
	preview := moving(t, movePane(t, movable("a")), "shop")
	refused := step(t, preview.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Err: errors.New("cross-project moves involving a federated project are unsupported")}).State
	if refused.Model.TopCard() != model.CardMove || refused.Model.CurrentMode() != model.ModeMoveProject {
		t.Fatalf("card %v mode %s, want the list again", refused.Model.TopCard(), refused.Model.CurrentMode())
	}
	if got := moveText(t, refused, "error"); !strings.Contains(got, "federated project are unsupported") {
		t.Errorf("error %q lacks kata's refusal", got)
	}
	if got := moveListed(t, refused); !slices.Equal(got, []string{"shop", "home"}) {
		t.Errorf("lists %v, want the Projects to choose another from", got)
	}
	if out := step(t, refused, key("enter")); len(out.Cmds) != 1 {
		t.Errorf("another Enter sent %v, want a new preview", out.Cmds)
	}
}

func TestMoveCardConflictKeepsTheCardOpenAndOffersAFreshPreview(t *testing.T) {
	s := movePane(t, movable("a"))
	previewed := step(t, moving(t, s, "shop").State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 3}).State
	moved := step(t, previewed, key("enter"))
	conflict := step(t, moved.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", Conflict: true})
	if conflict.State.Model.TopCard() != model.CardMove || len(conflict.Cmds) != 0 {
		t.Fatalf("card %v cmds %v, want the card open and nothing sent", conflict.State.Model.TopCard(), conflict.Cmds)
	}
	if got := moveText(t, conflict.State, "preview"); !strings.Contains(got, "changed since") || !strings.Contains(got, "nothing was moved") {
		t.Errorf("conflict text %q", got)
	}

	fresh := step(t, conflict.State, key("enter"))
	if got := theMove(t, fresh.Cmds); !got.DryRun || got.Revision != 0 || got.ToProjectUID != "s" {
		t.Fatalf("fresh preview %+v, want a dry run without a stale revision", got)
	}
	again := step(t, fresh.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 4}).State
	if got := moveText(t, again, "preview"); !strings.Contains(got, "moves to shop") {
		t.Errorf("preview %q after the fresh preview", got)
	}
	if got := theMove(t, step(t, again, key("enter")).Cmds); got.DryRun || got.Revision != 4 {
		t.Errorf("move %+v, want the real move guarded on the revision the fresh preview showed", got)
	}
}

func TestMoveCardNoticeLinksToTheNewProjectAndIssueAndOpensIt(t *testing.T) {
	s := movePane(t, movable("a"), movable("b"))
	previewed := step(t, moving(t, s, "shop").State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 3}).State
	sent := step(t, previewed, key("enter"))
	done := step(t, sent.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", NewShortID: "n3w1"})
	if done.State.Model.TopCard() != model.NoCard {
		t.Fatalf("card %v still open after the move", done.State.Model.TopCard())
	}
	if _, ok := snapshotFor(done.Cmds); !ok {
		t.Errorf("no fresh snapshot asked for after the move: %v", done.Cmds)
	}
	if !strings.Contains(done.State.Model.Notice, "shop#n3w1") {
		t.Fatalf("notice %q, want it to name shop#n3w1", done.State.Model.Notice)
	}
	var link string
	for _, n := range wireNodes(t, view.Render(done.State.Model, now)) {
		if strings.HasSuffix(n.ID, "notice") {
			link = n.ID
		}
	}
	if link == "" {
		t.Fatal("the notice is not drawn")
	}
	opened := step(t, done.State, click(link, view.NoticeLinkClick))
	var open *model.OpenIssue
	for _, cmd := range opened.Cmds {
		if c, ok := cmd.(model.OpenIssue); ok {
			open = &c
		}
	}
	if open == nil || open.Ref != "shop/n3w1" || open.ProjectID != 8 {
		t.Fatalf("clicking the notice sent %#v, want the new Issue opened in shop", opened.Cmds)
	}
	if opened.State.Model.Notice != "" {
		t.Errorf("notice %q stays after the click", opened.State.Model.Notice)
	}
}

func TestMovingAnIssueSelectsTheNeighbouringRowOnceTheCollectionNoLongerShowsIt(t *testing.T) {
	a, b, c := movable("a"), movable("b"), movable("c")
	for _, tt := range []struct{ name, selected, want string }{
		{"a middle row goes to the one after it", "b", "c"},
		{"the last row goes to the one before it", "c", "b"},
		{"the first row goes to the one after it", "a", "b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := movePane(t, a, b, c)
			s = step(t, s, model.SelectMsg{UID: tt.selected}).State
			previewed := step(t, moving(t, s, "shop").State, model.ProjectMoveAnswer{UID: tt.selected, ToProjectUID: "s", DryRun: true, Revision: 3}).State
			sent := step(t, previewed, key("enter"))
			done := step(t, sent.State, model.ProjectMoveAnswer{UID: tt.selected, ToProjectUID: "s", NewShortID: "n3w1"})
			var rest []model.Issue
			for _, issue := range []model.Issue{a, b, c} {
				if issue.UID != tt.selected {
					rest = append(rest, issue)
				}
			}
			after := movedSnapshot(t, done.State, done.Cmds, rest...)
			if after.Model.SelectedUID != tt.want {
				t.Errorf("selected %q, want %q", after.Model.SelectedUID, tt.want)
			}
		})
	}
}

func TestMovingTheIssueOfTheOpenDetailLeavesItForTheList(t *testing.T) {
	a, b := movable("a"), movable("b")
	s := step(t, movePane(t, a, b), key("enter")).State
	if s.Model.Focus != model.ColumnDetail {
		t.Fatalf("focus %v, want the detail", s.Model.Focus)
	}
	previewed := step(t, moving(t, s, "shop").State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 3}).State
	sent := step(t, previewed, key("enter"))
	done := step(t, sent.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", NewShortID: "n3w1"})
	after := movedSnapshot(t, done.State, done.Cmds, b)
	if after.Model.Focus != model.ColumnList || after.Model.SelectedUID != "b" {
		t.Errorf("focus %v selected %q, want the list on the neighbouring row", after.Model.Focus, after.Model.SelectedUID)
	}
}

func TestMoveCardDropsAnAnswerForAnotherIssueOrProject(t *testing.T) {
	preview := moving(t, movePane(t, movable("a")), "shop")
	for _, stale := range []model.ProjectMoveAnswer{
		{UID: "other", ToProjectUID: "s", DryRun: true},
		{UID: "a", ToProjectUID: "h", DryRun: true},
	} {
		got := step(t, preview.State, stale).State.Model
		if !got.MoveProject.Sending || got.MoveProject.Previewed {
			t.Errorf("answer %+v changed the card: %+v", stale, got.MoveProject)
		}
	}
}

func TestMoveCardEscStepsBackFromThePreviewToTheList(t *testing.T) {
	previewed := step(t, moving(t, movePane(t, movable("a")), "shop").State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 3}).State
	back := step(t, previewed, key("escape")).State
	if back.Model.TopCard() != model.CardMove || back.Model.CurrentMode() != model.ModeMoveProject {
		t.Fatalf("card %v mode %s, want the list", back.Model.TopCard(), back.Model.CurrentMode())
	}
	if got := moveListed(t, back); !slices.Equal(got, []string{"shop", "home"}) {
		t.Errorf("lists %v", got)
	}
}

func TestMoveClearsTheUndoHistoryOnlyWhenTheIssueMoves(t *testing.T) {
	s := movePane(t, movable("a"), movable("b"))
	entry := undoEntry(2)
	entry.UID, entry.ProjectID = "a", 7
	s = feed(t, s, model.FieldWritten{UID: "a", Record: model.WriteRecord{Entry: &entry}}).State
	if len(s.Model.Undo.Entries) != 1 {
		t.Fatalf("history %+v, want the change recorded", s.Model.Undo)
	}

	preview := moving(t, s, "shop")
	previewed := step(t, preview.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", DryRun: true, Revision: 3}).State
	if len(previewed.Model.Undo.Entries) != 1 {
		t.Fatalf("a preview left history %+v, want it kept", previewed.Model.Undo)
	}

	sent := step(t, previewed, key("enter"))
	done := step(t, sent.State, model.ProjectMoveAnswer{UID: "a", ToProjectUID: "s", NewShortID: "n3w1"}).State
	if len(sent.State.Model.Undo.Entries) != 0 || len(done.Model.Undo.Entries) != 0 {
		t.Fatalf("history after the move %+v, want it cleared", done.Model.Undo)
	}
	if got := pressU(t, done).State.Model.Notice; got != "nothing to undo: moving an Issue cannot be undone" {
		t.Errorf("u says %q", got)
	}
}
