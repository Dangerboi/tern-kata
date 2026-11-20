package driver_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

const docBody = "# Plan\n\nFirst paragraph.\n\nSecond paragraph."

// onDoc is the Pane on alpha, whose body is docBody, on a Tern that draws a doc (drawn false: one that does not).
func onDoc(t *testing.T, drawn bool) driver.State {
	t.Helper()
	issue := alpha
	issue.Body, issue.ProjectID, issue.Revision, issue.QualifiedID = docBody, 7, 3, "work#aaaa"
	return onDocWith(t, drawn, &model.IssueDetailData{Issue: &issue})
}

// onDocWith is the Pane on alpha, whose detail is detail, on a Tern that draws a doc (drawn false: one that does not).
func onDocWith(t *testing.T, drawn bool, detail *model.IssueDetailData) driver.State {
	t.Helper()
	s := feed(t, browsing(t, 160), model.DetailLoaded{UID: "alpha", Selected: detail}).State
	s.Model.InPlaceBody = drawn
	return feed(t, s, key("j"), key("k")).State // a draw on the new setting
}

// detailReloadedWith feeds the detail of alpha as someone else left it: revision 4 with body.
func detailReloadedWith(t *testing.T, s driver.State, body string) driver.State {
	t.Helper()
	theirs := alpha
	theirs.Body, theirs.ProjectID, theirs.Revision, theirs.QualifiedID = body, 7, 4, "work#aaaa"
	return feed(t, s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &theirs}}).State
}

func bodyNode(t *testing.T, s driver.State) tern.WireNode {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == view.BodyID || view.IsBodyDoc(n.ID) {
			return n
		}
	}
	t.Fatal("the view draws no body")
	return tern.WireNode{}
}

func bodyDocID(t *testing.T, s driver.State) string { return bodyNode(t, s).ID }

// Tern keeps a node while its id stays, and the margin card of a selection comment belongs to the node: each Issue's doc has its own
// id, so another Issue does not take the card over.
func TestEachIssuesDocHasItsOwnNode(t *testing.T) {
	s := onDoc(t, true)
	first := bodyDocID(t, s)
	other := beta
	other.Body, other.ProjectID, other.Revision, other.QualifiedID = docBody, 7, 1, "work#bbbb"
	s = feed(t, s, key("j"), model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &other}}).State
	if second := bodyDocID(t, s); second == first {
		t.Fatalf("alpha's and beta's docs share the id %q", first)
	}
}

func savedCmds(cmds []model.Cmd) []model.SaveDraft {
	var saves []model.SaveDraft
	for _, c := range cmds {
		if save, ok := c.(model.SaveDraft); ok {
			saves = append(saves, save)
		}
	}
	return saves
}

func TestBodyIsADocWhenTernDrawsOne(t *testing.T) {
	n := bodyNode(t, onDoc(t, true))
	if n.K != "doc" || n.P["edit"] != true || n.P["comment"] != true || n.P["text"] != docBody {
		t.Fatalf("body = %s %v, want a doc with edit and comment on holding the body", n.K, n.P)
	}
	if _, has := n.P["comments"]; has {
		t.Fatalf("the margin holds comments %v, want none", n.P["comments"])
	}
}

func TestBodyIsMarkdownOnATernWithoutDoc(t *testing.T) {
	s := onDoc(t, false)
	if n := bodyNode(t, s); n.K != "md" {
		t.Fatalf("body = %s, want md as before", n.K)
	}
	id := view.BodyDocID("alpha")
	if out := feed(t, s, &tern.Change{ID: id, Text: "x"}, &tern.Editing{ID: id, On: false}); len(savedCmds(out.Cmds)) != 0 {
		t.Fatalf("a doc event on a Tern without doc saved %v", out.Cmds)
	}
}

func edited(t *testing.T, s driver.State, text string) (driver.State, driver.Output) {
	t.Helper()
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: text}).State
	out := step(t, s, &tern.Editing{ID: id, On: false})
	return out.State, out
}

func TestLeavingTheDocSavesTheEditAgainstTheRevisionItBeganOn(t *testing.T) {
	s, out := edited(t, onDoc(t, true), docBody+"\n\nThird.")
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 {
		t.Fatalf("cmds %#v, want one SaveDraft", out.Cmds)
	}
	got := saves[0]
	if got.UID != "alpha" || got.Comment || got.Text != docBody+"\n\nThird." || got.BaseText != docBody || got.ProjectID != 7 {
		t.Fatalf("save = %+v", got)
	}
	if s.Model.EditorMode() != "" {
		t.Fatalf("the Edit Issue body overlay opened (mode %q)", s.Model.EditorMode())
	}
	done := feed(t, s, model.DraftSaved{UID: "alpha"}).State
	if _, kept := done.Model.Drafts["alpha"]; kept {
		t.Fatal("the Draft is kept after it saved")
	}
}

// Tern ends an in-place edit on a text op for the doc, and sends the edit while the user still types: the Pane must not send the
// edit back, whether it is held or saving.
func TestDocKeepsTheStoredBodyWhileTheEditIsHeldOrSaving(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "typed so far"}).State
	if n := bodyNode(t, s); n.P["text"] != docBody {
		t.Fatalf("doc text %q while typing, want the stored body", n.P["text"])
	}
	s, _ = edited(t, s, "typed so far")
	if n := bodyNode(t, s); n.P["text"] != docBody {
		t.Fatalf("doc text %q while saving, want the stored body", n.P["text"])
	}
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	if n := bodyNode(t, s); n.P["text"] != "typed so far" {
		t.Fatalf("doc text %q after the save failed, want the kept edit", n.P["text"])
	}
}

func TestLeavingTheDocUnchangedSavesNothing(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	out := feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Editing{ID: id, On: false})
	if len(savedCmds(out.Cmds)) != 0 {
		t.Fatalf("saved %v", out.Cmds)
	}
	out = feed(t, s, &tern.Change{ID: id, Text: docBody}, &tern.Editing{ID: id, On: false})
	if len(savedCmds(out.Cmds)) != 0 {
		t.Fatalf("an edit back to the same text saved %v", out.Cmds)
	}
}

func TestConcurrentEditOpensTheConflictCard(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	theirs := alpha
	theirs.Body, theirs.Revision = "theirs", 4
	s = feed(t, s, model.DraftConflict{UID: "alpha", Issue: theirs}).State
	if s.Model.EditorMode() != model.ModeConflict {
		t.Fatalf("mode %q, want the conflict card", s.Model.EditorMode())
	}
	draft, _ := s.Model.ActiveDraft()
	if draft.Editor.Text != "mine" || draft.Conflict == nil || draft.Conflict.Body != "theirs" {
		t.Fatalf("draft = %q conflict %v", draft.Editor.Text, draft.Conflict)
	}
	// Overwrite saves against the new revision, as on the overlay.
	out := step(t, s, key("o"))
	if saves := savedCmds(out.Cmds); len(saves) != 1 || saves[0].Text != "mine" || saves[0].BaseText != "theirs" {
		t.Fatalf("overwrite sent %#v", out.Cmds)
	}
}

