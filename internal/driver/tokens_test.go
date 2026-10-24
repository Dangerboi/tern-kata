package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// tokenList is the Credentials Page of an account that may read the audit, with two tokens listed, the newest (laptop) selected.
func tokenList(t *testing.T) driver.State {
	t.Helper()
	audited, load := credentialsAudited(t)
	audited.Model.Actor = "alice"
	return step(t, audited, model.CredentialsLoaded{Request: load.Request, ObservedAt: now, Tokens: []model.Credential{
		credentialToken(3, "laptop", "live", now),
		credentialToken(2, "old", "revoked", now.Add(-time.Hour)),
	}}).State
}

// tokenCard is tokenList with the Token card open.
func tokenCard(t *testing.T) driver.State {
	t.Helper()
	out := step(t, tokenList(t), key("n"))
	if out.State.Model.CurrentMode() != model.ModeTokenForm || out.State.Model.TopCard() != model.CardToken || len(out.Cmds) != 0 {
		t.Fatalf("n gave mode %q, card %v and %#v, want the Token card and no request", out.State.Model.CurrentMode(), out.State.Model.TopCard(), out.Cmds)
	}
	return out.State
}

// setToken puts text in the field of the open Token card the way a user does: focus it, clear it, type.
func setToken(t *testing.T, s driver.State, field model.Field, text string) driver.State {
	t.Helper()
	s = step(t, s, model.FocusFieldMsg{Field: field}).State
	s = step(t, s, key("ctrl+u")).State
	return typed(t, s, text)
}

func askedCreateToken(cmds []model.Cmd) (model.CreateTokenCmd, bool) {
	for _, cmd := range cmds {
		if create, ok := cmd.(model.CreateTokenCmd); ok {
			return create, true
		}
	}
	return model.CreateTokenCmd{}, false
}

// validToken is the Token card filled in for an account token into a file.
func validToken(t *testing.T) driver.State {
	t.Helper()
	s := setToken(t, tokenCard(t), model.TokenFile, "/secrets/ci.token")
	return setToken(t, s, model.TokenName, "ci")
}

func TestTokenCardOpensOnNWithThePanesActorAndAccountScope(t *testing.T) {
	s := tokenCard(t)
	v := s.Model.TokenForm.Values
	if v.Actor != "alice" || v.Scope != model.TokenScopeAccount {
		t.Errorf("the card opened with actor %q and scope %q, want alice and account", v.Actor, v.Scope)
	}
	wantText(t, "the Token card", wireTextOf(t, s), "New token", "Actor", "Expires in", "Scope", "Issue", "Teams", "Token file")
}

func TestNAndXDoNothingWithoutTokenAuditRead(t *testing.T) {
	opened := credentialsOpened(t, projectPane(t, 160, boardCard("a")))
	check, _ := askedCapabilities(opened.Cmds)
	unavailable := step(t, opened.State, model.AuthCapabilitiesLoaded{Request: check.Request}).State
	for _, k := range []string{"n", "x"} {
		out := step(t, unavailable, key(k))
		if out.State.Model.TopCard() != model.NoCard || len(out.Cmds) != 0 {
			t.Errorf("%s without audit rights opened card %v and asked %#v", k, out.State.Model.TopCard(), out.Cmds)
		}
	}
}

func TestTokenCardRefusesWhatKataWouldRefuseBeforeSendingAnything(t *testing.T) {
	tooMany := strings.TrimSuffix(strings.Repeat("t,", 257), ",")
	for _, c := range []struct {
		name   string
		fields map[model.Field]string
		want   string
	}{
		{"a missing actor", map[model.Field]string{model.TokenActor: ""}, "actor is required"},
		{"a duration that is not a duration", map[model.Field]string{model.TokenExpires: "soon"}, "positive whole-second duration"},
		{"a duration of a fraction of a second", map[model.Field]string{model.TokenExpires: "1500ms"}, "positive whole-second duration"},
		{"a duration of nothing", map[model.Field]string{model.TokenExpires: "0s"}, "positive whole-second duration"},
		{"a negative duration", map[model.Field]string{model.TokenExpires: "-1h"}, "positive whole-second duration"},
		{"a scope that is neither", map[model.Field]string{model.TokenScope: "team"}, "scope must be account or issue"},
		{"a subtree token without an Issue", map[model.Field]string{model.TokenScope: "issue", model.TokenExpires: "24h"}, "needs an Issue"},
		{"a subtree token without an expiry", map[model.Field]string{model.TokenScope: "issue", model.TokenIssue: "a12"}, "needs an expiry"},
		{"an account token with an Issue", map[model.Field]string{model.TokenIssue: "a12"}, "covers no Issue"},
		{"an Issue in a Project that is not there", map[model.Field]string{model.TokenScope: "issue", model.TokenIssue: "nowhere#a12", model.TokenExpires: "24h"}, "nowhere"},
		{"too many teams", map[model.Field]string{model.TokenTeams: tooMany}, "at most 256 teams"},
		{"no file", map[model.Field]string{model.TokenFile: ""}, "token file is required"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := setToken(t, tokenCard(t), model.TokenFile, "/secrets/ci.token")
			for field, text := range c.fields {
				s = setToken(t, s, field, text)
			}
			out := step(t, s, key("enter"))
			if len(out.Cmds) != 0 || out.State.Model.TokenForm.Saving {
				t.Fatalf("sent %#v, want nothing", out.Cmds)
			}
			wantText(t, "the card", wireTextOf(t, out.State), c.want)
			if !out.State.Model.TokenForm.Open {
				t.Error("the card closed on a refusal")
			}
		})
	}
}

