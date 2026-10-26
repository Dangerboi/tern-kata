package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func deleteText(t *testing.T, s driver.State, leaf string) string {
	t.Helper()
	text, _ := nodeText(t, s, view.LayerID(model.CardDelete)+"."+leaf)
	return text
}

func formText(t *testing.T, s driver.State, leaf string) string {
	t.Helper()
	text, _ := nodeText(t, s, view.LayerID(model.CardField)+"."+leaf)
	return text
}

func theDeletes(cmds []model.Cmd) []model.DeleteIssueCmd {
	var found []model.DeleteIssueCmd
	for _, cmd := range cmds {
		if c, ok := cmd.(model.DeleteIssueCmd); ok {
			found = append(found, c)
		}
	}
	return found
}

func oneDelete(t *testing.T, cmds []model.Cmd) model.DeleteIssueCmd {
	t.Helper()
	found := theDeletes(cmds)
	if len(found) != 1 {
		t.Fatalf("cmds %#v, want exactly one delete request", cmds)
	}
	return found[0]
}

// deleting is the card for Issue "a" of the Project "work".
func deleting(t *testing.T) driver.State {
	t.Helper()
	return step(t, movePane(t, movable("a"), movable("b")), key("shift+x")).State
}

func deleted(uid string) model.IssueDeleted {
	entry := model.UndoEntry{Kind: model.UndoDelete, UID: uid, ProjectID: 7, ShortID: uid, InstanceUID: "instance", Principal: "tester", Revision: 4}
	return model.IssueDeleted{UID: uid, Record: model.WriteRecord{Entry: &entry}}
}

func TestXOpensTheDeleteCardInEveryModeNamingTheIssueAndSayingItCanBeRestored(t *testing.T) {
	list := movePane(t, movable("a"), movable("b"))
	board := feed(t, list, key("b"), model.LanesLoaded{ProjectUID: "p"}).State
	lane := step(t, board, key("enter")).State
	detail := step(t, list, key("enter")).State
	for name, s := range map[string]driver.State{"list": list, "Board": board, "Lane view": lane, "detail": detail} {
		t.Run(name, func(t *testing.T) {
			open := step(t, s, key("shift+x"))
			m := open.State.Model
			if m.CurrentMode() != model.ModeDeleteForm || m.TopCard() != model.CardDelete || len(open.Cmds) != 0 {
				t.Fatalf("mode %s card %v cmds %v", m.CurrentMode(), m.TopCard(), open.Cmds)
			}
			if got := deleteText(t, open.State, "issue"); got != "work#a Title of a" {
				t.Errorf("the card names %q, want the qualified id and the title", got)
			}
			if got := deleteText(t, open.State, "note"); !strings.Contains(got, "can be restored") {
				t.Errorf("the card says %q, want that a deleted Issue can be restored", got)
			}
			if !strings.HasPrefix(open.State.Focus, view.LayerID(model.CardDelete)) {
				t.Errorf("text focus %q is not on the Delete card", open.State.Focus)
			}
			if closed := step(t, open.State, key("escape")); closed.State.Model.TopCard() != model.NoCard || len(closed.Cmds) != 0 {
				t.Fatalf("esc: card %v cmds %v", closed.State.Model.TopCard(), closed.Cmds)
			}
		})
	}
}

