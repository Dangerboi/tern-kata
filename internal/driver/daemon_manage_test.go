package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func askedSaveDaemon(cmds []model.Cmd) (model.SaveDaemonCmd, bool) {
	return asked[model.SaveDaemonCmd](cmds)
}

// manageEntries lists a daemon that has each kind of token, so an edit shows what it prefills.
var manageEntries = []model.DaemonEntry{
	{Name: "home", Endpoint: "local", Local: true},
	{Name: "work", Endpoint: "kata.example.com:7474", URL: "https://kata.example.com:7474", Token: true, TokenEnv: "WORK_TOKEN", Current: true},
	{Name: "lab", Endpoint: "10.0.0.5:7474", URL: "http://10.0.0.5:7474", Token: true, InlineToken: true, AllowInsecure: true},
}

func daemonsManaged(t *testing.T) driver.State {
	t.Helper()
	out := daemonsOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedDaemons(out.Cmds)
	return step(t, out.State, model.DaemonsLoaded{Request: load.Request, Entries: manageEntries, ConfigPath: "/kata/config.toml"}).State
}

func TestTheDaemonsFooterAndHelpNameTheManageKeys(t *testing.T) {
	s := daemonsManaged(t)
	wantText(t, "the footer", footerText(t, s), "n", "new", "e", "edit", "x", "remove")
	wantText(t, "the help sheet", shownText(t, step(t, s, key("?")).State), "Add a daemon", "Edit the daemon", "Remove the daemon")
	if empty := step(t, daemonsOpened(t, projectPane(t, 160, boardCard("a"))).State, key("e")); empty.State.Model.TopCard() != model.NoCard {
		t.Errorf("e before the catalog is read opened card %v", empty.State.Model.TopCard())
	}
}

func TestNAddsARemoteDaemonWithATokenVariable(t *testing.T) {
	s := step(t, daemonsManaged(t), key("n")).State
	if s.Model.TopCard() != model.CardDaemon || s.Model.CurrentMode() != model.ModeDaemonForm {
		t.Fatalf("n gave card %v and mode %q", s.Model.TopCard(), s.Model.CurrentMode())
	}
	wantShown(t, "the card", s, "Add daemon", "Name", "URL")
	wantText(t, "the choices", wireTextOf(t, s), "Runs", "Remote", "Local", "Token", "Environment variable", "Inline", "Insecure URL")
	wantNotShown(t, "the card", s, "Variable", "Secret")
	s = typeText(t, s, "staging")
	s = step(t, s, key("tab")).State // Runs
	s = step(t, s, key("tab")).State // URL
	s = typeText(t, s, "https://staging.example:7474")
	s = step(t, s, key("tab")).State // Token
	s = step(t, s, key("space")).State
	wantText(t, "a token of kind variable", wireTextOf(t, s), "Variable")
	s = step(t, s, key("tab")).State
	s = typeText(t, s, "STAGING_TOKEN")
	out := step(t, s, key("enter"))
	cmd, ok := askedSaveDaemon(out.Cmds)
	if !ok || cmd.Old != "" || cmd.Name != "staging" || cmd.URL != "https://staging.example:7474" || cmd.TokenEnv != "STAGING_TOKEN" || cmd.Token != "" || cmd.Local || cmd.AllowInsecure {
		t.Fatalf("Enter asked %#v", out.Cmds)
	}
	wantShown(t, "while it saves", out.State, "Saving…")

	saved := step(t, out.State, model.DaemonSaved{Request: cmd.Request, Name: "staging"})
	m := saved.State.Model
	if m.TopCard() != model.NoCard || m.Notice != "Saved staging." {
		t.Errorf("card %v, notice %q", m.TopCard(), m.Notice)
	}
	if _, ok := askedDaemons(saved.Cmds); !ok {
		t.Errorf("a save asked %#v, want the catalog read again", saved.Cmds)
	}
	load, _ := askedDaemons(saved.Cmds)
	reread := step(t, saved.State, model.DaemonsLoaded{Request: load.Request, Entries: append(manageEntries[:3:3], model.DaemonEntry{Name: "staging", Endpoint: "staging.example:7474"})})
	if got := reread.State.Model.SelectedDaemon(); got == nil || got.Name != "staging" {
		t.Errorf("the cursor is on %+v, want the saved daemon", got)
	}
}