func TestDiscardingTheConflictBringsTheBodyBack(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	theirs := alpha
	theirs.Body, theirs.Revision = "theirs", 4
	s = feed(t, s, model.DraftConflict{UID: "alpha", Issue: theirs}, key("d")).State
	if n := bodyNode(t, s); n.P["text"] != "theirs" {
		t.Fatalf("doc text %v, want the stored body", n.P["text"])
	}
}

func TestFailedSaveKeepsTheEditForTheOverlay(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	if s.Model.EditorMode() != "" {
		t.Fatalf("mode %q: a failure opened the overlay by itself", s.Model.EditorMode())
	}
	s = feed(t, s, key("e")).State
	draft, ok := s.Model.ActiveDraft()
	if !ok || draft.Editor.Text != "mine" {
		t.Fatalf("e opened %q, want the kept edit", draft.Editor.Text)
	}
}

func TestEditKeepsTheParentSectionTheDocHides(t *testing.T) {
	issue := alpha
	issue.Body, issue.ProjectID = "## Parent\n\nChild of #upup.\n\n# Plan\n\ntext", 7
	links := []model.Link{{ID: 1, Type: "parent", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "up", ToQualifiedID: "work#upup"}}
	s := onDocWith(t, true, &model.IssueDetailData{Issue: &issue, Links: links})
	if n := bodyNode(t, s); n.K != "doc" || n.P["text"] != "# Plan\n\ntext" {
		t.Fatalf("body = %s %v, want a doc without the Parent section", n.K, n.P["text"])
	}
	_, out := edited(t, s, "# Plan\n\nmore")
	if saves := savedCmds(out.Cmds); len(saves) != 1 || saves[0].Text != "## Parent\n\nChild of #upup.\n\n# Plan\n\nmore" {
		t.Fatalf("saved %#v", out.Cmds)
	}
}

func TestCommentOnASelectionQuotesItAboveTheNote(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	at := 8
	out := step(t, s, &tern.Comment{ID: id, Quote: "First paragraph.\n\nSecond", At: &at, Note: "merge these"})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 {
		t.Fatalf("cmds %#v, want one SaveDraft", out.Cmds)
	}
	if got, want := saves[0].Text, "> First paragraph.\n>\n> Second\n\nmerge these"; !saves[0].Comment || got != want || saves[0].UID != "alpha" || saves[0].BaseText != docBody {
		t.Fatalf("save = %+v, want comment %q", saves[0], want)
	}
	if out.State.Model.EditorMode() != "" {
		t.Fatalf("the comment overlay opened (mode %q)", out.State.Model.EditorMode())
	}
	if n := bodyNode(t, out.State); n.K != "doc" {
		t.Fatalf("body = %s", n.K)
	}
}

func TestCommentWithoutNoteOrQuoteSendsNothing(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	for _, c := range []*tern.Comment{{ID: id, Quote: "x", Note: "  "}, {ID: id, Quote: "", Note: "n"}, {ID: id, Comment: "c1", Quote: "x", Note: "n"}} {
		if out := step(t, s, c); len(savedCmds(out.Cmds)) != 0 {
			t.Fatalf("%+v sent %v", c, out.Cmds)
		}
	}
}

func TestCommentWhileADraftIsOpenKeepsTheDraft(t *testing.T) {
	s := feed(t, onDoc(t, true), key("e"), key("x")).State
	id := view.BodyDocID("alpha")
	out := step(t, s, &tern.Comment{ID: id, Quote: "q", Note: "n"})
	if len(savedCmds(out.Cmds)) != 0 {
		t.Fatalf("sent %v", out.Cmds)
	}
	if toastText(t, out) == "" {
		t.Fatal("no toast says why")
	}
	if draft, _ := out.State.Model.ActiveDraft(); !strings.Contains(draft.Editor.Text, "x") {
		t.Fatalf("draft %q lost", draft.Editor.Text)
	}
}

func TestTernDrawsDocFromItsVersion(t *testing.T) {
	for version, want := range map[string]bool{"0.9.26": true, "v0.9.26": true, "0.9.27": true, "0.10.0": true, "0.9.26-dev": false, "0.9.25": false, "0.9.9": false, "": false, "nonsense": false} {
		if got := model.TernDrawsDoc(version); got != want {
			t.Errorf("TernDrawsDoc(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestBodyWithAPictureStaysMarkdownOnATernWithDoc(t *testing.T) {
	for body, kind := range map[string]string{
		"a <img src=x.png> b": "md", "<IMG width=40 src=x.png>": "md", "see ![alt](x.png)": "md", "![](http://h/x.png)": "md",
		"no picture": "doc", "a [link](x.png) and ! [x]": "doc",
	} {
		issue := alpha
		issue.Body, issue.ProjectID = body, 7
		s := onDocWith(t, true, &model.IssueDetailData{Issue: &issue})
		if n := bodyNode(t, s); n.K != kind || kind == "md" && n.P["text"] != body {
			t.Errorf("body %q drawn as %s, want %s", body, n.K, kind)
		}
	}
}

func TestDocOffersNoEditOrCommentWhileADraftItCannotTakeIsHeld(t *testing.T) {
	failedComment := func(t *testing.T) driver.State {
		s := onDoc(t, true)
		s = feed(t, s, &tern.Comment{ID: bodyDocID(t, s), Quote: "q", Note: "n"}).State
		return feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	}
	savingEdit := func(t *testing.T) driver.State {
		s, _ := edited(t, onDoc(t, true), "mine")
		return s
	}
	for name, s := range map[string]driver.State{"a failed comment": failedComment(t), "an edit being saved": savingEdit(t)} {
		n := bodyNode(t, s)
		if n.K != "doc" || n.P["edit"] == true || n.P["comment"] == true {
			t.Errorf("%s: doc edit=%v comment=%v, want both off", name, n.P["edit"], n.P["comment"])
		}
	}
	// A Draft that only holds an in-place edit takes the next one.
	s := onDoc(t, true)
	s = feed(t, s, &tern.Editing{ID: bodyDocID(t, s), On: true}, &tern.Change{ID: bodyDocID(t, s), Text: "typing"}).State
	if n := bodyNode(t, s); n.P["edit"] != true || n.P["comment"] != true {
		t.Errorf("an edit in progress: doc edit=%v comment=%v, want both on", n.P["edit"], n.P["comment"])
	}
}

func TestLeavingTheDocAsTheSelectionMovesSavesToTheIssueEditedNotTheOneLoading(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	// The selection moves to beta while the doc still shows alpha, as when an agent closes alpha in a Collection that hides closed Issues.
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	out := step(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].UID != "alpha" || saves[0].Text != "mine" || saves[0].BaseText != docBody {
		t.Fatalf("beta's detail replacing alpha's sent %#v, want alpha's edit saved", out.Cmds)
	}
	// The doc, replaced by beta's, then sends the last second of typing and ends the edit, both with alpha's id: the text is alpha's,
	// so it waits for the save in flight and is saved to alpha, never to beta.
	s = out.State
	s = feed(t, s, &tern.Change{ID: id, Text: "mine, late"}, &tern.Editing{ID: id, On: false}).State
	if _, held := s.Model.Drafts["beta"]; held {
		t.Fatal("a late change wrote beta's Draft")
	}
	out = step(t, s, model.DraftSaved{UID: "alpha"})
	saves = savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].UID != "alpha" || saves[0].Text != "mine, late" || saves[0].BaseText != "mine" {
		t.Fatalf("alpha's first save settling sent %#v, want the late text saved to alpha on the text just saved", out.Cmds)
	}
}

func TestLateChangeWithNoDraftHeldYetIsSavedToTheIssueEdited(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}).State
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	out := step(t, s, &tern.Change{ID: id, Text: "mine"})
	if _, held := out.State.Model.Drafts["beta"]; held {
		t.Fatal("a late change wrote beta's Draft")
	}
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].UID != "alpha" || saves[0].Text != "mine" || saves[0].BaseText != docBody {
		t.Fatalf("a late change sent %#v, want alpha's edit saved", out.Cmds)
	}
	if again := savedCmds(step(t, out.State, &tern.Editing{ID: id, On: false}).Cmds); len(again) != 0 {
		t.Fatalf("editing off sent %#v, want the edit saved once", again)
	}
}