func TestTheDeleteCardRefusesUntilTheTypedIdMatches(t *testing.T) {
	s := deleting(t)
	for _, wrong := range []string{"", "work#", "work#b", "a", "WORK#A"} {
		typedWrong := typed(t, s, wrong)
		out := step(t, typedWrong, key("enter"))
		if len(theDeletes(out.Cmds)) != 0 {
			t.Fatalf("typed %q and Enter sent %#v", wrong, out.Cmds)
		}
		if got := deleteText(t, out.State, "error"); got != "Type work#a to confirm the delete." {
			t.Errorf("typed %q: the card says %q", wrong, got)
		}
		if out.State.Model.TopCard() != model.CardDelete {
			t.Fatalf("typed %q: the card closed", wrong)
		}
	}
	out := step(t, typed(t, s, "work#a"), key("enter"))
	want := model.DeleteIssueCmd{Workspace: "/repo", UID: "a", ProjectID: 7, QualifiedID: "work#a"}
	if got := oneDelete(t, out.Cmds); got != want {
		t.Fatalf("delete %+v, want %+v", got, want)
	}
	if !out.State.Model.DeleteForm.Saving {
		t.Fatal("the card does not wait for kata")
	}
	if again := step(t, out.State, key("enter")); len(again.Cmds) != 0 {
		t.Fatalf("Enter while the delete is out sent %#v; one delete is sent", again.Cmds)
	}
}

func TestTheDeleteCardSendsTheOptionalReason(t *testing.T) {
	s := typed(t, deleting(t), "work#a")
	s = typed(t, step(t, s, key("tab")).State, "duplicate of b")
	if got := s.Model.DeleteForm.Field; got != model.DeleteReason {
		t.Fatalf("tab moved to %q, want the reason", got)
	}
	got := oneDelete(t, step(t, s, key("enter")).Cmds)
	if got.Reason != "duplicate of b" || got.QualifiedID != "work#a" {
		t.Fatalf("delete %+v, want the reason with the confirmed id", got)
	}
}

func TestKatasRefusalOfADeleteShowsOnTheCardAndKeepsWhatWasTyped(t *testing.T) {
	s := typed(t, deleting(t), "work#a")
	sent := step(t, s, key("enter"))
	refused := step(t, sent.State, model.IssueDeleted{UID: "a", Err: errors.New("X-Kata-Confirm header value does not match\nexpected DELETE work#a")})
	m := refused.State.Model
	if m.TopCard() != model.CardDelete || m.DeleteForm.Saving || m.DeleteForm.ID != "work#a" {
		t.Fatalf("after the refusal: card %v saving %v typed %q", m.TopCard(), m.DeleteForm.Saving, m.DeleteForm.ID)
	}
	if got := deleteText(t, refused.State, "error"); !strings.Contains(got, "does not match") {
		t.Errorf("the card says %q, want kata's refusal", got)
	}
	if got := oneDelete(t, step(t, refused.State, key("enter")).Cmds); got.UID != "a" {
		t.Errorf("retry %+v", got)
	}
}

func TestADeleteClosesTheCardLeavesTheListAndUDeletesItBack(t *testing.T) {
	s := typed(t, deleting(t), "work#a")
	sent := step(t, s, key("enter"))
	out := step(t, sent.State, deleted("a"))
	m := out.State.Model
	if m.TopCard() != model.NoCard || m.Notice != "Deleted work#a. Press u to restore it." {
		t.Fatalf("card %v notice %q", m.TopCard(), m.Notice)
	}
	after := movedSnapshot(t, out.State, out.Cmds, movable("b"))
	if after.Model.SelectedUID != "b" {
		t.Errorf("the selection is %q, want the neighbouring row b", after.Model.SelectedUID)
	}

	undone := pressU(t, after)
	cmd := undoCmd(t, undone)
	if cmd.Entry.Kind != model.UndoDelete || cmd.Entry.UID != "a" || cmd.Workspace != "/repo" {
		t.Fatalf("u sent %+v, want the restore of a", cmd)
	}
	if len(theDeletes(undone.Cmds)) != 0 || len(undone.Cmds) != 1 {
		t.Fatalf("u sent %#v, want one undo", undone.Cmds)
	}
	back := step(t, undone.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoChanged, Ref: "a"})
	if got := back.State.Model.Notice; got != "undid delete on #a" {
		t.Errorf("notice %q", got)
	}
	if _, asked := snapshotFor(back.Cmds); !asked {
		t.Errorf("the Collection was not read again after the restore: %#v", back.Cmds)
	}
	if got := pressU(t, back.State).State.Model.Notice; got != "nothing to undo" {
		t.Errorf("a second u says %q", got)
	}
}

