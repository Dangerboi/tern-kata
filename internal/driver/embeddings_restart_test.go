package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

var restartOwedRemote = model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Remote: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}}

// restartAsked is the card open on the restart question, reached by the job's status coming in while the card shows.
func restartAsked(t *testing.T) driver.State {
	t.Helper()
	sent := turnedOn(t, embeddingsCardOpen(t))
	started := step(t, sent.State, model.EmbeddingsTurnedOn{Status: downloading, Connection: sent.State.Model.Connection})
	poll := lastRead(t, started.Cmds)
	s := step(t, started.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: restartOwed}).State
	if s.Model.CurrentMode() != model.ModeEmbeddingsRestart {
		t.Fatalf("mode %s, want the restart question", s.Model.CurrentMode())
	}
	return s
}

// The restart question comes up whether the job reaches restart through kata's answer to the turn-on or through a later status read,
// and a job waiting for a restart is not polled.
func TestTheRestartQuestionAppearsWhenTheJobReachesRestartWithTheCardOpen(t *testing.T) {
	s := restartAsked(t)
	wantText(t, "question", wireTextOf(t, s), "Restart the kata daemon now? Other Panes, agents and the worker lose their connection for a moment and reconnect. y/n", "Restart now", "Not now")

	sent := turnedOn(t, embeddingsCardOpen(t))
	out := step(t, sent.State, model.EmbeddingsTurnedOn{Status: restartOwed, Connection: sent.State.Model.Connection})
	wantText(t, "restart after the turn-on", wireTextOf(t, out.State), model.RestartQuestion)
	if _, polled := askedEmbeddings(out.Cmds); polled {
		t.Errorf("a job waiting for a restart is polled: %#v", out.Cmds)
	}
	if out.State.Model.CurrentMode() != model.ModeEmbeddingsRestart {
		t.Errorf("mode %s, want the restart question", out.State.Model.CurrentMode())
	}
}

func TestNClosesTheRestartQuestionAndKeepsTheRestartNeeded(t *testing.T) {
	declined := step(t, restartAsked(t), key("n"))
	if len(declined.Cmds) != 0 || declined.State.Model.TopCard() != model.NoCard {
		t.Fatalf("n sent %#v, card %v; want nothing sent and the card closed", declined.Cmds, declined.State.Model.TopCard())
	}
	closed := declined.State
	wantText(t, "row", wireTextOf(t, closed), "set up: needs a kata daemon restart")
}

func TestACardOpenedWhileARestartIsOwedOnALocalDaemonOpensOnTheRestartQuestion(t *testing.T) {
	sent := turnedOn(t, embeddingsCardOpen(t))
	started := step(t, sent.State, model.EmbeddingsTurnedOn{Status: downloading, Connection: sent.State.Model.Connection})
	poll := lastRead(t, started.Cmds)
	closed := step(t, started.State, key("escape")).State
	if closed.Model.TopCard() != model.NoCard {
		t.Fatal("esc did not close the card mid-download")
	}
	arrived := step(t, closed, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: restartOwed}).State
	if arrived.Model.TopCard() != model.NoCard {
		t.Fatalf("the status reaching restart opened a card: %v", arrived.Model.TopCard())
	}
	opened := step(t, arrived, key("enter"))
	if opened.State.Model.CurrentMode() != model.ModeEmbeddingsRestart || hasCmd[model.RestartKataDaemon](opened.Cmds) {
		t.Errorf("enter gave mode %s and sent %#v, want the restart question and no restart", opened.State.Model.CurrentMode(), opened.Cmds)
	}
	wantText(t, "question", wireTextOf(t, opened.State), model.RestartQuestion)
}

func TestROnTheRowIsOfferedOnlyWhileARestartIsOwed(t *testing.T) {
	for name, tc := range map[string]struct {
		status model.EmbeddingsStatus
		offers bool
	}{
		"owed":        {restartOwed, true},
		"remote":      {restartOwedRemote, false},
		"off":         {offWithDownload, false},
		"downloading": {downloading, false},
	} {
		s := embeddingsRowOn(t, tc.status)
		footer, sheet := offered(s.Model, "r")
		if footer != tc.offers || sheet != tc.offers {
			t.Errorf("%s: footer lists r %v, help sheet %v, want both %v", name, footer, sheet, tc.offers)
		}
		if !tc.offers {
			if out := step(t, s, key("r")); out.State.Model.TopCard() == model.CardEmbeddings || len(out.Cmds) != 0 {
				t.Errorf("%s: r opened the card or sent %#v", name, out.Cmds)
			}
		}
	}
}