func TestLateChangeAfterTheSaveSettledIsSavedOnTheTextSaved(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	s = feed(t, s, model.DraftSaved{UID: "alpha"}).State
	out := step(t, s, &tern.Change{ID: id, Text: "mine, late"})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].UID != "alpha" || saves[0].Text != "mine, late" || saves[0].BaseText != "mine" {
		t.Fatalf("a late change after the save settled sent %#v, want it saved to alpha on the text saved", out.Cmds)
	}
}

func TestLateChangeIsKeptWhenTheSaveInFlightFails(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	s = feed(t, s, &tern.Change{ID: id, Text: "mine, late"}, &tern.Editing{ID: id, On: false}).State
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	if draft := s.Model.Drafts["alpha"]; draft.Editor.Text != "mine, late" {
		t.Fatalf("alpha's kept Draft holds %q, want the late text", draft.Editor.Text)
	}
}

func TestEditBegunWhileTheSelectionIsLoadingBelongsToTheIssueDrawn(t *testing.T) {
	s := feed(t, onDoc(t, true), key("j")).State
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	out := step(t, s, &tern.Editing{ID: id, On: false})
	if saves := savedCmds(out.Cmds); len(saves) != 1 || saves[0].UID != "alpha" || saves[0].Text != "mine" {
		t.Fatalf("leaving the doc sent %#v, want alpha's edit saved", out.Cmds)
	}
}

func TestFailedInPlaceSaveToastsAfterTheUserLeftTheIssue(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	s = feed(t, s, key("j")).State
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	if n := s.Model.Notice; !strings.Contains(n, "work#aaaa") || !strings.Contains(n, errTest.Error()) {
		t.Fatalf("notice %q, want it to name the Issue and the error", n)
	}
}

func TestEditAfterAFailedSaveKeepsTheDocTextSteady(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine, and more"}).State
	if n := bodyNode(t, s); n.P["text"] != "mine" {
		t.Fatalf("doc text %q while editing, want what it drew when the edit began", n.P["text"])
	}
}

func TestTheDocKeepsShowingTheDraftWhileARetrySaves(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	if n := bodyNode(t, s); n.P["text"] != "mine" {
		t.Fatalf("doc text %q after the failure, want the Draft", n.P["text"])
	}
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine, and more"}).State
	s = feed(t, s, &tern.Editing{ID: id, On: false}).State // the retry is in flight
	if n := bodyNode(t, s); n.P["text"] != "mine, and more" {
		t.Fatalf("doc text %q while the retry saves, want the Draft's text, which the doc already holds", n.P["text"])
	}
	s = feed(t, s, model.DraftSaved{UID: "alpha"}).State
	if n := bodyNode(t, s); n.P["text"] != "mine, and more" {
		t.Fatalf("doc text %q after the save, want the saved text", n.P["text"])
	}
}

func TestTheDocKeepsShowingTheDraftWhileAnOverwriteSaves(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	theirs := alpha
	theirs.Body, theirs.Revision = "theirs", 4
	s = feed(t, s, model.DraftConflict{UID: "alpha", Issue: theirs}).State
	if n := bodyNode(t, s); n.P["text"] != "mine" {
		t.Fatalf("doc text %q during the conflict, want the Draft", n.P["text"])
	}
	s = step(t, s, key("o")).State // the overwrite is in flight
	if n := bodyNode(t, s); n.P["text"] != "mine" {
		t.Fatalf("doc text %q while the overwrite saves, want the Draft", n.P["text"])
	}
}

func TestBodyChangedByOthersAfterTheEditBeganStillConflicts(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}).State
	s = detailReloadedWith(t, s, "theirs")
	s = feed(t, s, &tern.Change{ID: id, Text: docBody + "\n\nThird."}).State
	out := step(t, s, &tern.Editing{ID: id, On: false})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].BaseText != docBody {
		t.Fatalf("save %#v, want one against the body the edit began on", out.Cmds)
	}
}

func TestSaveRefusedWhileReconnectingRaisesTheNoticeNamingTheIssue(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	s.Model.Stale = true
	s = feed(t, s, &tern.Editing{ID: id, On: false}).State
	if n := s.Model.Notice; !strings.Contains(n, "work#aaaa") || !strings.Contains(n, "disconnected from kata") {
		t.Fatalf("notice %q, want it to name the Issue and say kata is disconnected", n)
	}
	if n := s.Model.Notice; strings.Count(n, "kept") != 1 {
		t.Fatalf("notice %q says the Draft is kept twice", n)
	}
}

func TestEditingOnDropsWhatAnEarlierEditLeftBehind(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}).State // the edit ends without an editing off
	s = detailReloadedWith(t, s, "theirs")
	s = feed(t, s, &tern.Editing{ID: id, On: true}).State
	if n := bodyNode(t, s); n.P["text"] != "theirs" {
		t.Fatalf("doc text %q, want the body as it is when the new edit began", n.P["text"])
	}
}

func bodyErrorLine(t *testing.T, s driver.State) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasSuffix(n.ID, ".content.body-error") {
			return spansJoined(n)
		}
	}
	t.Fatal("the view draws no line under the doc")
	return ""
}

func TestTheLineUnderTheDocSaysTheDraftIsKeptOnce(t *testing.T) {
	for _, c := range []struct {
		name  string
		stale bool // kata is reconnecting: the leave is refused, with no failed save
	}{{"reconnecting", true}, {"another failure", false}} {
		s := onDoc(t, true)
		id := bodyDocID(t, s)
		s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
		if c.stale {
			s.Model.Stale = true
			s = feed(t, s, &tern.Editing{ID: id, On: false}).State
		} else {
			s = feed(t, s, &tern.Editing{ID: id, On: false}).State
			s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
		}
		line := bodyErrorLine(t, s)
		if strings.Count(line, "kept") != 1 || !strings.Contains(line, "press E or C to open it") && !strings.Contains(line, "Press E or C to open it") {
			t.Fatalf("%s: line %q, want it to say the Draft is kept once and how to open it", c.name, line)
		}
		if strings.Contains(line, "kept The") {
			t.Fatalf("%s: line %q runs two sentences together", c.name, line)
		}
	}
}

