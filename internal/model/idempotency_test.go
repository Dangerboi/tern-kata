package model_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

var errTest = errors.New("network down")

func keyOf(t *testing.T, cmds []model.Cmd) string {
	t.Helper()
	if len(cmds) != 1 {
		t.Fatalf("cmds %v", cmds)
	}
	switch c := cmds[0].(type) {
	case model.SaveDraft:
		return c.Key
	case model.CreateIssueCmd:
		return c.Key
	case model.CloseIssueCmd:
		return c.Key
	}
	t.Fatalf("cmd %T", cmds[0])
	return ""
}

// withoutKeys is cmds with the idempotency key blanked, for tests that do not care what it is.
func withoutKeys(cmds []model.Cmd) []model.Cmd {
	out := slices.Clone(cmds)
	for i, cmd := range out {
		if save, ok := cmd.(model.SaveDraft); ok {
			save.Key = ""
			out[i] = save
		}
	}
	return out
}

func started(t *testing.T) model.Model {
	m, _ := model.Update(onComments(), model.Start{Workspace: "/repo", Nonce: "run1"})
	return m
}

func TestCommentResendsReuseTheKeyUntilTheDraftChanges(t *testing.T) {
	m, _ := clickPress(started(t), "c")
	m = typed(m, "hello")
	m, cmds := clickPress(m, "meta+o")
	first := keyOf(t, cmds)
	if first == "" {
		t.Fatal("a comment send carries no key")
	}
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: errTest})
	m, cmds = clickPress(m, "meta+o")
	if got := keyOf(t, cmds); got != first {
		t.Fatalf("unchanged resend key %q, want %q", got, first)
	}
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: errTest})
	m = typed(m, " edited")
	m, cmds = clickPress(m, "meta+o")
	if got := keyOf(t, cmds); got == first {
		t.Fatalf("edited resend reused key %q", got)
	}
}

func TestACommentResentAfterOverwritingAConflictKeepsItsKey(t *testing.T) {
	m, _ := clickPress(started(t), "c")
	m = typed(m, "hello")
	m, cmds := clickPress(m, "meta+o")
	first := keyOf(t, cmds)
	latest := model.Issue{UID: "a", ProjectID: 2, Body: "someone else's body", Revision: 9}
	m, _ = model.Update(m, model.DraftConflict{UID: "a", Issue: latest})
	_, cmds = clickPress(m, "o")
	if got := keyOf(t, cmds); got != first {
		t.Fatalf("resend after overwrite key %q, want the first send's %q", got, first)
	}
}

func TestEachReplyDraftMintsItsOwnKey(t *testing.T) {
	send := func(m model.Model) (model.Model, string) {
		m, _ = clickPress(m, "shift+r")
		m = typed(m, "same text")
		m, cmds := clickPress(m, "meta+o")
		return m, keyOf(t, cmds)
	}
	m, first := send(started(t))
	m, _ = model.Update(m, model.DraftSaved{UID: "a"})
	_, second := send(m)
	if first == second {
		t.Fatalf("two Drafts shared key %q", first)
	}
}

func TestBodyEditsCarryNoKey(t *testing.T) {
	m, _ := clickPress(started(t), "e")
	m = typed(m, "x")
	_, cmds := clickPress(m, "meta+o")
	if got := keyOf(t, cmds); got != "" {
		t.Fatalf("body edit key %q", got)
	}
}

func TestCreateCardKeys(t *testing.T) {
	open := func(m model.Model) model.Model {
		m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
		m, _ = model.Update(m, model.KeyMsg{Text: "Title"})
		return m
	}
	m, _ := model.Update(model.Model{}, model.Start{Nonce: "run1"})
	m.Loading, m.Project, m.SelectedUID, m.Issues = false, model.Project{ID: 7}, "parent", []model.Issue{{UID: "parent", ProjectID: 7}}
	m = open(m)
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	first := keyOf(t, cmds)
	m, _ = model.Update(m, model.IssueCreated{Err: errTest})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if got := keyOf(t, cmds); got != first {
		t.Fatalf("unchanged resend key %q, want %q", got, first)
	}
	m, _ = model.Update(m, model.IssueCreated{Err: errTest})
	m, _ = model.Update(m, model.KeyMsg{Text: "!"})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	edited := keyOf(t, cmds)
	if edited == first {
		t.Fatal("edited resend reused the key")
	}
	m, _ = model.Update(m, model.IssueCreated{UID: "n"})
	m.Loading = false
	m = open(m)
	_, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if got := keyOf(t, cmds); got == first || got == edited {
		t.Fatalf("new card key %q reused", got)
	}
}

func TestCloseCardKeys(t *testing.T) {
	open := func(m model.Model) model.Model {
		m, _ = model.Update(m, model.KeyMsg{Chord: "x"})
		return m
	}
	m, _ := model.Update(model.Model{}, model.Start{Nonce: "run1"})
	m.SelectedUID, m.Issues = "issue", []model.Issue{{UID: "issue", ProjectID: 2}}
	m = open(m)
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	first := keyOf(t, cmds)
	m, _ = model.Update(m, model.CloseResultMsg{UID: "issue", Err: errTest})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	if got := keyOf(t, cmds); got != first {
		t.Fatalf("unchanged resend key %q, want %q", got, first)
	}
	m, _ = model.Update(m, model.CloseResultMsg{UID: "issue", Err: errTest})
	m, _ = model.Update(m, model.CloseFieldMsg{Field: model.CloseReason, Value: "wontfix"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	m, _ = model.Update(m, model.CloseResultMsg{UID: "issue", Err: errTest})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	edited := keyOf(t, cmds)
	if edited != first {
		// quick close ignores the typed reason, so it is the same request: the key stays.
		t.Fatalf("quick resend key %q, want %q", edited, first)
	}
	m, _ = model.Update(m, model.CloseResultMsg{UID: "issue", Err: errTest})
	m, _ = model.Update(m, model.CloseFieldMsg{Field: model.CloseMessage, Value: "Shipped in the release, verified by the tests."})
	m, _ = model.Update(m, model.CloseFieldMsg{Field: model.CloseCommit, Value: "abcdef1"})
	m, _ = model.Update(m, model.CloseFieldMsg{Field: model.CloseReason, Value: "done"})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	full := keyOf(t, cmds)
	if full == first {
		t.Fatal("an edited close reused the key")
	}
	m, _ = model.Update(m, model.CloseResultMsg{UID: "issue"})
	m = open(m)
	_, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	if got := keyOf(t, cmds); got == first || got == full {
		t.Fatalf("new card key %q reused", got)
	}
}
