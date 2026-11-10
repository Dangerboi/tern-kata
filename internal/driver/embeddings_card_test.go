package driver_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

var (
	offWithDownload = model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true}
	offWithOllama   = model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, OllamaAvailable: true, CanChange: true}
	downloading     = model.EmbeddingsStatus{Provider: model.EmbeddingsOff, Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading, Downloaded: 120_000_000, Total: 290_000_000}}
	restartOwed     = model.EmbeddingsStatus{Provider: model.EmbeddingsOff, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}}
	jobFailed       = model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, DownloadBytes: 290_400_000, Job: &model.EmbeddingsJob{State: model.EmbeddingsFailed, Error: "disk full"}}
)

// embeddingsRowOn is the Config Page with the status answered and the cursor on the Search by meaning row.
func embeddingsRowOn(t *testing.T, status model.EmbeddingsStatus) driver.State {
	t.Helper()
	s, ask := embeddingsPane(t, embeddingsFilesFixture)
	s = statusAnswered(t, s, ask, status).State
	for range 2 {
		s = step(t, s, key("j")).State
	}
	if f := s.Model.SelectedConfigFile(); f == nil || f.Key != model.ConfigEmbeddings {
		t.Fatalf("cursor on %+v, want the Search by meaning row", f)
	}
	return s
}

func embeddingsCardOpen(t *testing.T) driver.State {
	t.Helper()
	return step(t, embeddingsRowOn(t, offWithDownload), key("s")).State
}

// turnedOn sends the question's y and returns the POST's output.
func turnedOn(t *testing.T, s driver.State) driver.Output {
	t.Helper()
	out := step(t, s, key("y"))
	if !hasCmd[model.TurnOnEmbeddings](out.Cmds) {
		t.Fatalf("y sent %#v, want the turn-on", out.Cmds)
	}
	return out
}

// lastRead is the status read among cmds; it fails the test when there is none.
func lastRead(t *testing.T, cmds []model.Cmd) model.LoadEmbeddingsStatus {
	t.Helper()
	read, ok := askedEmbeddings(cmds)
	if !ok {
		t.Fatalf("sent %#v, want a read of the search by meaning status", cmds)
	}
	return read
}

func offered(m model.Model, chord string) (footer, sheet bool) {
	for b := range model.BindingsForMode(m.CurrentMode(), true) {
		if b.Chord == chord && b.Offered(m) {
			footer = true
		}
	}
	for _, g := range model.HelpSheetOn(model.PageConfig, m) {
		for _, l := range g.Lines {
			if slices.Contains(l.Keys, chord) {
				sheet = true
			}
		}
	}
	return
}

func TestSOnTheSearchByMeaningRowOpensTheCardOnlyWhereKataWouldTakeATurnOn(t *testing.T) {
	for name, tc := range map[string]struct {
		status model.EmbeddingsStatus
		opens  bool
	}{
		"off and allowed":             {offWithDownload, true},
		"a failed job":                {jobFailed, true},
		"off for a non-administrator": {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000}, false},
		"downloading":                 {downloading, false},
		"on":                          {model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, CanChange: true}, false},
	} {
		s := embeddingsRowOn(t, tc.status)
		footer, sheet := offered(s.Model, "s")
		if footer != tc.opens || sheet != tc.opens {
			t.Errorf("%s: footer lists s %v, help sheet %v, want both %v", name, footer, sheet, tc.opens)
		}
		out := step(t, s, key("s"))
		if opened := out.State.Model.TopCard() == model.CardEmbeddings; opened != tc.opens {
			t.Errorf("%s: s opened the card %v, want %v", name, opened, tc.opens)
		}
		if len(out.Cmds) != 0 && !hasCmd[model.LoadEmbeddingsStatus](out.Cmds) {
			t.Errorf("%s: s sent %#v", name, out.Cmds)
		}
	}
}