func TestTheNoticeSaysTheDraftIsKeptOnceWhateverTheError(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}, &tern.Editing{ID: id, On: false}).State
	s = feed(t, s, key("j"), model.DraftFailed{UID: "alpha", Err: errTest}).State
	if n := s.Model.Notice; strings.Count(n, "kept") != 1 || !strings.Contains(n, "daemon said no") {
		t.Fatalf("notice %q, want the error and the Draft kept once", n)
	}
}

// leftAndClickedAgain edits the Issue body to "mine", leaves the doc (the save is in flight) and double clicks it again before the
// doc draws read-only; the second double click is ignored.
func leftAndClickedAgain(t *testing.T) (driver.State, string) {
	t.Helper()
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	s = feed(t, s, &tern.Editing{ID: id, On: false}).State
	s = feed(t, s, &tern.Editing{ID: id, On: true}).State
	return s, id
}

func TestADoubleClickBeforeTheReadOnlyDrawTakesNoStaleBase(t *testing.T) {
	s, id := leftAndClickedAgain(t)
	s = feed(t, s, model.DraftSaved{UID: "alpha"}).State
	s = feed(t, s, &tern.Change{ID: id, Text: "mine more"}).State
	out := step(t, s, &tern.Editing{ID: id, On: false})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].BaseText != "mine" {
		t.Fatalf("save %#v, want one against the body the first save wrote", out.Cmds)
	}
}

func TestAnIgnoredEditingOnDoesNotSendTheFailedDraftAgain(t *testing.T) {
	s, id := leftAndClickedAgain(t)
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	out := step(t, s, &tern.Editing{ID: id, On: false}) // the edit Tern kept open ends when the doc shows the kept Draft
	if saves := savedCmds(out.Cmds); len(saves) != 0 {
		t.Fatalf("saves %#v, want none: nothing was edited since the failure", saves)
	}
	if line := bodyErrorLine(t, out.State); !strings.Contains(line, "daemon said no") {
		t.Fatalf("line %q, want the failure still shown", line)
	}
}

func TestAChangeAfterAnIgnoredEditingOnKeepsTheEditAsTheDraft(t *testing.T) {
	s, id := leftAndClickedAgain(t)
	s = feed(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	s = feed(t, s, &tern.Change{ID: id, Text: "mine more"}).State
	if d := s.Model.Drafts["alpha"]; !strings.HasSuffix(d.DocText(), "mine more") || d.Err != nil {
		t.Fatalf("draft %q (%v), want the edit kept as the Draft", d.DocText(), d.Err)
	}
	saves := savedCmds(step(t, s, &tern.Editing{ID: id, On: false}).Cmds)
	if len(saves) != 1 || !strings.HasSuffix(saves[0].Text, "mine more") {
		t.Fatalf("saves %#v, want the edit sent once, on leaving", saves)
	}
}

func TestConflictWhileAnotherIssuesCardIsOpenRaisesTheNoticeNamingTheIssueAndLeavesALineUnderItsBody(t *testing.T) {
	s, _ := edited(t, onDoc(t, true), "mine")
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}, key("e")).State
	if s.Model.EditorUID != "beta" {
		t.Fatalf("editor on %q, want beta's card open", s.Model.EditorUID)
	}
	theirs := alpha
	theirs.Body, theirs.Revision = "theirs", 4
	s = feed(t, s, model.DraftConflict{UID: "alpha", Issue: theirs}).State
	if n := s.Model.Notice; !strings.Contains(n, "work#aaaa") || !strings.Contains(n, "conflict") || !strings.Contains(n, "Press "+model.Key("e")+" or "+model.Key("c")+" on that Issue to open the conflict card.") {
		t.Fatalf("notice %q, want it to name the Issue and say it conflicts", n)
	}
	if s.Model.EditorUID != "beta" {
		t.Fatalf("editor on %q, want beta's card left open", s.Model.EditorUID)
	}
	s = feed(t, s, key("x")).State // the next key clears the Notice
	if s.Model.Notice != "" {
		t.Fatalf("notice %q, want it cleared by the key", s.Model.Notice)
	}
	s = feed(t, s, key("escape"), key("y"), key("escape"), key("k")).State // beta's card is discarded, and the selection goes back to alpha
	back := alpha
	back.Body, back.ProjectID, back.Revision, back.QualifiedID = docBody, 7, 3, "work#aaaa"
	s = feed(t, s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &back}}).State
	if line := bodyErrorLine(t, s); !strings.Contains(line, "conflicts with a newer change") || !strings.Contains(line, "Press E or C to open the conflict card.") {
		t.Fatalf("line %q, want it to say the edit conflicts and how to open the card", line)
	}
	s = feed(t, s, key("e")).State
	if s.Model.EditorUID != "alpha" || s.Model.EditorMode() != model.ModeConflict {
		t.Fatalf("e opened %q in mode %q, want alpha's conflict card", s.Model.EditorUID, s.Model.EditorMode())
	}
}

func TestAHeldDraftStaysExplainedWhateverTheShownBodyHolds(t *testing.T) {
	for _, body := range []string{"", "![shot](shot.png)"} {
		for _, c := range []struct {
			name string
			msg  any
			want string
		}{
			{"conflict", model.DraftConflict{UID: "alpha", Issue: alpha}, "conflicts with a newer change"},
			{"failure", model.DraftFailed{UID: "alpha", Err: errTest}, "daemon said no"},
		} {
			s, _ := edited(t, onDoc(t, true), "mine")
			s = feed(t, s, key("j")).State
			loaded := beta
			loaded.Body, loaded.ProjectID = "beta body", 7
			s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}, key("e"), c.msg).State
			s = feed(t, s, key("x"), key("escape"), key("y"), key("escape"), key("k")).State
			s = detailReloadedWith(t, s, body)
			if line := bodyErrorLine(t, s); !strings.Contains(line, c.want) {
				t.Fatalf("%s, body %q: line %q, want it to say %q", c.name, body, line, c.want)
			}
		}
	}
}

func TestTheLineUnderTheBodyNamesTheCommentItKeepsAndTheNoticeSaysTheSame(t *testing.T) {
	s := onDoc(t, true)
	s = feed(t, s, &tern.Comment{ID: bodyDocID(t, s), Quote: "q", Note: "n"}, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}, key("e")).State
	theirs := alpha
	theirs.Body, theirs.Revision = "theirs", 4
	s = feed(t, s, model.DraftConflict{UID: "alpha", Issue: theirs}).State
	if n := s.Model.Notice; !strings.Contains(n, "Cannot save your comment on work#aaaa: it conflicts") {
		t.Fatalf("notice %q, want it to name the comment", n)
	}
	s = feed(t, s, key("x"), key("escape"), key("y"), key("escape"), key("k")).State
	back := alpha
	back.Body, back.ProjectID, back.Revision, back.QualifiedID = docBody, 7, 3, "work#aaaa"
	s = feed(t, s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &back}}).State
	if line := bodyErrorLine(t, s); !strings.Contains(line, "Cannot save your comment: it conflicts with a newer change.") {
		t.Fatalf("line %q, want it to name the comment", line)
	}
}