func TestYSendsExactlyOneRestartAndItsSuccessReadsTheStatusAgain(t *testing.T) {
	out := step(t, restartAsked(t), key("y"))
	want := []model.Cmd{model.RestartKataDaemon{Workspace: out.State.Model.Workspace, Connection: out.State.Model.Connection}}
	if !equalCmds(out.Cmds, want) {
		t.Fatalf("y sent %#v, want %#v", out.Cmds, want)
	}
	wantText(t, "restarting", wireTextOf(t, out.State), "Restarting the kata daemon…")
	for _, chords := range [][]string{{"escape", "enter", "y"}, {"escape", "r", "y"}} {
		inputs := make([]any, len(chords))
		for i, c := range chords {
			inputs[i] = key(c)
		}
		if again := feed(t, out.State, inputs...); hasCmd[model.RestartKataDaemon](again.Cmds) {
			t.Errorf("%s sent a second restart while the first is out", strings.Join(chords, ", "))
		}
	}
	done := step(t, out.State, model.KataDaemonRestarted{Connection: out.State.Model.Connection})
	if read := lastRead(t, done.Cmds); read.After != 0 {
		t.Errorf("read after %v, want at once", read.After)
	}
	if done.State.Model.Embeddings.Restarting {
		t.Error("the restart still counts as out after its answer")
	}
}

func TestAFailedRestartShowsKatasLineOnTheCardAndOffersNoSecondRestartUntilAsked(t *testing.T) {
	out := step(t, restartAsked(t), key("y"))
	failed := step(t, out.State, model.KataDaemonRestarted{Err: errors.New("daemon is not running"), Connection: out.State.Model.Connection})
	wantText(t, "failure", wireTextOf(t, failed.State), "The kata daemon was not restarted: daemon is not running", "set up: needs a kata daemon restart")
	if len(failed.Cmds) != 0 {
		t.Errorf("a failure sent %#v", failed.Cmds)
	}
	if again := step(t, step(t, failed.State, key("escape")).State, key("r")); again.State.Model.CurrentMode() != model.ModeEmbeddingsRestart {
		t.Errorf("r after a failure gave mode %s, want the question", again.State.Model.CurrentMode())
	}
}

func TestAFailedRestartAnsweringAfterTheCardClosedShowsOnTheConfigPage(t *testing.T) {
	out := step(t, restartAsked(t), key("y"))
	closed := step(t, out.State, key("escape")).State
	failed := step(t, closed, model.KataDaemonRestarted{Err: errors.New("daemon is not running"), Connection: closed.Model.Connection})
	if c := failed.State.Model.ConfigPage; !c.MessageErr || !strings.Contains(c.Message, "daemon is not running") {
		t.Errorf("message %q (error %v), want kata's line", c.Message, c.MessageErr)
	}
}

func TestARemoteDaemonGetsTheInstructionAndNeitherRNorTheQuestion(t *testing.T) {
	row := embeddingsRowOn(t, restartOwedRemote)
	wantText(t, "row", wireTextOf(t, row), "run `kata daemon restart` on that machine")
	opened := step(t, row, key("enter")).State
	wantText(t, "card", wireTextOf(t, opened), "run `kata daemon restart` on that machine")
	if opened.Model.CurrentMode() != model.ModeEmbeddings {
		t.Errorf("mode %s, want no question", opened.Model.CurrentMode())
	}

	remoteOff := model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true, Remote: true}
	sent := turnedOn(t, step(t, embeddingsRowOn(t, remoteOff), key("s")).State)
	started := step(t, sent.State, model.EmbeddingsTurnedOn{Status: model.EmbeddingsStatus{Provider: model.EmbeddingsOff, Remote: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading, Total: 10}}, Connection: sent.State.Model.Connection})
	poll := lastRead(t, started.Cmds)
	arrived := step(t, started.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: restartOwedRemote}).State
	if arrived.Model.CurrentMode() == model.ModeEmbeddingsRestart {
		t.Error("a remote daemon's restart state asked the question")
	}
	wantText(t, "arrived", wireTextOf(t, arrived), "run `kata daemon restart` on that machine")
}

func TestALateRestartAnswerOfAnEarlierConnectionIsDropped(t *testing.T) {
	out := step(t, restartAsked(t), key("y"))
	late := step(t, out.State, model.KataDaemonRestarted{Err: errors.New("old daemon"), Connection: out.State.Model.Connection + 1})
	if !late.State.Model.Embeddings.Restarting || len(late.Cmds) != 0 || late.State.Model.EmbeddingsCard.Err != nil {
		t.Errorf("an answer of another connection was taken: restarting %v, cmds %#v", late.State.Model.Embeddings.Restarting, late.Cmds)
	}
}
