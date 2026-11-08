package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

const (
	agentStarterFixture = "# actor = \"me\"\n# [projects.demo]\n# checkout = \"/x\"\n"
	agentFileFixture    = "name = \"builder\"\n"
)

// agentRows is every row with the agent.toml row holding text (a missing file when missing) and the starter.
func agentRows(text string, missing bool) []model.ConfigFile {
	rows := configAllRows()
	for i, f := range rows {
		if f.Key == model.ConfigAgent {
			rows[i].Text, rows[i].Starter = text, agentStarterFixture
			if !missing {
				rows[i].Status = model.ConfigLoads
			}
		}
	}
	return rows
}

func agentEditOpened(t *testing.T, text string, missing bool) driver.Output {
	t.Helper()
	s := configOn(t, projectPane(t, 160, boardCard("a")), agentRows(text, missing), model.ConfigAgent)
	out := step(t, s, key("e"))
	if out.Editor == nil {
		t.Fatalf("e on agent.toml opened no editor; cmds %#v", out.Cmds)
	}
	return out
}

func TestConfigAgentEditOpensTheFileInEditorWithItsOwnDraftAndATomlSuffix(t *testing.T) {
	out := agentEditOpened(t, agentFileFixture, false)
	if out.Editor.UID != model.AgentDraftUID || out.Editor.Text != agentFileFixture || out.Editor.Suffix != ".toml" {
		t.Errorf("e opened %#v, want the file in its own draft with a .toml suffix", out.Editor)
	}
	if out.Editor.UID == model.WorkerDraftUID {
		t.Errorf("agent.toml shares the worker's draft UID")
	}
}

func TestConfigAgentEditWithNoFileStartsFromTheStarter(t *testing.T) {
	out := agentEditOpened(t, "", true)
	if out.Editor.Text != agentStarterFixture {
		t.Errorf("e opened %q, want the starter", out.Editor.Text)
	}
	back := step(t, out.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: agentStarterFixture})
	if len(back.Cmds) != 0 {
		t.Errorf("a starter saved unchanged sent %#v", back.Cmds)
	}
}

func TestConfigEOnEveryOtherRowDoesNothing(t *testing.T) {
	for _, f := range configAllRows() {
		if f.Key == model.ConfigAgent {
			continue
		}
		s := configOn(t, projectPane(t, 160, boardCard("a")), agentRows(agentFileFixture, false), f.Key)
		out := step(t, s, key("e"))
		if out.Editor != nil || len(out.Cmds) != 0 || out.State.Model.Page != model.PageConfig {
			t.Errorf("%s: e gave editor %#v, cmds %#v, page %v", f.Key, out.Editor, out.Cmds, out.State.Model.Page)
		}
	}
}

func TestConfigAgentEditThatIsUnchangedOrBlankSendsNothing(t *testing.T) {
	opened := agentEditOpened(t, agentFileFixture, false)
	for name, text := range map[string]string{"unchanged": agentFileFixture, "unchanged with CRLF": "name = \"builder\"\r\n", "blank": " \n\n"} {
		back := step(t, opened.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: text})
		if len(back.Cmds) != 0 || back.State.Model.ConfigPage.Saving {
			t.Errorf("%s: sent %#v", name, back.Cmds)
		}
	}
}

func TestConfigAgentEditThatDoesNotLoadKeepsTheTextAndTheNextEStartsFromIt(t *testing.T) {
	opened := agentEditOpened(t, agentFileFixture, false)
	bad := agentFileFixture + "bogus = 1\n"
	sent := step(t, opened.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: bad})
	if want := []model.Cmd{model.SaveAgentConfig{Text: bad, Base: agentFileFixture}}; !equalCmds(sent.Cmds, want) {
		t.Fatalf("the edited text sent %#v, want %#v", sent.Cmds, want)
	}
	refused := step(t, sent.State, model.AgentConfigSaved{Text: bad, Err: errors.New("agent.toml: unknown key bogus")})
	wantText(t, "the Page", wireTextOf(t, refused.State), "unknown key bogus", "not saved", "your text is kept")
	if len(refused.Cmds) != 0 {
		t.Errorf("a refusal sent %#v", refused.Cmds)
	}
	again := step(t, refused.State, key("e"))
	if again.Editor == nil || again.Editor.Text != bad {
		t.Fatalf("e reopened %#v, want the refused text", again.Editor)
	}
}

