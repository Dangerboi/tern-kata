package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

var now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

var (
	alpha = model.Issue{UID: "alpha", ShortID: "aaaa", Title: "Alpha", Status: "open", ProjectName: "work"}
	beta  = model.Issue{UID: "beta", ShortID: "bbbb", Title: "Beta", Status: "open", ProjectName: "work"}
)

func key(chord string) tern.Key {
	if len(chord) == 1 {
		return tern.Key{Name: chord, Text: chord}
	}
	if name, ok := strings.CutPrefix(chord, "ctrl+"); ok {
		return tern.Key{Name: name, Ctrl: true}
	}
	return tern.Key{Name: chord}
}

// openMenu is what a click on the top bar's Menu sends.
var openMenu = model.ActionMsg{Action: model.OpenPicker}

func resize(cols int) model.ResizeMsg { return model.ResizeMsg{Cols: cols} }

func alphaLoaded() model.Msg {
	return model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &alpha}}
}

// drawnIDs is every node id Tern would hold after v is rendered.
func drawnIDs(t *testing.T, v tern.View) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, node := range wireNodes(t, v) {
		ids[node.ID] = true
	}
	return ids
}

// step runs one input and checks what holds for every step: no error, and no
// reveal or scroll names a node the rendered view lacks (Tern ends the program
// on those). A focus op may name a node a hidden layer or a narrow Pane does not
// draw: Tern's "unknown id" answer to it is ignored.
func step(t *testing.T, s driver.State, in any) driver.Output {
	t.Helper()
	out := driver.Step(s, in, now)
	if out.Err != nil {
		t.Fatalf("%T: fatal error %v", in, out.Err)
	}
	if out.View == nil {
		return out
	}
	ids := drawnIDs(t, *out.View)
	for _, op := range out.Ops {
		if id, ok := op[1].(string); ok && op[0] != "focus" && !ids[id] {
			t.Errorf("%T: op %v names %q, which the view does not draw", in, op, id)
		}
	}
	return out
}

func feed(t *testing.T, s driver.State, inputs ...any) driver.Output {
	t.Helper()
	var out driver.Output
	out.State = s
	for _, in := range inputs {
		out = step(t, out.State, in)
	}
	return out
}

// browsing is a Pane of the given width on a Project of alpha and beta, alpha's detail loaded.
func browsing(t *testing.T, cols int) driver.State {
	t.Helper()
	return loadedWith(t, cols, model.Project{ID: 7, UID: "p", Name: "work"}, model.Snapshot{Issues: []model.Issue{alpha, beta}, Catalog: []model.CatalogProject{{Project: model.Project{ID: 7, UID: "p", Name: "work"}}}}, alphaLoaded())
}

// loadedWith is a Pane of the given width after its first load of the Project and snapshot, then the given messages.
func loadedWith(t *testing.T, cols int, project model.Project, snapshot model.Snapshot, then ...any) driver.State {
	t.Helper()
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(cols)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: project}, model.SnapshotLoaded{Snapshot: snapshot})
	return feed(t, s, then...).State
}

func wantOps(t *testing.T, out driver.Output, want ...tern.Op) {
	t.Helper()
	if len(out.Ops) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(out.Ops, want) {
		t.Fatalf("ops = %v, want %v", out.Ops, want)
	}
}

func TestTextFocusIsSentOnlyOnChange(t *testing.T) {
	s := browsing(t, 160)
	out := step(t, s, key("/"))
	wantOps(t, out, tern.OpFocus(view.SearchID))
	out = step(t, out.State, key("a"))
	wantOps(t, out)
	// Leaving the search restores the Issues, so the selected row is revealed after the caret goes.
	out = step(t, out.State, key("escape"))
	wantOps(t, out, tern.OpFocus(""), tern.OpReveal(issueRow(t, out.State.Model, view.IssueListID, alpha), "nearest"))
}

func TestTextFocusIsClearedWhileSavingOrConfirming(t *testing.T) {
	open := feed(t, browsing(t, 160), key("n"), key("x"))
	if len(open.Ops) != 0 {
		t.Fatalf("typing into the focused field sent %v", open.Ops)
	}
	saving := step(t, open.State, key("ctrl+o"))
	wantOps(t, saving, tern.OpFocus(""))
	if len(saving.Cmds) != 1 {
		t.Fatalf("saving commands = %#v", saving.Cmds)
	}

	confirming := step(t, open.State, key("escape"))
	wantOps(t, confirming, tern.OpFocus(""))

	draft := feed(t, browsing(t, 160), key("e"))
	wantOps(t, step(t, draft.State, key("ctrl+o")), tern.OpFocus(""))
}

