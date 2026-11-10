package driver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func askedEmbeddings(cmds []model.Cmd) (model.LoadEmbeddingsStatus, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadEmbeddingsStatus); ok {
			return load, true
		}
	}
	return model.LoadEmbeddingsStatus{}, false
}

var embeddingsFilesFixture = []model.ConfigFile{
	{Key: model.ConfigLanes, Title: "lanes.toml", Path: "/cfg/tern-kata/lanes.toml", Summary: "no lanes"},
	{Key: model.ConfigDocbank, Title: "Docbank (kata's config.toml)", Path: "/kata/config.toml", Summary: "not set up"},
	{Key: model.ConfigKata, Title: "config.toml (kata's, read-only)", Path: "/kata/config.toml", Summary: "1 daemon"},
}

// embeddingsPane is the Config Page opened on a Pane connected to daemon "home", its rows answered and its status read asked but
// not yet answered.
func embeddingsPane(t *testing.T) (driver.State, model.LoadEmbeddingsStatus) {
	t.Helper()
	s := projectPane(t, 160, boardCard("a"))
	s.Model.Daemon = model.Daemon{Name: "home", Kind: model.DaemonLocal}
	out := configOpened(t, s)
	status, ok := askedEmbeddings(out.Cmds)
	if !ok {
		t.Fatalf("entering the Config Page sent %#v, want a read of the search by meaning status", out.Cmds)
	}
	files, _ := askedConfigFiles(out.Cmds)
	return step(t, out.State, model.ConfigFilesLoaded{Request: files.Request, Folder: "/cfg/tern-kata", Files: embeddingsFilesFixture}).State, status
}

func statusAnswered(t *testing.T, s driver.State, ask model.LoadEmbeddingsStatus, status model.EmbeddingsStatus) driver.Output {
	t.Helper()
	return step(t, s, model.EmbeddingsStatusLoaded{Request: ask.Request, Status: status})
}

func TestConfigPageSearchByMeaningRowSaysWhatKataReportsForTheConnectedDaemon(t *testing.T) {
	for name, tc := range map[string]struct {
		status model.EmbeddingsStatus
		want   string
	}{
		"off with a download":         {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true}, "off: kata searches by words · about 290 MB to download"},
		"off with Ollama":             {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, OllamaAvailable: true, DownloadBytes: 290_400_000, CanChange: true}, "off: kata searches by words · your Ollama will be used"},
		"off for a non-administrator": {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000}, "off: turning it on needs a daemon administrator"},
		"downloading":                 {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading, Downloaded: 120_000_000, Total: 290_000_000}}, "downloading 120 / 290 MB"},
		"restart owed":                {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}}, "set up: needs a kata daemon restart"},
		"job failed":                  {model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsFailed, Error: "disk full\nfree some space"}}, "turning it on failed: disk full free some space"},
		"indexing with an ETA":        {model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200, ETA: 150 * time.Second}}, "indexing 40 / 200 Issues · about 3 min"},
		"indexing without an ETA":     {model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200}}, "indexing 40 / 200 Issues"},
		"indexing failed":             {model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200, Failed: true}}, "indexing failed"},
		"built-in":                    {model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn}, "on · built-in model"},
		"Ollama":                      {model.EmbeddingsStatus{Provider: model.EmbeddingsOllama}, "on · your Ollama"},
		"another provider":            {model.EmbeddingsStatus{Provider: model.EmbeddingsOther}, "on · another provider (kata's config.toml)"},
	} {
		s, ask := embeddingsPane(t)
		text := wireTextOf(t, statusAnswered(t, s, ask, tc.status).State)
		wantText(t, name, text, "Search by meaning (kata daemon: home)", tc.want, "3 files in /cfg/tern-kata")
		docbank, row, kata := strings.Index(text, "Docbank (kata's"), strings.Index(text, "Search by meaning (kata daemon"), strings.Index(text, "config.toml (kata's, read-only)")
		if !(docbank < row && row < kata) {
			t.Errorf("%s: the row is not between the Docbank row and kata's config.toml row: %s", name, text)
		}
	}
}

func TestConfigPageSearchByMeaningRowShowsKatasErrorWhenTheStatusCannotBeRead(t *testing.T) {
	s, ask := embeddingsPane(t)
	out := step(t, s, model.EmbeddingsStatusLoaded{Request: ask.Request, Err: errors.New("local embeddings setup is unavailable")})
	wantText(t, "a failed read", wireTextOf(t, out.State), "kata cannot report search by meaning here: local embeddings setup is unavailable")
	if _, again := askedEmbeddings(out.Cmds); again {
		t.Errorf("a failed read asked again: %#v", out.Cmds)
	}
}