func TestALocalDaemonHasNoURLTokenOrInsecureField(t *testing.T) {
	s := step(t, daemonsManaged(t), key("n")).State
	s = typeText(t, s, "box")
	s = step(t, s, key("tab")).State
	s = step(t, s, key("space")).State
	if text := wireTextOf(t, step(t, s, key("Tab")).State); strings.Contains(text, "layer.daemon.url") || strings.Contains(text, "Insecure URL") {
		t.Errorf("a local entry still draws its remote fields: %s", text)
	}
	out := step(t, s, key("enter"))
	cmd, ok := askedSaveDaemon(out.Cmds)
	if !ok || !cmd.Local || cmd.Name != "box" || cmd.URL != "" {
		t.Fatalf("Enter asked %#v", out.Cmds)
	}
}

func TestTheCardRefusesWhatKatasConfigWouldRefuse(t *testing.T) {
	for name, c := range map[string]struct {
		typed []string
		want  string
	}{
		"no name":      {nil, "Cannot save the daemon: the name is required."},
		"a taken name": {[]string{"home"}, "Cannot save the daemon: another daemon has that name."},
		"a bad url":    {[]string{"x", "", "not a url"}, "Cannot save the daemon: the URL must start with http:// or https:// and name a host."},
	} {
		s := step(t, daemonsManaged(t), key("n")).State
		for i, text := range c.typed {
			if i > 0 {
				s = step(t, s, key("tab")).State
			}
			if text != "" {
				s = typeText(t, s, text)
			}
		}
		out := step(t, s, key("enter"))
		if len(out.Cmds) != 0 {
			t.Errorf("%s: asked %#v", name, out.Cmds)
		}
		wantShown(t, name, out.State, c.want)
		if out.State.Model.TopCard() != model.CardDaemon {
			t.Errorf("%s: the card closed", name)
		}
	}
}

func TestEPrefillsTheSelectedDaemonAndAnEmptyInlineTokenKeepsIt(t *testing.T) {
	s := step(t, daemonsManaged(t), key("j")).State // lab: inline token, insecure
	s = step(t, s, key("e")).State
	v := s.Model.DaemonForm.Values
	if v.Name != "lab" || v.URL != "http://10.0.0.5:7474" || v.Auth != model.AuthInline || v.Secret != "" || !v.Insecure || s.Model.DaemonForm.Old != "lab" {
		t.Fatalf("values %+v, old %q", v, s.Model.DaemonForm.Old)
	}
	wantShown(t, "the card", s, "Edit daemon")
	out := step(t, s, key("enter"))
	cmd, ok := askedSaveDaemon(out.Cmds)
	if !ok || cmd.Old != "lab" || !cmd.KeepToken || cmd.Token != "" || !cmd.AllowInsecure {
		t.Fatalf("Enter asked %#v", out.Cmds)
	}

	envForm := step(t, step(t, daemonsManaged(t), key("e")).State, key("tab")).State // work: variable
	if v := envForm.Model.DaemonForm.Values; v.Auth != model.AuthEnv || v.Secret != "WORK_TOKEN" {
		t.Errorf("work prefilled %+v", v)
	}
}

func TestTheDaemonThisPaneUsesKeepsItsNameAndCannotBeRemoved(t *testing.T) {
	s := daemonsManaged(t) // the cursor starts on work, the current daemon
	renamed := step(t, typeText(t, step(t, s, key("e")).State, "2"), key("enter"))
	if len(renamed.Cmds) != 0 {
		t.Errorf("a rename of the current daemon asked %#v", renamed.Cmds)
	}
	wantShown(t, "the refusal", renamed.State, "Cannot save the daemon: this Pane uses it, so it keeps its name.")

	removed := step(t, s, key("x"))
	m := removed.State.Model
	if m.TopCard() != model.NoCard || m.Notice != "Cannot remove work: this Pane uses it." || m.NoticeKind != model.NoticeFail {
		t.Errorf("x on the current daemon: card %v, notice %q", m.TopCard(), m.Notice)
	}
}