func TestCommentSentAfterTheSelectionMovedIsWrittenOnTheIssueItWasMadeOn(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	out := step(t, s, &tern.Comment{ID: id, Quote: "First paragraph.", Note: "merge these"})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].UID != "alpha" || !saves[0].Comment {
		t.Fatalf("cmds %#v, want a comment on alpha", out.Cmds)
	}
	if _, held := out.State.Model.Drafts["beta"]; held {
		t.Fatal("the comment became beta's Draft")
	}
}

func reopened(cmds []model.Cmd) bool {
	for _, c := range cmds {
		if _, ok := c.(model.ReopenIssueCmd); ok {
			return true
		}
	}
	return false
}

// editSaved is a whole edit session of the doc, "mine" typed and saved, that ended before now: the doc's text has changed since.
func editSaved(t *testing.T, s driver.State) driver.State {
	t.Helper()
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now.Add(-10*time.Second)).State
	s = driver.Step(s, &tern.Change{ID: id, Text: "mine"}, now.Add(-10*time.Second+10*time.Millisecond)).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(-10*time.Second+20*time.Millisecond)).State
	return driver.Step(s, model.DraftSaved{UID: "alpha"}, now.Add(-10*time.Second+30*time.Millisecond)).State
}

// frameThenOpen is the Pane sending a frame (someone else changed the body, so the doc draws other text) 20 ms before Tern's editing
// on, a frame Tern may take as sent after it, and returns the state at the editing on, at now, and the id of the doc.
func frameThenOpen(t *testing.T, s driver.State) (driver.State, string) {
	t.Helper()
	theirs := alpha
	theirs.Body, theirs.ProjectID, theirs.Revision, theirs.QualifiedID = "someone else's text", 7, 4, "work#aaaa"
	s = driver.Step(s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &theirs}}, now.Add(-20*time.Millisecond)).State
	id := bodyDocID(t, s)
	return driver.Step(s, &tern.Editing{ID: id, On: true}, now).State, id
}

// deadEdit is a double click after a frame: Tern closes the edit at once, with no change.
func deadEdit(t *testing.T, s driver.State) driver.State {
	t.Helper()
	s, id := frameThenOpen(t, editSaved(t, s))
	return driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(40*time.Millisecond)).State
}

func TestNoKeyGuardWhenNoFrameWentOutAroundTheEditingOn(t *testing.T) {
	// A quick double click and esc with no frame between is a real edit session that changed nothing.
	s := editSaved(t, onDoc(t, true))
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now.Add(time.Second)).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(time.Second+40*time.Millisecond)).State
	if n := s.Model.Notice; strings.Contains(n, "Cannot open the body for editing") {
		t.Fatalf("notice %q after a double click and esc that no frame came between", n)
	}
	if out := driver.Step(s, key("r"), now.Add(time.Second+100*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after a double click and esc sent %#v, want the reopen", out.Cmds)
	}
}

func TestKeyGuardWhenAFrameWentOutJustBeforeTheEditingOn(t *testing.T) {
	s, id := frameThenOpen(t, onDoc(t, true))
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(40*time.Millisecond)).State
	for _, k := range []string{"r", "u"} {
		if out := driver.Step(s, key(k), now.Add(100*time.Millisecond)); len(out.Cmds) != 0 || !strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
			t.Fatalf("%s after the dead edit sent %#v with notice %q, want it swallowed", k, out.Cmds, out.State.Model.Notice)
		}
	}
}

// Tern ends an edit that a frame follows, so the Pane sends none while the edit is open: the model takes what comes (a daemon result,
// the clock moving) and the editing off draws it.
func TestNoFrameWhileTheDocEditIsOpen(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
	if out := driver.Step(s, model.DraftSaved{UID: "none"}, now.Add(time.Second)); out.View != nil {
		t.Fatal("a step with the clock moved on drew while the edit was open")
	}
	theirs := alpha
	theirs.Body, theirs.ProjectID, theirs.Revision, theirs.QualifiedID = "someone else's text", 7, 4, "work#aaaa"
	out := driver.Step(s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &theirs}}, now.Add(5*time.Second))
	if out.View != nil {
		t.Fatal("a daemon result drew while the edit was open")
	}
	s = driver.Step(out.State, &tern.Change{ID: id, Text: "mine"}, now.Add(6*time.Second)).State
	if out = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(7*time.Second)); out.View == nil {
		t.Fatal("the editing off did not draw")
	}
}

// Tern closes an edit at once when a frame was already on its way. The user types on, so keys that would run a binding are swallowed
// for a moment, with a Notice that says why.
func TestKeysAfterADeadEditRunNoBindingUntilTheWindowEnds(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	at := now.Add(100 * time.Millisecond)
	for _, k := range []string{"r", "u", "A", "enter"} {
		out := driver.Step(s, key(k), at)
		if len(out.Cmds) != 0 {
			t.Fatalf("%s ran a binding: %#v", k, out.Cmds)
		}
		if n := out.State.Model.Notice; !strings.Contains(n, "Cannot open the body for editing") {
			t.Fatalf("%s: notice %q, want it to say the body did not open", k, n)
		}
	}
	if out := driver.Step(s, key("escape"), at); strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
		t.Fatal("escape was swallowed")
	}
	if out := driver.Step(s, key("r"), now.Add(3*time.Second)); !reopened(out.Cmds) {
		t.Fatalf("after the window r sent %#v, want the reopen", out.Cmds)
	}
}

func TestAgentStatesRelayDoesNotEndTheKeyGuard(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	s = driver.Step(s, &tern.Action{ID: "relay", Act: string(model.AgentStatesAction), Value: "[]"}, now.Add(100*time.Millisecond)).State
	out := driver.Step(s, key("r"), now.Add(200*time.Millisecond))
	if len(out.Cmds) != 0 {
		t.Fatalf("r after an agent-states relay sent %#v, want it swallowed", out.Cmds)
	}
	if !strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
		t.Fatalf("notice %q, want the guard's", out.State.Model.Notice)
	}
}

func TestAButtonPressEndsTheKeyGuard(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	s = driver.Step(s, click("legend.chip", view.LegendClick()), now.Add(100*time.Millisecond)).State
	if out := driver.Step(s, key("r"), now.Add(200*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after a press sent %#v, want the reopen", out.Cmds)
	}
}

func TestKeysAreSwallowedOnTheMenuAfterADeadEdit(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	s = driver.Step(s, model.FocusColumnMsg{Column: model.ColumnCollections}, now.Add(100*time.Millisecond)).State
	if mode := s.Model.CurrentMode(); mode != model.ModeCollections {
		t.Fatalf("mode %q after focusing the Menu, want the Menu", mode)
	}
	for _, k := range []string{"b", "enter", "P"} {
		out := driver.Step(s, key(k), now.Add(200*time.Millisecond))
		if len(out.Cmds) != 0 || out.State.Model.CurrentMode() != model.ModeCollections {
			t.Fatalf("%s on the Menu ran a binding: mode %q, %#v", k, out.State.Model.CurrentMode(), out.Cmds)
		}
		if !strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
			t.Fatalf("%s: notice %q, want the guard's", k, out.State.Model.Notice)
		}
	}
	if out := driver.Step(s, key("P"), now.Add(3*time.Second)); out.State.Model.CurrentMode() != model.ModeCollections || out.State.Model.Notice != "" {
		t.Fatalf("P after the window: mode %q, notice %q", out.State.Model.CurrentMode(), out.State.Model.Notice)
	}
}