func TestConfigAgentEditOfAFileThatChangedKeepsTheTextReadsAgainAndSaysSo(t *testing.T) {
	opened := agentEditOpened(t, agentFileFixture, false)
	mine := "name = \"mine\"\n"
	sent := step(t, opened.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: mine})
	refused := step(t, sent.State, model.AgentConfigSaved{Text: mine, Err: model.ErrAgentConfigChanged})
	wantText(t, "the Page", wireTextOf(t, refused.State), "changed while you edited", "lacks that change")
	load, ok := askedConfigFiles(refused.Cmds)
	if !ok {
		t.Fatalf("cmds %#v, want the rows read again", refused.Cmds)
	}
	theirs := "name = \"theirs\"\n"
	reread := step(t, refused.State, model.ConfigFilesLoaded{Request: load.Request, Folder: "/cfg/tern-kata", Files: agentRows(theirs, false)})
	again := step(t, reread.State, key("e"))
	if again.Editor == nil || again.Editor.Text != mine {
		t.Fatalf("e reopened %#v, want the kept text", again.Editor)
	}
	back := step(t, again.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: mine + "# more\n"})
	if want := []model.Cmd{model.SaveAgentConfig{Text: mine + "# more\n", Base: theirs}}; !equalCmds(back.Cmds, want) {
		t.Errorf("the second save sent %#v, want %#v", back.Cmds, want)
	}
}

func TestConfigAgentEditThatSavesReadsTheRowsAgainAndSaysAgentsUseItFromNow(t *testing.T) {
	opened := agentEditOpened(t, agentFileFixture, false)
	next := "name = \"renamed\"\n"
	sent := step(t, opened.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: next})
	done := step(t, sent.State, model.AgentConfigSaved{Text: next})
	load, ok := askedConfigFiles(done.Cmds)
	if !ok {
		t.Fatalf("cmds %#v, want the rows read again", done.Cmds)
	}
	wantText(t, "the Page", wireTextOf(t, done.State), "agent.toml saved; agents started from now on use it.")
	reread := step(t, done.State, model.ConfigFilesLoaded{Request: load.Request, Folder: "/cfg/tern-kata", Files: agentRows(next, false)})
	if again := step(t, reread.State, key("e")); again.Editor == nil || again.Editor.Text != next {
		t.Errorf("e after a save opened %#v, want the new file", again.Editor)
	}
}

func TestConfigAgentEditWithNoEditorSaysSoAndStaysOnThePage(t *testing.T) {
	opened := agentEditOpened(t, agentFileFixture, false)
	back := step(t, opened.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: agentFileFixture, Err: errors.New("no editor to open: set $VISUAL or $EDITOR")})
	if back.State.Model.Page != model.PageConfig || len(back.Cmds) != 0 {
		t.Errorf("page %v, cmds %#v", back.State.Model.Page, back.Cmds)
	}
	wantText(t, "the Page", wireTextOf(t, back.State), "no editor to open: set $VISUAL or $EDITOR")
}

func TestConfigAgentEditKeyIsIgnoredWhileASaveIsOut(t *testing.T) {
	opened := agentEditOpened(t, agentFileFixture, false)
	sent := step(t, opened.State, model.DraftEditorClosed{UID: model.AgentDraftUID, Text: "name = \"x\"\n"})
	again := step(t, sent.State, key("e"))
	if again.Editor != nil || len(again.Cmds) != 0 {
		t.Errorf("e while saving gave editor %#v, cmds %#v", again.Editor, again.Cmds)
	}
}

func TestConfigAgentRowOffersEditAsAButtonAndInTheFooter(t *testing.T) {
	s := configOn(t, projectPane(t, 160, boardCard("a")), agentRows(agentFileFixture, false), model.ConfigAgent)
	out := step(t, s, click(view.ConfigFileID(model.ConfigAgent)+".edit", view.ConfigActClick(model.ConfigAgent, model.ConfigEdit)))
	if out.Editor == nil || out.Editor.UID != model.AgentDraftUID {
		t.Errorf("a click on the row's edit button opened %#v", out.Editor)
	}
	found := false
	for b := range model.BindingsForMode(s.Model.CurrentMode(), true) {
		if b.Chord == "e" && b.Offered(s.Model) {
			found = b.Described(s.Model) == "edit agent.toml"
		}
	}
	if !found {
		t.Errorf("the footer does not offer e as editing agent.toml")
	}
}