func TestUAfterADeleteSaysAlreadyRestoredWhenKataHoldsItRestored(t *testing.T) {
	s := typed(t, deleting(t), "work#a")
	s = feed(t, s, key("enter"), deleted("a")).State
	cmd := undoCmd(t, pressU(t, s))
	out := step(t, pressU(t, s).State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoAlready, Ref: "a"})
	if got := out.State.Model.Notice; got != "#a already restored; no change made" {
		t.Fatalf("notice %q", got)
	}
}

func TestDeletingFromTheDetailLeavesTheDetail(t *testing.T) {
	list := movePane(t, movable("a"), movable("b"))
	detail := step(t, list, key("enter")).State
	s := typed(t, step(t, detail, key("shift+x")).State, "work#a")
	out := feed(t, s, key("enter"), deleted("a"))
	if out.State.Model.Focus != model.ColumnList || out.State.Model.OpenedIssueUID() == "a" {
		t.Fatalf("focus %v opened %q, want the list without the deleted Issue's detail", out.State.Model.Focus, out.State.Model.OpenedIssueUID())
	}
}

func menuRestore(t *testing.T, s driver.State) driver.State {
	t.Helper()
	menu := typed(t, step(t, s, openMenu).State, "Restore")
	open := step(t, menu, key("enter"))
	m := open.State.Model
	if m.CurrentMode() != model.ModeFieldForm || m.FieldForm.Action != model.RestoreDeletedAction || len(open.Cmds) != 0 {
		t.Fatalf("mode %s action %q cmds %v", m.CurrentMode(), m.FieldForm.Action, open.Cmds)
	}
	return open.State
}

func TestTheMenuOffersToRestoreADeletedIssueThroughOneField(t *testing.T) {
	s := menuRestore(t, movePane(t, movable("a")))
	if got := s.Model.FieldFormTitle(); got != "Restore a deleted Issue (short id, project#id or ULID)" {
		t.Errorf("title %q", got)
	}
	// Enter reads the Issue first.
	asked := step(t, typed(t, s, "a"), key("enter"))
	want := model.LookupDeleted{Workspace: "/repo", Typed: "a", Ref: "a", ProjectID: 7}
	if got := onlyCmd[model.LookupDeleted](t, asked.Cmds); got != want {
		t.Fatalf("lookup %+v, want %+v", got, want)
	}
	// It shows the title and that the Issue is deleted.
	found := step(t, asked.State, model.DeletedLooked{Ref: "a", Issue: model.DeletedIssue{UID: "x", ProjectID: 7, ShortID: "a", Title: "Gone", Deleted: true}})
	if got := formText(t, found.State, "found"); got != "work#a Gone" {
		t.Errorf("the form shows %q, want the Issue and its title", got)
	}
	if got := formText(t, found.State, "found-state"); !strings.Contains(got, "is deleted") {
		t.Errorf("the form says %q, want that the Issue is deleted", got)
	}
	// Enter restores it.
	restoring := step(t, found.State, key("enter"))
	if got := onlyCmd[model.RestoreIssueCmd](t, restoring.Cmds); got != (model.RestoreIssueCmd{Workspace: "/repo", UID: "x", ProjectID: 7}) {
		t.Fatalf("restore %+v", got)
	}
	if again := step(t, restoring.State, key("enter")); len(again.Cmds) != 0 {
		t.Fatalf("Enter while the restore is out sent %#v", again.Cmds)
	}
	done := step(t, restoring.State, model.IssueRestored{UID: "x", Changed: true})
	if done.State.Model.TopCard() != model.NoCard || done.State.Model.Notice != "Restored work#a." {
		t.Fatalf("card %v notice %q", done.State.Model.TopCard(), done.State.Model.Notice)
	}
	if _, asked := snapshotFor(done.Cmds); !asked {
		t.Errorf("the Collection was not read again after the restore: %#v", done.Cmds)
	}
}