func TestAPointerActionEndsTheKeyGuard(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	s = driver.Step(s, &tern.Select{ID: "main.columns.list.issues", Item: "alpha"}, now.Add(100*time.Millisecond)).State
	if out := driver.Step(s, key("r"), now.Add(200*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after a click sent %#v, want the reopen", out.Cmds)
	}
}

func TestAPointerChangeEndsTheKeyGuard(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	s = driver.Step(s, &tern.Change{ID: "some.card.dropdown", Value: "closed"}, now.Add(100*time.Millisecond)).State
	if out := driver.Step(s, key("r"), now.Add(200*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after a dropdown or switch changed sent %#v, want the reopen", out.Cmds)
	}
}

func TestAnEditOfTheBodyEndsTheKeyGuard(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now.Add(100*time.Millisecond)).State
	s = driver.Step(s, &tern.Change{ID: id, Text: "mine again"}, now.Add(300*time.Millisecond)).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(400*time.Millisecond)).State
	s = driver.Step(s, model.DraftSaved{UID: "alpha"}, now.Add(500*time.Millisecond)).State
	if out := driver.Step(s, key("r"), now.Add(600*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after an edit that came inside the window sent %#v, want the reopen", out.Cmds)
	}
}

func TestAnEditingOnThatLastsPastTheBoundEndsTheKeyGuard(t *testing.T) {
	s := deadEdit(t, onDoc(t, true))
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now.Add(100*time.Millisecond)).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(900*time.Millisecond)).State
	if out := driver.Step(s, key("r"), now.Add(1000*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after an edit that stayed open past the bound sent %#v, want the reopen", out.Cmds)
	}
}

func TestTheKeyGuardNoticeGoesWhenTheWindowEndsWithNoInput(t *testing.T) {
	s, id := frameThenOpen(t, editSaved(t, onDoc(t, true)))
	out := driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(40*time.Millisecond))
	var wait *model.WaitDocGuard
	for _, c := range out.Cmds {
		if w, ok := c.(model.WaitDocGuard); ok {
			wait = &w
		}
	}
	if wait == nil {
		t.Fatalf("the dead edit asked for no wait: %#v", out.Cmds)
	}
	if !strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
		t.Fatalf("notice %q, want the guard's", out.State.Model.Notice)
	}
	ended := driver.Step(out.State, model.DocGuardExpired(*wait), now.Add(2*time.Second+40*time.Millisecond)).State
	if n := ended.Model.Notice; n != "" {
		t.Fatalf("notice %q after the window ended, want none", n)
	}
}

func TestAnotherNoticeSurvivesTheEndOfTheKeyGuard(t *testing.T) {
	s, id := frameThenOpen(t, editSaved(t, onDoc(t, true)))
	out := driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(40*time.Millisecond))
	var wait model.WaitDocGuard
	for _, c := range out.Cmds {
		if w, ok := c.(model.WaitDocGuard); ok {
			wait = w
		}
	}
	state := out.State
	state.Model.Notice = "Another notice."
	ended := driver.Step(state, model.DocGuardExpired(wait), now.Add(3*time.Second)).State
	if n := ended.Model.Notice; n != "Another notice." {
		t.Fatalf("notice %q, want the other notice kept", n)
	}
}

func TestKeysTypedIntoAnOpenInputAreNotSwallowed(t *testing.T) {
	s := editSaved(t, onDoc(t, true))
	s = driver.Step(s, key("/"), now.Add(-5*time.Second)).State
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(40*time.Millisecond)).State
	out := driver.Step(s, key("x"), now.Add(100*time.Millisecond))
	if out.State.Model.Filters.Search != "x" {
		t.Fatalf("search holds %q, want the typed x", out.State.Model.Filters.Search)
	}
	if strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
		t.Fatalf("notice %q, the key was swallowed", out.State.Model.Notice)
	}
}

func TestNoKeyGuardAfterANormalEdit(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
	s = driver.Step(s, &tern.Change{ID: id, Text: "mine"}, now.Add(10*time.Millisecond)).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(20*time.Millisecond)).State
	s = driver.Step(s, model.DraftSaved{UID: "alpha"}, now.Add(30*time.Millisecond)).State
	if out := driver.Step(s, key("r"), now.Add(40*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after an edit sent %#v, want the reopen", out.Cmds)
	}
	// An edit open longer than the bound that ends with no change is not a dead one either.
	s = onDoc(t, true)
	id = bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
	s = driver.Step(s, &tern.Editing{ID: id, On: false}, now.Add(2*time.Second)).State
	if out := driver.Step(s, key("r"), now.Add(2100*time.Millisecond)); !reopened(out.Cmds) {
		t.Fatalf("r after a long edit sent %#v, want the reopen", out.Cmds)
	}
}

// lateWhileSaving is alpha's edit "mine" in flight (the selection moved to beta), then the doc's last second of typing for alpha, with
// alpha selected and shown again.
func lateWhileSaving(t *testing.T) driver.State {
	t.Helper()
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	s = feed(t, s, &tern.Change{ID: id, Text: "mine, late"}).State
	s = feed(t, s, key("k")).State
	back := alpha
	back.Body, back.ProjectID, back.Revision, back.QualifiedID = docBody, 7, 3, "work#aaaa"
	return feed(t, s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &back}}).State
}

func TestLateTextIsSavedOnceWhenTheUserOpenedTheCardBeforeTheSaveSettled(t *testing.T) {
	s := lateWhileSaving(t)
	s = feed(t, s, key("e")).State
	out := step(t, s, model.DraftSaved{UID: "alpha"})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].Text != "mine, late" || saves[0].BaseText != "mine" {
		t.Fatalf("the first save settling sent %#v, want the late text saved once on the text saved", out.Cmds)
	}
	// A later edit of alpha is not overwritten by the late text.
	s = out.State
	s = feed(t, s, model.DraftSaved{UID: "alpha"}).State
	back := alpha
	back.Body, back.ProjectID, back.Revision, back.QualifiedID = "mine, late", 7, 5, "work#aaaa"
	s = feed(t, s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &back}}).State
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "newer"}).State
	out = step(t, s, &tern.Editing{ID: id, On: false})
	if saves := savedCmds(out.Cmds); len(saves) != 1 || saves[0].Text != "newer" || saves[0].BaseText != "mine, late" {
		t.Fatalf("the newer edit sent %#v", out.Cmds)
	}
	out = step(t, out.State, model.DraftSaved{UID: "alpha"})
	if saves := savedCmds(out.Cmds); len(saves) != 0 {
		t.Fatalf("the newer edit saved, then %#v was sent: stale late text", saves)
	}
}