func TestTokenCardSendsOneCreateForAnAccountToken(t *testing.T) {
	s := setToken(t, validToken(t), model.TokenExpires, "720h")
	s = setToken(t, s, model.TokenTeams, " core, ops ,, ")
	out := step(t, s, key("enter"))
	want := model.CreateTokenCmd{Workspace: "/repo", Actor: "alice", Name: "ci", ExpiresInSeconds: 720 * 3600, Teams: []string{"core", "ops"}, File: "/secrets/ci.token"}
	if len(out.Cmds) != 1 || !reflect.DeepEqual(out.Cmds[0], want) {
		t.Fatalf("sent %#v, want %#v", out.Cmds, want)
	}
	wantText(t, "while saving", wireTextOf(t, out.State), "Creating token")
	if again := step(t, out.State, key("enter")); len(again.Cmds) != 0 {
		t.Errorf("a second enter while saving sent %#v", again.Cmds)
	}
}

func TestTokenCardResolvesAnIssueSubtreeToItsProject(t *testing.T) {
	for ref, want := range map[string]struct {
		project int64
		short   string
	}{"a12": {7, "a12"}, "#a12": {7, "a12"}, "work#a12": {7, "a12"}, "work/a12": {7, "a12"}, "ops#9": {8, "9"}} {
		s := tokenCard(t)
		s.Model.Snapshot.Catalog = append(s.Model.Snapshot.Catalog, model.CatalogProject{Project: model.Project{ID: 8, UID: "o", Name: "ops"}})
		s = setToken(t, s, model.TokenFile, "/secrets/sub.token")
		s = setToken(t, s, model.TokenScope, "issue")
		s = setToken(t, s, model.TokenIssue, ref)
		s = setToken(t, s, model.TokenExpires, "24h")
		create, ok := askedCreateToken(step(t, s, key("enter")).Cmds)
		if !ok || !create.Subtree || create.ProjectID != want.project || create.ShortID != want.short || create.ExpiresInSeconds != 86400 {
			t.Errorf("Issue %q sent %+v, want a subtree token of %d/%s for 24h", ref, create, want.project, want.short)
		}
	}
}

func TestACreatedTokenIsReportedByIdAndFileAndTheListIsReadAgain(t *testing.T) {
	out := step(t, validToken(t), key("enter"))
	done := step(t, out.State, model.TokenCreated{ID: 11, File: "/secrets/ci.token"})
	wantText(t, "the card", wireTextOf(t, done.State), "Created token 11", "/secrets/ci.token", "not shown")
	if strings.Contains(wireTextOf(t, done.State), "kata_") {
		t.Error("the card draws something that looks like a secret")
	}
	if read, ok := askedCredentials(done.Cmds); !ok || read.After != 0 {
		t.Errorf("after the create: %#v, want the list read at once", done.Cmds)
	}
	// The card only reports now: keys type nothing, and enter or esc close it.
	if typedOn := typed(t, done.State, "zz"); typedOn.Model.TokenForm.Values != done.State.Model.TokenForm.Values {
		t.Error("typing changed the card of a made token")
	}
	for _, k := range []string{"enter", "escape"} {
		if closed := step(t, done.State, key(k)).State.Model; closed.TokenForm.Open {
			t.Errorf("%s left the card open", k)
		}
	}
}