func TestTheSearchByMeaningCardAsksInTheWordsOfWhatWillHappen(t *testing.T) {
	for name, tc := range map[string]struct {
		status model.EmbeddingsStatus
		want   string
	}{
		"your Ollama":     {offWithOllama, "Turn on search by meaning? kata will use your Ollama; nothing to download. y/n"},
		"this machine":    {offWithDownload, "Turn on search by meaning? kata downloads about 290 MB (shared by all Kenn apps) to the kata daemon's machine (this machine). Search stays by words until the daemon restarts. y/n"},
		"another machine": {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true, Remote: true}, "Turn on search by meaning? kata downloads about 290 MB (shared by all Kenn apps) to the kata daemon's machine (another machine). Search stays by words until the daemon restarts. y/n"},
		"a named host":    {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true, Remote: true, Host: "kata.example:7474"}, "Turn on search by meaning? kata downloads about 290 MB (shared by all Kenn apps) to the kata daemon's machine (kata.example:7474). Search stays by words until the daemon restarts. y/n"},
		"already there":   {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true}, "Turn on search by meaning? kata will use its built-in model, already on the kata daemon's machine; nothing to download. Search stays by words until the daemon restarts. y/n"},
	} {
		s := step(t, embeddingsRowOn(t, tc.status), key("s")).State
		wantText(t, name, wireTextOf(t, s), "Search by meaning", tc.want, "Turn on", "Not now")
	}
}

func TestNAndEscapeCloseTheQuestionAndSendNothing(t *testing.T) {
	for _, chord := range []string{"n", "escape"} {
		out := step(t, embeddingsCardOpen(t), key(chord))
		if len(out.Cmds) != 0 || out.State.Model.TopCard() != model.NoCard || !out.State.Model.OnConfig() {
			t.Errorf("%s sent %#v, card %v, on Config %v; want nothing sent and the Config Page again", chord, out.Cmds, out.State.Model.TopCard(), out.State.Model.OnConfig())
		}
	}
}

func TestYSendsExactlyOnePostAndTheCardWaitsForKatasAnswer(t *testing.T) {
	out := turnedOn(t, embeddingsCardOpen(t))
	if want := []model.Cmd{model.TurnOnEmbeddings{Workspace: out.State.Model.Workspace, Connection: out.State.Model.Connection}}; !equalCmds(out.Cmds, want) {
		t.Fatalf("y sent %#v, want %#v", out.Cmds, want)
	}
	wantText(t, "waiting", wireTextOf(t, out.State), "Turning it on…")
	for _, chords := range [][]string{{"escape", "s", "y"}, {"escape", "enter", "y"}} {
		inputs := make([]any, len(chords))
		for i, c := range chords {
			inputs[i] = key(c)
		}
		if again := feed(t, out.State, inputs...); hasCmd[model.TurnOnEmbeddings](again.Cmds) {
			t.Errorf("%s sent a second turn-on while the first is out", strings.Join(chords, ", "))
		}
	}
	if closed := step(t, out.State, key("escape")); closed.State.Model.TopCard() != model.NoCard {
		t.Error("esc does not close the card while the turn-on is out")
	}
}

func TestTheCardFollowsTheJobWithTheRowsWordsABarAndPolling(t *testing.T) {
	sent := turnedOn(t, embeddingsCardOpen(t))
	started := step(t, sent.State, model.EmbeddingsTurnedOn{Status: downloading, Connection: sent.State.Model.Connection})
	wantText(t, "download", wireTextOf(t, started.State), "downloading 120 / 290 MB", "kata keeps going if you close this.")
	if !hasProgress(t, started.State, "layer.embeddings.download") {
		t.Error("the download draws no progress bar")
	}
	poll := lastRead(t, started.Cmds)

	indexing := model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200, ETA: 150 * time.Second}}
	next := step(t, started.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: indexing})
	wantText(t, "indexing", wireTextOf(t, next.State), "indexing 40 / 200 Issues · about 3 min", "kata keeps going if you close this.")
	if !hasProgress(t, next.State, "layer.embeddings.indexing") || hasProgress(t, next.State, "layer.embeddings.download") {
		t.Error("indexing should draw its own bar and the download's none")
	}
	poll = lastRead(t, next.Cmds)

	done := step(t, next.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn}})
	text := wireTextOf(t, done.State)
	wantText(t, "done", text, "on · built-in model")
	if strings.Contains(text, "kata keeps going") {
		t.Errorf("a finished setup still says kata keeps going: %s", text)
	}
}