func TestLateTextIsDroppedWithTheDraftsOnADaemonSwitch(t *testing.T) {
	s := lateWhileSaving(t)
	s = switchedAnswer(t, s).State
	if out := step(t, s, model.DraftSaved{UID: "alpha"}); len(savedCmds(out.Cmds)) != 0 {
		t.Fatalf("a save settling after the switch sent %#v", out.Cmds)
	}
}

func TestLateChangeAfterADaemonSwitchSendsNoText(t *testing.T) {
	s := lateWhileSaving(t)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: false}).State
	s = switchedAnswer(t, s).State
	out := step(t, s, &tern.Change{ID: id, Text: "mine, later"})
	if saves := savedCmds(out.Cmds); len(saves) != 0 {
		t.Fatalf("a change after the switch sent %#v, want the old daemon's text dropped", saves)
	}
	if len(out.State.Model.Drafts) != 0 {
		t.Fatalf("a change after the switch kept Drafts %#v", out.State.Model.Drafts)
	}
}

func TestChangesAfterADaemonSwitchWithTheEditOpenSendNoText(t *testing.T) {
	for name, endEdit := range map[string]bool{"edit still open": false, "editing off after the switch": true} {
		t.Run(name, func(t *testing.T) {
			s := onDoc(t, true)
			id := bodyDocID(t, s)
			s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
			s = switchedAnswer(t, s).State
			if endEdit {
				out := step(t, s, &tern.Editing{ID: id, On: false})
				if saves := savedCmds(out.Cmds); len(saves) != 0 {
					t.Fatalf("editing off after the switch sent %#v", saves)
				}
				s = out.State
			}
			out := step(t, s, &tern.Change{ID: id, Text: "mine, later"})
			if saves := savedCmds(out.Cmds); len(saves) != 0 {
				t.Fatalf("a change after the switch sent %#v, want the old daemon's text dropped", saves)
			}
			if len(out.State.Model.Drafts) != 0 {
				t.Fatalf("a change after the switch kept Drafts %#v", out.State.Model.Drafts)
			}
		})
	}
}

// lateWithTheEditEnded is lateWhileSaving after Tern ended the edit (editing off), so no edit is open in the Pane when the save settles.
func lateWithTheEditEnded(t *testing.T) driver.State {
	t.Helper()
	s := lateWhileSaving(t)
	return feed(t, s, &tern.Editing{ID: bodyDocID(t, s), On: false}).State
}

func TestFailureOfTheFollowUpSaveIsAnnouncedWhetherOrNotTheCardWasOpen(t *testing.T) {
	cases := map[string]struct {
		start func(*testing.T) driver.State
		open  bool
	}{
		"edit open, card open":      {lateWhileSaving, true},
		"edit open, card not open":  {lateWhileSaving, false},
		"edit ended, card open":     {lateWithTheEditEnded, true},
		"edit ended, card not open": {lateWithTheEditEnded, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := c.start(t)
			if c.open {
				s = feed(t, s, key("e")).State
			}
			out := step(t, s, model.DraftSaved{UID: "alpha"})
			if len(savedCmds(out.Cmds)) != 1 {
				t.Fatalf("the first save settling sent %#v, want the follow-up save", out.Cmds)
			}
			if out.State.Model.EditorUID != "" {
				t.Fatalf("the card is still open for %q after its save landed", out.State.Model.EditorUID)
			}
			failed := step(t, out.State, model.DraftFailed{UID: "alpha", Err: errTest}).State
			if n := failed.Model.Notice; !strings.Contains(n, "work#aaaa") || !strings.Contains(n, errTest.Error()) {
				t.Fatalf("notice %q, want it to name the Issue and the error", n)
			}
			theirs := alpha
			theirs.Body, theirs.Revision = "theirs", 9
			conflict := step(t, out.State, model.DraftConflict{UID: "alpha", Issue: theirs}).State
			if conflict.Model.EditorUID != "alpha" {
				t.Fatalf("the conflict opened no card, editor %q", conflict.Model.EditorUID)
			}
		})
	}
}

// The detail of alpha stays on screen while the selection loads beta's, and alpha has no row in the list: the Issue is known only by
// the detail.
func TestFollowUpSaveNamesTheIssueWhenTheListHoldsNoRowForIt(t *testing.T) {
	s := lateWithTheEditEnded(t)
	s = feed(t, s, key("e")).State
	s = feed(t, s, &tern.Select{ID: "main.columns.list.issues", Item: "beta"}).State
	if s.Model.DetailIssueUID() != "alpha" || s.Model.SelectedUID != "beta" {
		t.Fatalf("detail %q, selection %q, want alpha's detail still shown while beta's loads", s.Model.DetailIssueUID(), s.Model.SelectedUID)
	}
	s.Model.Issues = slices.DeleteFunc(slices.Clone(s.Model.Issues), func(i model.Issue) bool { return i.UID == "alpha" })
	out := step(t, s, model.DraftSaved{UID: "alpha"})
	if len(savedCmds(out.Cmds)) != 1 {
		t.Fatalf("the first save settling sent %#v, want the follow-up save", out.Cmds)
	}
	failed := step(t, out.State, model.DraftFailed{UID: "alpha", Err: errTest}).State
	if n := failed.Model.Notice; !strings.Contains(n, "work#aaaa") {
		t.Fatalf("notice %q, want it to name the Issue", n)
	}
}

// crlfParentDetail is alpha whose body opens with a Parent section, with CRLF line breaks, as kata stores a body typed on Windows.
func crlfParentDetail() *model.IssueDetailData {
	issue := alpha
	issue.Body, issue.ProjectID, issue.Revision, issue.QualifiedID = "## Parent\r\n\r\nChild of #upup.\r\n\r\n# Plan\r\n\r\ntext", 7, 3, "work#aaaa"
	links := []model.Link{{ID: 1, Type: "parent", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "up", ToQualifiedID: "work#upup"}}
	return &model.IssueDetailData{Issue: &issue, Links: links}
}

// lateOnTheParentedIssue is lateWhileSaving for crlfParentDetail, ending with the card open on alpha.
func lateOnTheParentedIssue(t *testing.T) (driver.State, string) {
	t.Helper()
	s := onDocWith(t, true, crlfParentDetail())
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}).State
	s = feed(t, s, key("j")).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	s = feed(t, s, &tern.Change{ID: id, Text: "mine, late"}).State
	s = feed(t, s, key("k")).State
	s = feed(t, s, model.DetailLoaded{UID: "alpha", Selected: crlfParentDetail()}).State
	return feed(t, s, key("e")).State, id
}

func TestFollowUpSaveOfACRLFBodyWithAParentSectionDrawsItOnce(t *testing.T) {
	s, id := lateOnTheParentedIssue(t)
	out := step(t, s, model.DraftSaved{UID: "alpha"})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 {
		t.Fatalf("the first save settling sent %#v, want the follow-up save", out.Cmds)
	}
	if n := strings.Count(saves[0].Text, "## Parent"); n != 1 {
		t.Fatalf("the follow-up save holds the Parent section %d times: %q", n, saves[0].Text)
	}
	if n := strings.Count(out.State.Model.Drafts["alpha"].Editor.Text, "## Parent"); n != 1 {
		t.Fatalf("the saved Draft holds the Parent section %d times: %q", n, out.State.Model.Drafts["alpha"].Editor.Text)
	}
	s = step(t, out.State, model.DraftFailed{UID: "alpha", Err: errTest}).State
	s = feed(t, s, &tern.Editing{ID: id, On: false}).State // Tern's end of the edit, which comes after the failure
	if n := bodyNode(t, s); n.P["text"] != "mine, late" {
		t.Fatalf("doc text %q after the follow-up failed, want the edit without the Parent section", n.P["text"])
	}
}