func TestXAsksBeforeRemovingAndSendsTheRemoval(t *testing.T) {
	s := step(t, step(t, daemonsManaged(t), key("k")).State, key("x")).State // home
	if s.Model.TopCard() != model.CardDaemonRemove || s.Model.CurrentMode() != model.ModeDaemonRemove {
		t.Fatalf("x gave card %v and mode %q", s.Model.TopCard(), s.Model.CurrentMode())
	}
	wantShown(t, "the question", s, "Remove daemon", "Remove home?", "Its entry leaves config.toml.")
	if kept := step(t, s, key("n")); kept.State.Model.TopCard() != model.NoCard || len(kept.Cmds) != 0 {
		t.Errorf("n gave card %v and %#v", kept.State.Model.TopCard(), kept.Cmds)
	}
	out := step(t, s, key("y"))
	cmd, ok := asked[model.RemoveDaemonCmd](out.Cmds)
	if !ok || cmd.Name != "home" {
		t.Fatalf("y asked %#v", out.Cmds)
	}
	failed := step(t, out.State, model.DaemonRemoved{Request: cmd.Request, Name: "home", Err: errors.New("active_daemon \"home\" is not in daemon catalog")})
	if failed.State.Model.TopCard() != model.CardDaemonRemove || failed.State.Model.DaemonRemove.Saving {
		t.Error("a refused removal closed the question or left it saving")
	}
	wantShown(t, "the refusal", failed.State, `Cannot remove home: active_daemon "home" is not in daemon catalog.`)

	done := step(t, out.State, model.DaemonRemoved{Request: cmd.Request, Name: "home"})
	if done.State.Model.TopCard() != model.NoCard || done.State.Model.Notice != "Removed home." {
		t.Errorf("card %v, notice %q", done.State.Model.TopCard(), done.State.Model.Notice)
	}
	if _, ok := askedDaemons(done.Cmds); !ok {
		t.Errorf("a removal asked %#v, want the catalog read again", done.Cmds)
	}
}

func TestAFailedSaveKeepsTheCardWithKatasReason(t *testing.T) {
	s := step(t, daemonsManaged(t), key("n")).State
	s = typeText(t, s, "x")
	s = step(t, step(t, s, key("tab")).State, key("tab")).State
	out := step(t, typeText(t, s, "https://x.example"), key("enter"))
	cmd, _ := askedSaveDaemon(out.Cmds)
	failed := step(t, out.State, model.DaemonSaved{Request: cmd.Request, Name: "x", Err: errors.New("file changed since it was read")})
	if failed.State.Model.TopCard() != model.CardDaemon || failed.State.Model.DaemonForm.Saving {
		t.Fatal("the card closed or is still saving")
	}
	if !strings.Contains(shownText(t, failed.State), "Cannot save the daemon: file changed since it was read.") {
		t.Errorf("no reason shown: %s", shownText(t, failed.State))
	}
}

func TestCtrlUClearsTheNameSoATakenOneIsRefused(t *testing.T) {
	s := step(t, step(t, daemonsManaged(t), key("j")).State, key("e")).State
	s = step(t, s, key("ctrl+u")).State
	if got := s.Model.DaemonForm.Values.Name; got != "" {
		t.Fatalf("ctrl+u left the name %q", got)
	}
	out := step(t, typeText(t, s, "home"), key("enter"))
	if len(out.Cmds) != 0 {
		t.Errorf("a taken name asked %#v", out.Cmds)
	}
	wantShown(t, "the refusal", out.State, "Cannot save the daemon: another daemon has that name.")
}

func TestTheCardErrorSitsInTheFootUnderItsLayer(t *testing.T) {
	s := step(t, daemonsManaged(t), key("n")).State
	out := step(t, s, key("enter"))
	if text, ok := nodeText(t, out.State, "layer.daemon.foot.error"); !ok || text != "Cannot save the daemon: the name is required." {
		t.Errorf("foot error %q, %v", text, ok)
	}
}

func TestChangingATokenFromVariableToInlineStartsWithAnEmptySecret(t *testing.T) {
	s := step(t, daemonsManaged(t), key("e")).State // work: token variable WORK_TOKEN
	s = step(t, s, key("tab")).State                // Runs
	s = step(t, s, key("tab")).State                // URL
	s = step(t, s, key("tab")).State                // Token
	s = step(t, s, key("space")).State              // Inline
	if v := s.Model.DaemonForm.Values; v.Auth != model.AuthInline || v.Secret != "" {
		t.Fatalf("inline token field holds %+v, want it empty", v)
	}
	s = step(t, s, key("tab")).State
	s = typeText(t, s, "pasted")
	cmd, ok := askedSaveDaemon(step(t, s, key("enter")).Cmds)
	if !ok || cmd.Token != "pasted" || cmd.TokenEnv != "" {
		t.Fatalf("Enter asked %#v, want the token alone", cmd)
	}

	back := step(t, step(t, s, key("shift+tab")).State, key("space")).State // None
	back = step(t, back, key("space")).State                                // Environment variable
	if v := back.Model.DaemonForm.Values; v.Auth != model.AuthEnv || v.Secret != "" {
		t.Errorf("after a round trip the variable field holds %+v, want it empty", v)
	}
}