func TestOpeningACardFocusesItsField(t *testing.T) {
	out := step(t, browsing(t, 160), key("n"))
	wantOps(t, out, tern.OpFocus(view.FieldID(model.CardCreate, model.CreateTitle)))
	out = step(t, out.State, &tern.Focus{ID: view.FieldID(model.CardCreate, model.CreateLabels)})
	wantOps(t, out, tern.OpFocus(view.FieldID(model.CardCreate, model.CreateLabels)))
}

func TestRowIsRevealedOnKeyMovesButNotOnRowClicks(t *testing.T) {
	s := browsing(t, 160)
	out := step(t, s, key("j"))
	betaRow := issueRow(t, s.Model, view.IssueListID, beta)
	wantOps(t, out, tern.OpReveal(betaRow, "nearest"))
	if !reflect.DeepEqual(out.Cmds, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "beta"}}) {
		t.Fatalf("commands = %#v", out.Cmds)
	}

	out = step(t, s, &tern.Select{ID: view.IssueListID, Item: betaRow})
	if out.State.Model.SelectedUID != "beta" {
		t.Fatalf("click did not select: %q", out.State.Model.SelectedUID)
	}
	wantOps(t, out)

	out = step(t, s, click(betaRow, view.ToggleClick("beta")))
	wantOps(t, out)
}

func TestRowIsNotRevealedUnlessTheListIsFocusedAndDrawn(t *testing.T) {
	s := browsing(t, 160)
	out := step(t, s, click("", view.FocusDetailClick))
	if out.State.Model.Focus != model.ColumnDetail {
		t.Fatalf("fixture: focus %v", out.State.Model.Focus)
	}
	wantOps(t, step(t, out.State, key("j")))
}

// Every op of every step is checked against the drawn view by step; this sweeps
// the width tiers, both column focuses and every key that can scroll or reveal.
func TestNoOpNamesAColumnTheViewDoesNotDraw(t *testing.T) {
	betaRow := issueRow(t, browsing(t, 160).Model, view.IssueListID, beta)
	inputs := []any{
		key("j"), key("k"), key("page_down"), key("page_up"), key("down"), key("up"), key("tab"), key("shift+tab"), key("enter"), key("escape"),
		click("", view.ActivityTabClick(model.TabEvents)), click("", view.FocusDetailClick),
		&tern.Select{ID: view.IssueListID, Item: betaRow},
	}
	for _, cols := range []int{60, 79, 80, 100, 139, 140, 160} {
		for _, detailFocused := range []bool{false, true} {
			base := browsing(t, cols)
			if detailFocused {
				base = step(t, base, click("", view.FocusDetailClick)).State
			}
			for _, in := range inputs {
				step(t, base, in)
			}
		}
	}
}

func TestScrollsAndRevealsAreDroppedWhenNothingIsDrawn(t *testing.T) {
	detailFocused := func(cols int) driver.State {
		return feed(t, browsing(t, 160), click("", view.FocusDetailClick), resize(cols)).State
	}
	firstLoad := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	failed := feed(t, browsing(t, 160), click("", view.FocusDetailClick), model.Failed{Err: errors.New("kata server not responding")}).State
	for name, s := range map[string]driver.State{"below 80 columns": detailFocused(79), "first load": firstLoad, "daemon failure": failed} {
		for _, chord := range []string{"page_down", "page_up", "down", "up", "tab", "shift+tab", "j"} {
			for _, op := range step(t, s, key(chord)).Ops {
				if op[0] == "reveal" || op[0] == "scroll" {
					t.Errorf("%s, %s: sent %v", name, chord, op)
				}
			}
		}
	}
}

// The detail column shows a placeholder instead of its scroller while it holds no Issue: an error line or an Issue kata no
// longer shows. A scroll key there must send nothing, since Tern ends the program on a scroll of a node it doesn't draw.
func TestDetailScrollsAreDroppedWhileTheDetailHoldsNoIssue(t *testing.T) {
	for name, then := range map[string]model.Msg{
		"detail error": model.DetailFailed{UID: "alpha", Err: errors.New("kata server not responding")},
		"unavailable":  model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{}},
	} {
		for _, cols := range []int{100, 160} {
			s := feed(t, browsing(t, cols), click("", view.FocusDetailClick), then).State
			for _, chord := range []string{"page_down", "page_up", "down", "up"} {
				for _, op := range step(t, s, key(chord)).Ops {
					if op[0] == "scroll" {
						t.Errorf("%s at %d columns, %s: sent %v", name, cols, chord, op)
					}
				}
			}
		}
	}
}