func TestFollowUpSaveHidesTheParentSectionOfItsOwnIssueWhateverDetailIsShown(t *testing.T) {
	s, id := lateOnTheParentedIssue(t)
	s = feed(t, s, &tern.Editing{ID: id, On: false}).State
	loaded := beta
	loaded.Body, loaded.ProjectID = "beta body", 7
	s = feed(t, s, &tern.Select{ID: "main.columns.list.issues", Item: "beta"}, model.DetailLoaded{UID: "beta", Selected: &model.IssueDetailData{Issue: &loaded}}).State
	if s.Model.DetailIssueUID() != "beta" {
		t.Fatalf("the detail shows %q, want beta", s.Model.DetailIssueUID())
	}
	s = step(t, s, model.DraftSaved{UID: "alpha"}).State
	s = step(t, s, model.DraftFailed{UID: "alpha", Err: errTest}).State
	draft, held := s.Model.Drafts["alpha"]
	if !held || draft.Doc == nil {
		t.Fatalf("alpha holds %#v, want its doc Draft", draft)
	}
	if text := draft.DocText(); text != "mine, late" {
		t.Fatalf("the doc draws %q for alpha's Draft, want the edit without the Parent section", text)
	}
}

func TestLateChangeAfterTheCardsSaveSettledOpensTheConflictCardAgainstWhatItSaved(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}, &tern.Editing{ID: id, On: false}).State
	s = feed(t, s, key("e")).State // the card takes the Draft while its save is in flight
	s = feed(t, s, model.DraftSaved{UID: "alpha"}).State
	out := step(t, s, &tern.Change{ID: id, Text: "mine, late"})
	saves := savedCmds(out.Cmds)
	if len(saves) != 1 || saves[0].UID != "alpha" || saves[0].Text != "mine, late" || saves[0].BaseText != docBody {
		t.Fatalf("a late change after the card's save settled sent %#v, want it saved on the text the edit began on", out.Cmds)
	}
	theirs := alpha
	theirs.Body, theirs.Revision = "mine", 4
	conflict := step(t, out.State, model.DraftConflict{UID: "alpha", Issue: theirs}).State
	if conflict.Model.EditorUID != "alpha" {
		t.Fatalf("the conflict opened no card, editor %q", conflict.Model.EditorUID)
	}
}

func TestLateChangeAfterAnOrdinarySaveSettledIsSavedOnTheTextSaved(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = feed(t, s, &tern.Editing{ID: id, On: true}, &tern.Change{ID: id, Text: "mine"}, &tern.Editing{ID: id, On: false}).State
	s = feed(t, s, model.DraftSaved{UID: "alpha"}).State
	out := step(t, s, &tern.Change{ID: id, Text: "mine, late"})
	if saves := savedCmds(out.Cmds); len(saves) != 1 || saves[0].Text != "mine, late" || saves[0].BaseText != "mine" {
		t.Fatalf("a late change after an ordinary save settled sent %#v, want it saved on the text saved", out.Cmds)
	}
}

func focusOps(ops []tern.Op) int {
	n := 0
	for _, op := range ops {
		if driver.IsFocusOp(op) {
			n++
		}
	}
	return n
}

// The SDK sends a raw op as a frame, which ends the edit too: ops wait with the frames, and the step that releases the hold sends them
// after its view, with only the last focus.
func TestOpsWaitForTheEditingOff(t *testing.T) {
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
	s.Focus = "stale.one"
	theirs := alpha
	theirs.Body, theirs.ProjectID, theirs.Revision, theirs.QualifiedID = "someone else's text", 7, 4, "work#aaaa"
	out := driver.Step(s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &theirs}}, now.Add(time.Second))
	if len(out.Ops) != 0 || out.View != nil {
		t.Fatalf("a step during the edit sent view %v ops %#v", out.View != nil, out.Ops)
	}
	if focusOps(out.State.HeldOps) != 1 {
		t.Fatalf("held ops %#v, want the focus op kept", out.State.HeldOps)
	}
	s = out.State
	s.Focus = "stale.two"
	out = driver.Step(s, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &theirs}}, now.Add(2*time.Second))
	if focusOps(out.State.HeldOps) != 1 {
		t.Fatalf("held ops %#v, want only the last focus", out.State.HeldOps)
	}
	out = driver.Step(out.State, &tern.Editing{ID: id, On: false}, now.Add(3*time.Second))
	if out.View == nil || focusOps(out.Ops) != 1 || len(out.State.HeldOps) != 0 {
		t.Fatalf("the editing off sent view %v ops %#v, held %#v; want the view and one focus", out.View != nil, out.Ops, out.State.HeldOps)
	}
}

func TestInputOutsideTheDocReleasesTheHold(t *testing.T) {
	cases := map[string]struct {
		in      any
		release bool
	}{
		"key":              {key("x"), true},
		"select elsewhere": {&tern.Select{ID: "main.columns.list.issues", Item: "alpha"}, true},
		"action":           {&tern.Action{ID: "legend.chip", Act: "press"}, true},
		"change elsewhere": {&tern.Change{ID: "some.card.dropdown", Value: "closed"}, true},
		"focus elsewhere":  {&tern.Focus{ID: "some.field"}, true},
		"agent relay":      {&tern.Action{ID: "relay", Act: string(model.AgentStatesAction), Value: "[]"}, true},
	}
	for name, c := range cases {
		s := onDoc(t, true)
		id := bodyDocID(t, s)
		s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
		out := driver.Step(s, c.in, now.Add(time.Second))
		if name == "agent relay" { // the relay is the window half, not the user
			if out.View != nil {
				t.Fatalf("%s drew while the edit was open", name)
			}
			continue
		}
		if out.State.Model.HoldsFrames() {
			t.Fatalf("%s did not release the hold", name)
		}
		// a late editing off does no harm and starts no guard
		out = driver.Step(out.State, &tern.Editing{ID: id, On: false}, now.Add(1100*time.Millisecond))
		if strings.Contains(out.State.Model.Notice, "Cannot open the body for editing") {
			t.Fatalf("%s: the late editing off started the guard", name)
		}
	}
	s := onDoc(t, true)
	id := bodyDocID(t, s)
	s = driver.Step(s, &tern.Editing{ID: id, On: true}, now).State
	for _, in := range []any{&tern.Change{ID: id, Text: "mine"}, &tern.Focus{ID: id}, &tern.Select{ID: id}} {
		if out := driver.Step(s, in, now.Add(time.Second)); out.View != nil {
			t.Fatalf("%T on the doc itself released the hold", in)
		} else {
			s = out.State
		}
	}
}
