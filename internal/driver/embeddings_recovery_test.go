package driver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

var (
	indexingFailed = model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200, Failed: true}}
	indexingBusy   = model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200}}
)

// cardClosedMidTurnOn is the question answered with y on the Config Page and the card closed before kata answers.
func cardClosedMidTurnOn(t *testing.T) driver.State {
	t.Helper()
	s := turnedOn(t, embeddingsCardOpen(t)).State
	s = step(t, s, key("escape")).State
	if s.Model.TopCard() != model.NoCard {
		t.Fatal("esc did not close the card while the turn-on is out")
	}
	return s
}

func TestARefusedTurnOnAfterTheCardWasClosedShowsOnTheConfigPage(t *testing.T) {
	s := cardClosedMidTurnOn(t)
	out := step(t, s, model.EmbeddingsTurnedOn{Err: errors.New("kata: daemon administration required"), Connection: s.Model.Connection})
	wantText(t, "Config Page", wireTextOf(t, out.State), "Search by meaning was not turned on: kata: daemon administration required")
	if c := out.State.Model.ConfigPage; !c.MessageErr {
		t.Errorf("message %q not marked as an error", c.Message)
	}
	if !hasCmd[model.LoadEmbeddingsStatus](out.Cmds) {
		t.Errorf("a refusal sent %#v, want the status read again", out.Cmds)
	}
}

func TestARefusedTurnOnAfterTheCardWasClosedOnTheSearchLineIsANotice(t *testing.T) {
	out := searchedByWords(t)
	s := statusAnswered(t, out.State, lastRead(t, out.Cmds), offWithDownload).State
	s = step(t, s, key("ctrl+o")).State
	s = turnedOn(t, s).State
	s = step(t, s, key("escape")).State
	if s.Model.EmbeddingsCard.Open {
		t.Fatal("esc did not close the card")
	}
	done := step(t, s, model.EmbeddingsTurnedOn{Err: errors.New("kata is unreachable"), Connection: s.Model.Connection})
	if got := done.State.Model.Notice; got != "Search by meaning was not turned on: kata is unreachable" {
		t.Errorf("notice %q, want kata's reason", got)
	}
}

func TestAFailedReadKeepsPollingWhileTheDownloadRunsAndRecoversOnTheNextGoodRead(t *testing.T) {
	s, ask := embeddingsPane(t)
	out := statusAnswered(t, s, ask, downloading)
	poll := lastRead(t, out.Cmds)
	failed := step(t, out.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Err: errors.New("connection refused")})
	retry := lastRead(t, failed.Cmds)
	if retry.After != 2*time.Second {
		t.Fatalf("retry after %v, want the poll pause", retry.After)
	}
	done := step(t, failed.State, model.EmbeddingsStatusLoaded{Request: retry.Request, Status: restartOwed})
	if hasCmd[model.LoadEmbeddingsStatus](done.Cmds) {
		t.Errorf("a finished job was read again: %#v", done.Cmds)
	}
	wantText(t, "row", wireTextOf(t, done.State), "set up: needs a kata daemon restart")
}

func TestAFailedFirstReadIsFinal(t *testing.T) {
	s, ask := embeddingsPane(t)
	out := step(t, s, model.EmbeddingsStatusLoaded{Request: ask.Request, Err: errors.New("404 not found")})
	if hasCmd[model.LoadEmbeddingsStatus](out.Cmds) {
		t.Errorf("an old daemon's 404 was read again: %#v", out.Cmds)
	}
}

func TestAFailedReadRightAfterARestartAnswerKeepsPollingUntilTheDaemonIsBack(t *testing.T) {
	sent := step(t, restartAsked(t), key("y"))
	answered := step(t, sent.State, model.KataDaemonRestarted{Connection: sent.State.Model.Connection})
	first := lastRead(t, answered.Cmds)
	failed := step(t, answered.State, model.EmbeddingsStatusLoaded{Request: first.Request, Err: errors.New("connection refused")})
	retry := lastRead(t, failed.Cmds)
	if retry.After != 2*time.Second {
		t.Fatalf("retry after %v, want the poll pause", retry.After)
	}
	back := step(t, failed.State, model.EmbeddingsStatusLoaded{Request: retry.Request, Status: indexingBusy})
	if next := lastRead(t, back.Cmds); next.After != 2*time.Second {
		t.Errorf("indexing is polled after %v, want the poll pause", next.After)
	}
	wantText(t, "card", wireTextOf(t, back.State), "indexing 40 / 200 Issues")
}

func TestTheRowAfterADocbankSwitchIsReadAgainSoAnOwedRestartIsGone(t *testing.T) {
	base := projectPane(t, 160, boardCard("a"))
	base.Model.Daemon = model.Daemon{Name: "home", Kind: model.DaemonLocal}
	opened := configOpened(t, base)
	ask, _ := askedEmbeddings(opened.Cmds)
	files, _ := askedConfigFiles(opened.Cmds)
	s := step(t, opened.State, model.ConfigFilesLoaded{Request: files.Request, Folder: "/cfg/tern-kata", Files: pinnedDocbankRows()}).State
	s = statusAnswered(t, s, ask, restartOwed).State
	wantText(t, "row before", wireTextOf(t, s), "set up: needs a kata daemon restart")
	for s.Model.SelectedConfigFile().Key != model.ConfigDocbank {
		s = step(t, s, key("j")).State
	}
	asked := step(t, s, key("s")).State
	sent := step(t, asked, key("y"))
	done := step(t, sent.State, model.DocbankSwitched{})
	read := lastRead(t, done.Cmds)
	after := step(t, done.State, model.EmbeddingsStatusLoaded{Request: read.Request, Status: indexingBusy})
	text := wireTextOf(t, after.State)
	wantText(t, "row after", text, "indexing 40 / 200 Issues")
	if want := "needs a kata daemon restart"; strings.Contains(text, want) {
		t.Errorf("row still says %q: %s", want, text)
	}
}

func TestTheRowIsBrokenWhenTheJobOrTheIndexingFailedAndLoadsWhileOnOrIndexing(t *testing.T) {
	for name, tc := range map[string]struct {
		status model.EmbeddingsStatus
		want   model.ConfigFileStatus
	}{
		"job failed":      {jobFailed, model.ConfigBroken},
		"indexing failed": {indexingFailed, model.ConfigBroken},
		"indexing":        {indexingBusy, model.ConfigLoads},
		"on":              {model.EmbeddingsStatus{Provider: model.EmbeddingsOllama}, model.ConfigLoads},
	} {
		f := embeddingsRowOn(t, tc.status).Model.SelectedConfigFile()
		if f.Status != tc.want {
			t.Errorf("%s: row status %v, want %v", name, f.Status, tc.want)
		}
	}
}

func TestPollingFromTheSearchLineFollowsTheDownloadAndStopsWhenTheCardIsClosed(t *testing.T) {
	out := searchedByWords(t)
	s := statusAnswered(t, out.State, lastRead(t, out.Cmds), offWithDownload).State
	s = turnedOn(t, step(t, s, key("ctrl+o")).State).State
	started := step(t, s, model.EmbeddingsTurnedOn{Status: downloading, Connection: s.Model.Connection})
	poll := lastRead(t, started.Cmds)
	if poll.After != 2*time.Second {
		t.Fatalf("first poll after %v, want 2s", poll.After)
	}
	closed := step(t, started.State, key("escape")).State
	next := step(t, closed, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: downloading})
	if hasCmd[model.LoadEmbeddingsStatus](next.Cmds) {
		t.Errorf("a poll was asked after the card closed: %#v", next.Cmds)
	}
}