func TestConfigPageReadsTheStatusAgainEveryTwoSecondsOnlyWhileKataIsBusyAndThePageIsShown(t *testing.T) {
	downloading := model.EmbeddingsStatus{Provider: model.EmbeddingsOff, Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading, Downloaded: 1, Total: 2}}
	s, ask := embeddingsPane(t)
	out := statusAnswered(t, s, ask, downloading)
	next, ok := askedEmbeddings(out.Cmds)
	if !ok || next.After != 2*time.Second || next.Request == ask.Request {
		t.Fatalf("a download in progress sent %#v, want a new read after 2s", out.Cmds)
	}
	indexing := model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{Done: 1, Total: 9}}
	out = statusAnswered(t, out.State, next, indexing)
	next2, ok := askedEmbeddings(out.Cmds)
	if !ok || next2.After != 2*time.Second {
		t.Fatalf("indexing under way sent %#v, want a new read after 2s", out.Cmds)
	}
	done := statusAnswered(t, out.State, next2, model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn})
	if _, again := askedEmbeddings(done.Cmds); again {
		t.Fatalf("a finished setup asked again: %#v", done.Cmds)
	}
	wantText(t, "the finished setup", wireTextOf(t, done.State), "on · built-in model")

	// Leaving the Page stops the polling: the answer that comes then is dropped and asks nothing.
	left := step(t, out.State, key("escape"))
	if left.State.Model.Page == model.PageConfig {
		t.Fatalf("escape left the Page on %v", left.State.Model.Page)
	}
	late := statusAnswered(t, left.State, next2, indexing)
	if _, again := askedEmbeddings(late.Cmds); again {
		t.Errorf("an answer after leaving the Page asked again: %#v", late.Cmds)
	}
}

func TestConfigPageDropsALateStatusAnswer(t *testing.T) {
	s, first := embeddingsPane(t)
	left := step(t, s, key("escape")).State
	second := configOpened(t, left)
	secondAsk, _ := askedEmbeddings(second.Cmds)
	if secondAsk.Request == first.Request {
		t.Fatalf("both entries asked request %d", first.Request)
	}
	files, _ := askedConfigFiles(second.Cmds)
	s = step(t, second.State, model.ConfigFilesLoaded{Request: files.Request, Folder: "/cfg/tern-kata", Files: embeddingsFilesFixture}).State
	s = statusAnswered(t, s, secondAsk, model.EmbeddingsStatus{Provider: model.EmbeddingsOllama}).State
	stale := statusAnswered(t, s, first, model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true})
	text := wireTextOf(t, stale.State)
	wantText(t, "after a late answer", text, "on · your Ollama")
	if strings.Contains(text, "off: kata searches by words") {
		t.Errorf("a late answer replaced the newer one: %s", text)
	}
}

func TestConfigPageSearchByMeaningRowFollowsADaemonSwitch(t *testing.T) {
	s, ask := embeddingsPane(t)
	s = statusAnswered(t, s, ask, model.EmbeddingsStatus{Provider: model.EmbeddingsOllama}).State
	wantText(t, "before the switch", wireTextOf(t, s), "Search by meaning (kata daemon: home)", "on · your Ollama")

	daemons, asked := switchOut(t)
	s.Model.Daemons = daemons
	switched := step(t, s, model.DaemonSwitched{Request: asked.Request, Name: "work", Daemon: model.Daemon{Name: "work", Kind: model.DaemonRemote}})
	out := configOpened(t, switched.State)
	status, _ := askedEmbeddings(out.Cmds)
	files, _ := askedConfigFiles(out.Cmds)
	s = step(t, out.State, model.ConfigFilesLoaded{Request: files.Request, Folder: "/cfg/tern-kata", Files: embeddingsFilesFixture}).State
	if _, listed := embeddingsRowOf(s); listed {
		t.Errorf("the old daemon's row shows before the new daemon answers: %+v", s.Model.ConfigPage.Files)
	}
	s = statusAnswered(t, s, status, model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_000_000, CanChange: true, Remote: true}).State
	row, listed := embeddingsRowOf(s)
	if !listed || row.Title != "Search by meaning (kata daemon: work)" || row.Summary != "off: kata searches by words · about 290 MB to download" {
		t.Errorf("row %+v, want the new daemon's name and answer", row)
	}
}

func embeddingsRowOf(s driver.State) (model.ConfigFile, bool) {
	for _, f := range s.Model.ConfigPage.Files {
		if f.Key == model.ConfigEmbeddings {
			return f, true
		}
	}
	return model.ConfigFile{}, false
}