func TestARefusalShowsKatasReasonOnTheCardAndReadsTheStatusAgain(t *testing.T) {
	const reason = "a local embeddings setup is already running"
	sent := turnedOn(t, embeddingsCardOpen(t))
	out := step(t, sent.State, model.EmbeddingsTurnedOn{Err: errors.New(reason), Connection: sent.State.Model.Connection})
	wantText(t, "refusal", wireTextOf(t, out.State), reason)
	if out.State.Model.TopCard() != model.CardEmbeddings {
		t.Errorf("%q closed the card", reason)
	}
	read := lastRead(t, out.Cmds)
	if read.After != 0 {
		t.Errorf("the read after a refusal waits %v, want it at once", read.After)
	}
	if hasCmd[model.TurnOnEmbeddings](out.Cmds) {
		t.Error("a refusal sent another turn-on")
	}
}

func TestAFailedJobShowsKatasErrorAndSAsksAgain(t *testing.T) {
	sent := turnedOn(t, embeddingsCardOpen(t))
	started := step(t, sent.State, model.EmbeddingsTurnedOn{Status: downloading, Connection: sent.State.Model.Connection})
	poll := lastRead(t, started.Cmds)
	failed := step(t, started.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: jobFailed})
	wantText(t, "failure", wireTextOf(t, failed.State), "turning it on failed: disk full", "Turn on…")
	if failed.State.Model.CurrentMode() != model.ModeEmbeddingsOff {
		t.Fatalf("mode %s, want the mode where s asks again", failed.State.Model.CurrentMode())
	}
	if footer, _ := offered(failed.State.Model, "s"); !footer {
		t.Error("the footer does not list s on a failed job")
	}
	asked := step(t, failed.State, key("s"))
	if len(asked.Cmds) != 0 || asked.State.Model.CurrentMode() != model.ModeEmbeddingsAsk {
		t.Fatalf("s sent %#v in mode %s, want the question and nothing sent", asked.Cmds, asked.State.Model.CurrentMode())
	}
	if retried := turnedOn(t, asked.State); len(retried.Cmds) != 1 {
		t.Errorf("y after s sent %#v", retried.Cmds)
	}
}

func TestClosingTheCardMidJobLeavesTheRowPollingAndANewCardPicksTheJobUp(t *testing.T) {
	sent := turnedOn(t, embeddingsCardOpen(t))
	started := step(t, sent.State, model.EmbeddingsTurnedOn{Status: downloading, Connection: sent.State.Model.Connection})
	poll := lastRead(t, started.Cmds)
	closed := step(t, started.State, key("escape"))
	if closed.State.Model.TopCard() != model.NoCard {
		t.Fatal("esc did not close the card mid-job")
	}
	more := step(t, closed.State, model.EmbeddingsStatusLoaded{Request: poll.Request, Status: downloading})
	lastRead(t, more.Cmds)
	wantText(t, "row", wireTextOf(t, more.State), "downloading 120 / 290 MB")

	reopened := step(t, more.State, key("enter"))
	text := wireTextOf(t, reopened.State)
	wantText(t, "reopened", text, "downloading 120 / 290 MB", "kata keeps going if you close this.")
	if strings.Contains(text, "Turn on search by meaning?") || reopened.State.Model.CurrentMode() != model.ModeEmbeddings {
		t.Errorf("the reopened card asks again instead of following the job: mode %s, %s", reopened.State.Model.CurrentMode(), text)
	}
	if hasCmd[model.TurnOnEmbeddings](reopened.Cmds) {
		t.Error("reopening the card sent a turn-on")
	}
	lastRead(t, reopened.Cmds)
}

func TestAnAnswerToATurnOnFromAnEarlierDaemonIsDropped(t *testing.T) {
	sent := turnedOn(t, embeddingsCardOpen(t))
	late := step(t, sent.State, model.EmbeddingsTurnedOn{Status: downloading, Connection: sent.State.Model.Connection + 1})
	if late.State.Model.Embeddings.Status.Job != nil {
		t.Errorf("an answer of another connection was taken: %+v", late.State.Model.Embeddings.Status)
	}
}

func hasProgress(t *testing.T, s driver.State, id string) bool {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id && n.K == "progress" {
			return true
		}
	}
	return false
}