func TestScrollAndRevealAreSentForDrawnColumns(t *testing.T) {
	s := feed(t, browsing(t, 160), click("", view.FocusDetailClick)).State
	wantOps(t, step(t, s, key("page_down")), tern.OpScroll(view.DetailContentID, "page-down"))
	wantOps(t, step(t, s, key("down")), tern.OpScroll(view.DetailContentID, "line-down"))

	// The only column drawn is the detail below 140 columns.
	narrow := feed(t, browsing(t, 100), click("", view.FocusDetailClick)).State
	wantOps(t, step(t, narrow, key("page_up")), tern.OpScroll(view.DetailContentID, "page-up"))
}

func TestActivityTabRevealGoesOutInOrder(t *testing.T) {
	s := feed(t, browsing(t, 160), click("", view.FocusDetailClick)).State
	want := []tern.Op{tern.OpReveal(view.DetailActivityID+".empty", "nearest"), tern.OpReveal(view.DetailActivityID+".tabs", "nearest")}
	for name, in := range map[string]any{"tab": key("tab"), "shift+tab": key("shift+tab"), "click": click("", view.ActivityTabClick(model.TabEvents))} {
		t.Run(name, func(t *testing.T) { wantOps(t, step(t, s, in), want...) })
	}
	wantOps(t, step(t, s, click("", view.ActivityTabClick(model.TabComments))))
}

func TestActivityTabRevealIsDroppedWhileTheTabsAreNotDrawn(t *testing.T) {
	loaded := feed(t, browsing(t, 160), click("", view.FocusDetailClick)).State
	folded := feed(t, loaded, resize(60)).State
	wantOps(t, step(t, folded, key("tab")))

	failed := feed(t, browsing(t, 160), model.DetailFailed{UID: "alpha", Err: errors.New("down")}, click("", view.FocusDetailClick)).State
	wantOps(t, step(t, failed, key("tab")))
}

func TestColumnsStayMountedWhileHelpIsOpen(t *testing.T) {
	s := feed(t, browsing(t, 160), click("", view.FocusDetailClick), key("?")).State
	if !s.Model.HelpOpen {
		t.Fatal("fixture: help is closed")
	}
	out := step(t, s, click("", view.ActivityTabClick(model.TabEvents)))
	wantOps(t, out, tern.OpReveal(view.DetailActivityID+".empty", "nearest"), tern.OpReveal(view.DetailActivityID+".tabs", "nearest"))
	closed := step(t, out.State, key("escape"))
	if closed.State.Model.HelpOpen {
		t.Fatal("help did not close")
	}
}

func TestUnknownIDOnATransientNodeIsIgnored(t *testing.T) {
	s := browsing(t, 160)
	for _, id := range []string{issueRow(t, s.Model, view.IssueListID, beta), view.SearchID, view.FieldID(model.CardCreate, model.CreateBody), view.FieldID(model.CardClose, model.ClosePR), view.DetailActivityID + ".empty", view.DetailActivityID + ".tabs", view.FieldID(model.CardProject, ""), view.BoardID + ".lane1.cards.beta"} {
		out := driver.Step(s, &tern.ErrorEvent{Msg: "unknown id " + id}, now)
		if out.Err != nil || out.Quit || out.View != nil || len(out.Ops) != 0 {
			t.Errorf("unknown id %q: %+v", id, out)
		}
	}
}

func TestAnyOtherErrorIsFatal(t *testing.T) {
	s := browsing(t, 160)
	for _, msg := range []string{"unknown id " + view.DetailActivityID, "unknown id main.columns", "unknown id " + view.BoardID, "bad op", "unknown id "} {
		out := driver.Step(s, &tern.ErrorEvent{Msg: msg}, now)
		if out.Err == nil || !strings.Contains(out.Err.Error(), msg) {
			t.Errorf("%q: err = %v", msg, out.Err)
		}
	}
}

func TestFreshSurfaceResendsTextFocus(t *testing.T) {
	s := feed(t, browsing(t, 160), key("/")).State
	if out := step(t, s, key("a")); len(out.Ops) != 0 {
		t.Fatalf("fixture: focus sent again: %v", out.Ops)
	}
	wantOps(t, step(t, s.Fresh(), key("a")), tern.OpFocus(view.SearchID))
	// Nothing to focus on a fresh surface: nothing to send.
	wantOps(t, step(t, browsing(t, 160).Fresh(), resize(161)))
}

func TestQuitStateEndsTheLoop(t *testing.T) {
	s := browsing(t, 160)
	out := driver.Step(s, key("q"), now)
	if !out.Quit || out.Err != nil || out.View != nil || len(out.Ops) != 0 || len(out.Cmds) != 0 {
		t.Fatalf("q: %+v", out)
	}
	out = driver.Step(s, key("ctrl+c"), now)
	if !out.Quit {
		t.Fatalf("ctrl+c: %+v", out)
	}
	if out := driver.Step(s, key("j"), now); out.Quit {
		t.Fatal("j quit")
	}
}