func TestRestoringAnIssueOfAProjectOutsideTheCatalogNamesItByShortIdAlone(t *testing.T) {
	s := menuRestore(t, movePane(t, movable("a")))
	asked := step(t, typed(t, s, "a"), key("enter"))
	found := step(t, asked.State, model.DeletedLooked{Ref: "a", Issue: model.DeletedIssue{UID: "x", ProjectID: 99, ShortID: "abcd", Title: "Gone", Deleted: true}})
	restoring := step(t, found.State, key("enter"))
	done := step(t, restoring.State, model.IssueRestored{UID: "x", Changed: true})
	if got := done.State.Model.Notice; got != "Restored abcd." {
		t.Errorf("notice %q, want no bare leading #", got)
	}
}

func onlyCmd[C model.Cmd](t *testing.T, cmds []model.Cmd) C {
	t.Helper()
	var found []C
	for _, cmd := range cmds {
		if c, ok := cmd.(C); ok {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("cmds %#v, want exactly one %T", cmds, *new(C))
	}
	return found[0]
}

func TestTheMenuRestoreTakesProjectQualifiedIdsAndULIDs(t *testing.T) {
	const ulid = "01J9ZZZZZZZZZZZZZZZZZZZZZZ"
	for _, tt := range []struct {
		typed string
		want  model.LookupDeleted
	}{
		{"shop#zz", model.LookupDeleted{Workspace: "/repo", Typed: "shop#zz", Ref: "zz", ProjectID: 8}},
		{"#zz", model.LookupDeleted{Workspace: "/repo", Typed: "#zz", Ref: "zz", ProjectID: 7}},
		{ulid, model.LookupDeleted{Workspace: "/repo", Typed: ulid, Ref: ulid, ByUID: true}},
	} {
		s := menuRestore(t, movePane(t, movable("a")))
		if got := onlyCmd[model.LookupDeleted](t, step(t, typed(t, s, tt.typed), key("enter")).Cmds); got != tt.want {
			t.Errorf("typed %q: lookup %+v, want %+v", tt.typed, got, tt.want)
		}
	}
}

func TestTheMenuRestoreRefusesWhatItCannotAsk(t *testing.T) {
	for typedRef, want := range map[string]string{
		"nowhere#zz": `kata Project "nowhere" not found`,
		"shop#":      "Enter an Issue id after #.",
		"":           "Enter a value.",
	} {
		s := menuRestore(t, movePane(t, movable("a")))
		out := step(t, typed(t, s, typedRef), key("enter"))
		if len(out.Cmds) != 0 {
			t.Errorf("typed %q sent %#v", typedRef, out.Cmds)
		}
		if got := formText(t, out.State, "error"); got != want {
			t.Errorf("typed %q: the form says %q, want %q", typedRef, got, want)
		}
	}
}

func TestKatasRefusalsShowOnTheRestoreForm(t *testing.T) {
	s := menuRestore(t, movePane(t, movable("a")))
	asked := step(t, typed(t, s, "zz"), key("enter"))
	missing := step(t, asked.State, model.DeletedLooked{Ref: "zz", Err: errors.New("issue not found")})
	if got := formText(t, missing.State, "error"); got != "issue not found" || missing.State.Model.TopCard() != model.CardField {
		t.Fatalf("the form says %q, want kata's refusal on the open form", got)
	}

	// An Issue kata holds as not deleted is shown as such; restoring it reports kata's answer.
	again := step(t, missing.State, key("enter"))
	found := step(t, again.State, model.DeletedLooked{Ref: "zz", Issue: model.DeletedIssue{UID: "x", ProjectID: 7, ShortID: "zz", Title: "Alive"}})
	if got := formText(t, found.State, "found-state"); !strings.Contains(got, "not deleted") {
		t.Errorf("the form says %q, want that the Issue is not deleted", got)
	}
	restoring := step(t, found.State, key("enter"))
	nothing := step(t, restoring.State, model.IssueRestored{UID: "x"})
	if got := formText(t, nothing.State, "error"); got != "work#zz is not deleted; kata restored nothing." {
		t.Errorf("the form says %q", got)
	}
	refused := step(t, step(t, nothing.State, key("enter")).State, model.IssueRestored{UID: "x", Err: errors.New("federated replicas are read-only")})
	if got := formText(t, refused.State, "error"); got != "federated replicas are read-only" || refused.State.Model.TopCard() != model.CardField {
		t.Errorf("the form says %q", got)
	}
}

func TestEditingTheReferenceAfterTheIssueIsShownReadsAgain(t *testing.T) {
	s := menuRestore(t, movePane(t, movable("a")))
	asked := step(t, typed(t, s, "zz"), key("enter"))
	found := step(t, asked.State, model.DeletedLooked{Ref: "zz", Issue: model.DeletedIssue{UID: "x", ProjectID: 7, ShortID: "zz", Title: "Gone", Deleted: true}})
	edited := typed(t, found.State, "y")
	if edited.Model.FieldForm.Found != nil {
		t.Fatal("the form still shows the Issue after the reference changed")
	}
	if got := onlyCmd[model.LookupDeleted](t, step(t, edited, key("enter")).Cmds); got.Typed != "zzy" {
		t.Errorf("lookup %+v, want the edited reference", got)
	}
}

func TestARestoreFromTheMenuClearsTheUndoHistory(t *testing.T) {
	s := typed(t, deleting(t), "work#a")
	s = feed(t, s, key("enter"), deleted("a")).State
	menu := menuRestore(t, s)
	asked := step(t, typed(t, menu, "zz"), key("enter"))
	found := step(t, asked.State, model.DeletedLooked{Ref: "zz", Issue: model.DeletedIssue{UID: "x", ProjectID: 7, ShortID: "zz", Title: "Gone", Deleted: true}})
	restoring := step(t, found.State, key("enter"))
	done := feed(t, restoring.State, model.IssueRestored{UID: "x", Changed: true})
	if got := pressU(t, done.State).State.Model.Notice; got != "nothing to undo: restoring an Issue cannot be undone" {
		t.Fatalf("u says %q", got)
	}
}

func TestErasingTheRestoreFieldForgetsTheIssueFoundForIt(t *testing.T) {
	for _, chord := range []string{"backspace", "ctrl+u"} {
		s := menuRestore(t, movePane(t, movable("a")))
		asked := step(t, typed(t, s, "a"), key("enter"))
		found := step(t, asked.State, model.DeletedLooked{Ref: "a", Issue: model.DeletedIssue{UID: "x", ProjectID: 7, ShortID: "a", Title: "Gone", Deleted: true}})
		if found.State.Model.FieldForm.Found == nil {
			t.Fatal("fixture: no Issue found")
		}
		erased := step(t, found.State, key(chord))
		if erased.State.Model.FieldForm.Found != nil {
			t.Errorf("%s left the Issue found for the old text", chord)
		}
		for _, cmd := range step(t, erased.State, key("enter")).Cmds {
			if restore, ok := cmd.(model.RestoreIssueCmd); ok {
				t.Fatalf("%s then Enter restored %+v from an erased field", chord, restore)
			}
		}
	}
}

func TestCtrlUClearsTheDeleteCardsConfirmation(t *testing.T) {
	s := typed(t, deleting(t), "work#wrong")
	s = step(t, s, key("ctrl+u")).State
	if got := s.Model.DeleteForm.ID; got != "" {
		t.Fatalf("ctrl+u left %q in the confirmation", got)
	}
}