func TestKatasRefusalsToCreateShowOnTheCardAndLeaveItEditable(t *testing.T) {
	for _, refusal := range []string{
		"identity tokens are not accepted while auth.require_token_identity is false",
		"the selected daemon does not support issue-scoped tokens (issue_subtree_tokens_unsupported)",
		"scoped tokens cannot be issued for a spoke replica (scoped_token_spoke_forbidden)",
		"token administration requires the bootstrap token or a local no-auth session",
		"token file /secrets/ci.token already exists; name a new file",
	} {
		out := step(t, validToken(t), key("enter"))
		failed := step(t, out.State, model.TokenCreated{Err: errors.New(refusal)})
		wantText(t, "the card", wireTextOf(t, failed.State), refusal)
		if !failed.State.Model.TokenForm.Open || failed.State.Model.TokenForm.Saving || failed.State.Model.TokenForm.Done {
			t.Errorf("after %q the card is %+v, want it open and editable", refusal, failed.State.Model.TokenForm)
		}
		fixed := setToken(t, failed.State, model.TokenFile, "/secrets/other.token")
		if _, ok := askedCreateToken(step(t, fixed, key("enter")).Cmds); !ok {
			t.Errorf("after %q the card could not send again", refusal)
		}
	}
}

func TestEscClosesTheTokenCardWithoutSending(t *testing.T) {
	out := step(t, validToken(t), key("escape"))
	if out.State.Model.TokenForm.Open || len(out.Cmds) != 0 {
		t.Errorf("esc left the card %+v and sent %#v", out.State.Model.TokenForm, out.Cmds)
	}
}

func TestXAsksBeforeRevokingAndYRevokes(t *testing.T) {
	s := tokenList(t)
	asked := step(t, s, key("x"))
	if asked.State.Model.CurrentMode() != model.ModeRevokeToken || len(asked.Cmds) != 0 {
		t.Fatalf("x gave mode %q and %#v, want the question and no request", asked.State.Model.CurrentMode(), asked.Cmds)
	}
	wantText(t, "the question", wireTextOf(t, asked.State), "Revoke token 3 (laptop, alice)? This cannot be undone.")

	for _, answer := range []string{"n", "escape"} {
		kept := step(t, asked.State, key(answer))
		if kept.State.Model.RevokeCard.Open || len(kept.Cmds) != 0 {
			t.Errorf("%s left the question open or sent %#v", answer, kept.Cmds)
		}
	}
	for _, other := range []string{"z", "enter", "x"} {
		if still := step(t, asked.State, key(other)); !still.State.Model.RevokeCard.Open || len(still.Cmds) != 0 {
			t.Errorf("%s answered the question: %#v", other, still.Cmds)
		}
	}

	sent := step(t, asked.State, key("y"))
	if len(sent.Cmds) != 1 || !reflect.DeepEqual(sent.Cmds[0], model.RevokeTokenCmd{Workspace: "/repo", ID: 3}) {
		t.Fatalf("y sent %#v, want one revoke of token 3", sent.Cmds)
	}
	if again := step(t, sent.State, key("y")); len(again.Cmds) != 0 {
		t.Errorf("a second y while revoking sent %#v", again.Cmds)
	}

	done := step(t, sent.State, model.TokenRevoked{ID: 3})
	if done.State.Model.RevokeCard.Open {
		t.Error("the question stays after the token is revoked")
	}
	if read, ok := askedCredentials(done.Cmds); !ok || read.After != 0 {
		t.Errorf("after the revoke: %#v, want the list read at once", done.Cmds)
	}
	if !strings.Contains(done.State.Model.Notice, "Revoked token 3") {
		t.Errorf("notice %q, want it to name token 3", done.State.Model.Notice)
	}
}

func TestKatasRefusalToRevokeShowsOnTheQuestion(t *testing.T) {
	sent := step(t, step(t, tokenList(t), key("x")).State, key("y"))
	failed := step(t, sent.State, model.TokenRevoked{ID: 3, Err: errors.New("token not found")})
	wantText(t, "the question", wireTextOf(t, failed.State), "token not found", "Revoke token 3")
	if !failed.State.Model.RevokeCard.Open || failed.State.Model.RevokeCard.Saving {
		t.Errorf("after a refusal the question is %+v, want it open to answer again", failed.State.Model.RevokeCard)
	}
	if again := step(t, failed.State, key("y")); len(again.Cmds) != 1 {
		t.Errorf("y after a refusal sent %#v, want another try", again.Cmds)
	}
}

func TestXOnARevokedTokenAsksNothing(t *testing.T) {
	s := step(t, tokenList(t), key("j")).State
	out := step(t, s, key("x"))
	if out.State.Model.RevokeCard.Open || len(out.Cmds) != 0 || !strings.Contains(out.State.Model.Notice, "already revoked") {
		t.Errorf("x on a revoked token: card %+v, notice %q, sent %#v", out.State.Model.RevokeCard, out.State.Model.Notice, out.Cmds)
	}
}