func TestInputTheModelIgnoresDrawsAndSendsNothing(t *testing.T) {
	out := driver.Step(browsing(t, 160), &tern.Theme{Dark: true}, now)
	if out.View != nil || len(out.Ops) != 0 || len(out.Cmds) != 0 || out.Err != nil || out.Quit {
		t.Fatalf("theme: %+v", out)
	}
}

func TestDaemonCommandsAreReturnedAndOpsAreNot(t *testing.T) {
	out := step(t, browsing(t, 160), key("j"))
	for _, cmd := range out.Cmds {
		switch cmd.(type) {
		case model.Scroll, model.RevealActivityTab:
			t.Errorf("intent %#v leaked into the commands", cmd)
		}
	}
	if len(out.Cmds) != 1 {
		t.Fatalf("commands = %#v", out.Cmds)
	}
}

func TestDifferentIssueScrollsTheDetailToItsStart(t *testing.T) {
	s := feed(t, browsing(t, 160), key("j")).State
	out := step(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &beta}})
	wantOps(t, out, tern.OpScroll(view.DetailContentID, "start"))
}

func TestMenuAndNewProjectCardTakeTheCaret(t *testing.T) {
	menu := step(t, browsing(t, 160), openMenu)
	wantOps(t, menu, tern.OpFocus(view.FieldID(model.CardPicker, "")))

	form := feed(t, menu.State, key("end"), key("enter"))
	wantOps(t, form, tern.OpFocus(view.FieldID(model.CardProject, "")))
	if !form.State.Model.ProjectForm.Open {
		t.Fatal("fixture: the New Project card is closed")
	}

	saving := step(t, form.State, key("enter"))
	if !saving.State.Model.ProjectForm.Saving {
		t.Fatal("fixture: the New Project card is not saving")
	}
	wantOps(t, saving, tern.OpFocus(""))
}

func TestSearchKeepsItsCaretWhileACollectionLoads(t *testing.T) {
	s := feed(t, browsing(t, 160), key("/")).State
	out := step(t, s, model.CollectionSelected{Collection: s.Model.Collections[4]})
	if !out.State.Model.Loading || out.State.Focus != view.SearchID {
		t.Fatalf("loading=%v focus=%q", out.State.Model.Loading, out.State.Focus)
	}
	wantOps(t, out)
}

// Tern keeps the node of the last reveal in view and ignores the wheel past it, so nothing but a
// tab switch may reveal inside the detail: not a click on the title or a row, a jump, a
// Collection switch, a resize, a key move in the detail, or a live refresh of the shown Issue.
func TestOnlyATabSwitchRevealsInsideTheDetail(t *testing.T) {
	peer := "peer"
	withLink := &model.IssueDetailData{Issue: &alpha, Links: []model.Link{{ID: 1, FromIssueUID: "alpha", ToIssueUID: peer, ToQualifiedID: "work#peer", Type: "related"}}}
	s := feed(t, browsing(t, 160), model.DetailLoaded{UID: "alpha", Selected: withLink}, click("", view.FocusDetailClick)).State
	for name, in := range map[string]any{
		"title click":     click("", view.FocusDetailClick),
		"row click":       click("", view.DetailItemClick(model.TabComments, "x")),
		"jump":            click("", view.JumpClick(peer)),
		"j":               key("j"),
		"k":               key("k"),
		"enter":           key("enter"),
		"escape":          key("escape"),
		"resize":          resize(100),
		"refresh":         model.DetailLoaded{UID: "alpha", Selected: withLink},
		"Collection pick": model.CollectionSelected{Collection: s.Model.Collections[4]},
	} {
		for _, op := range step(t, s, in).Ops {
			if op[0] == "reveal" && strings.HasPrefix(op[1].(string), view.DetailID) {
				t.Errorf("%s: %v", name, op)
			}
		}
	}
}

func TestOneStepKeepsEveryDaemonCommandAndTheEditorRoundTrip(t *testing.T) {
	s := feed(t, browsing(t, 160), key("e")).State
	s.Model.SelectedUID = "beta" // the detail on screen is no longer the selected Issue's, so the step reloads it too
	out := step(t, s, key("ctrl+e"))
	load := model.LoadDetail{Workspace: "/repo", UID: "beta"}
	if !reflect.DeepEqual(out.Cmds, []model.Cmd{load}) {
		t.Errorf("daemon commands = %#v, want %#v", out.Cmds, load)
	}
	if want := (model.OpenDraftEditor{UID: "alpha", Text: alpha.Body}); out.Editor == nil || *out.Editor != want {
		t.Errorf("editor = %#v, want %#v", out.Editor, want)
	}
}
