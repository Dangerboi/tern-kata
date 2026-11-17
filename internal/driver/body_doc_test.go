package driver_test

import (
	"strings"
	"testing"

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
		if strings.HasSuffix(n.ID, ".content.body") {
			return n
		}
	}
	t.Fatal("the view draws no body")
	return tern.WireNode{}
}

func bodyDocID(t *testing.T, s driver.State) string { return bodyNode(t, s).ID }

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
	id := view.BodyID
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
	id := view.BodyID
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
	// The doc, replaced by beta's, ends the edit and may send a late change: neither writes beta.
	s = out.State
	late := feed(t, s, &tern.Change{ID: id, Text: "late"}, &tern.Editing{ID: id, On: false})
	if _, held := late.State.Model.Drafts["beta"]; held || len(savedCmds(late.Cmds)) != 0 {
		t.Fatalf("late events wrote %v, held beta draft %v", late.Cmds, held)
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
	if n := s.Model.Notice; !strings.Contains(n, "work#aaaa") || !strings.Contains(n, "reconnecting") {
		t.Fatalf("notice %q, want it to name the Issue and say kata is reconnecting", n)
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
		if strings.Count(line, "kept") != 1 || !strings.Contains(line, "press e or c to open it") && !strings.Contains(line, "Press e or c to open it") {
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
