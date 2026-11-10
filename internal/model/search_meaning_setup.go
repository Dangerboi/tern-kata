package model

import (
	"strconv"
	"time"
)

// Search by meaning needs embeddings on the kata daemon. This file is what the Pane knows of that setup: the status kata reports
// for the daemon the Pane is connected to, and the Config Page's row that words it, which opens the card and offers turning it on
// and restarting the daemon. Other places that act on the setup
// read the same EmbeddingsStatus.

// EmbeddingsProvider is who produces the vectors search by meaning ranks with, as kata names it.
type EmbeddingsProvider string

const (
	// EmbeddingsOff is no provider: kata searches by words.
	EmbeddingsOff     EmbeddingsProvider = "none"
	EmbeddingsOllama  EmbeddingsProvider = "ollama"
	EmbeddingsBuiltIn EmbeddingsProvider = "built_in"
	// EmbeddingsOther is a provider set in kata's config.toml that is neither Ollama nor kata's built-in model.
	EmbeddingsOther EmbeddingsProvider = "other"
)

// EmbeddingsJobState is where kata's setup job is.
type EmbeddingsJobState string

const (
	EmbeddingsDownloading EmbeddingsJobState = "downloading"
	// EmbeddingsRestart is config.toml written: kata uses the setup after its daemon restarts.
	EmbeddingsRestart EmbeddingsJobState = "restart"
	EmbeddingsFailed  EmbeddingsJobState = "failed"
)

// EmbeddingsJob is the setup job living in the daemon's memory. Downloaded and Total are bytes; Error is kata's reason when the
// job failed (kata words it generally for an account without daemon administration).
type EmbeddingsJob struct {
	State             EmbeddingsJobState
	Downloaded, Total int64
	Error             string
}

// EmbeddingsIndexing is kata embedding the Issues in the background. Done and Total are Issues; both are zero when kata withholds
// them from an account without daemon administration. ETA is zero when kata gives none.
type EmbeddingsIndexing struct {
	Done, Total int64
	ETA         time.Duration
	Failed      bool
}

// EmbeddingsStatus is kata's answer for one daemon. Job and Indexing are nil when there is none. CanChange says this account may
// turn search by meaning on. Daemon is the catalog name of the daemon the answer came from, "" for an unnamed connection, and Remote
// says it runs on another machine. Host is that machine as the Pane reaches it ("" when unknown or the daemon is local).
type EmbeddingsStatus struct {
	Provider        EmbeddingsProvider
	DownloadBytes   int64
	OllamaAvailable bool
	CanChange       bool
	Job             *EmbeddingsJob
	Indexing        *EmbeddingsIndexing
	Daemon          string
	Remote          bool
	Host            string
}

// Busy says kata is still working: a download runs or the indexing is under way, so the status is worth reading again soon. A
// failed indexing is not busy: it will not end by itself being read.
func (s EmbeddingsStatus) Busy() bool {
	if s.Job != nil && s.Job.State == EmbeddingsDownloading {
		return true
	}
	return s.Provider != EmbeddingsOff && s.Indexing != nil && !s.Indexing.Failed
}

// failed says the setup job failed or the indexing of a provider in use failed.
func (s EmbeddingsStatus) failed() bool {
	return s.Job != nil && s.Job.State == EmbeddingsFailed || s.Provider != EmbeddingsOff && s.Indexing != nil && s.Indexing.Failed
}

// RestartOwed says the setup job finished and waits for a kata daemon restart.
func (s EmbeddingsStatus) RestartOwed() bool { return s.Job != nil && s.Job.State == EmbeddingsRestart }

// megabytes is a size as kata words it: decimal megabytes, rounded.
func megabytes(bytes int64) string { return strconv.FormatInt((bytes+500_000)/1_000_000, 10) }

// Words is the Config row's text for the status.
func (s EmbeddingsStatus) Words() string {
	if s.Job != nil {
		switch s.Job.State {
		case EmbeddingsDownloading:
			if s.Job.Total > 0 {
				return "downloading " + megabytes(s.Job.Downloaded) + " / " + megabytes(s.Job.Total) + " MB"
			}
			return "downloading " + megabytes(s.Job.Downloaded) + " MB"
		case EmbeddingsRestart:
			if s.Remote {
				return "set up: needs a kata daemon restart · " + s.remoteRestartWords()
			}
			return "set up: needs a kata daemon restart"
		case EmbeddingsFailed:
			if reason := oneLine(s.Job.Error); reason != "" {
				return "turning it on failed: " + reason
			}
			return "turning it on failed"
		}
	}
	if s.Provider != EmbeddingsOff && s.Indexing != nil {
		return s.Indexing.words()
	}
	switch s.Provider {
	case EmbeddingsBuiltIn:
		return "on · built-in model"
	case EmbeddingsOllama:
		return "on · your Ollama"
	case EmbeddingsOther:
		return "on · another provider (kata's config.toml)"
	}
	if !s.CanChange {
		return "off: turning it on needs a daemon administrator"
	}
	switch {
	case s.OllamaAvailable:
		return "off: kata searches by words · your Ollama will be used"
	case s.DownloadBytes > 0:
		return "off: kata searches by words · about " + megabytes(s.DownloadBytes) + " MB to download"
	}
	return "off: kata searches by words"
}

func (i EmbeddingsIndexing) words() string {
	if i.Failed {
		return "indexing failed"
	}
	text := "indexing"
	if i.Total > 0 {
		text += " " + strconv.FormatInt(i.Done, 10) + " / " + strconv.FormatInt(i.Total, 10) + " Issues"
	}
	if i.ETA > 0 {
		text += " · about " + strconv.Itoa(max(1, int((i.ETA+time.Minute-1)/time.Minute))) + " min"
	}
	return text
}

// embeddingsPollEvery is how long after an answer that is still busy the Config Page reads the status again.
const embeddingsPollEvery = 2 * time.Second

// LoadEmbeddingsStatus asks the daemon the Pane is connected to for its search by meaning status, After a pause when After is set
// (the poll). Request numbers the asks: an answer to an older one is dropped.
type LoadEmbeddingsStatus struct {
	Workspace string
	Request   int64
	After     time.Duration
}

func (LoadEmbeddingsStatus) cmd() {}

// EmbeddingsStatusLoaded answers LoadEmbeddingsStatus. Err is kata's error line; an older daemon answers 404.
type EmbeddingsStatusLoaded struct {
	Request int64
	Status  EmbeddingsStatus
	Err     error
}

func (EmbeddingsStatusLoaded) msg() {}

// EmbeddingsSetup is the last status read from the connected daemon, wherever the Pane asked for it: the Config Page's row and the
// Search by meaning card show it, and so may a place outside the Config Page. Read says an answer has come, which is when the row
// shows. It is dropped with the connection, as it describes that daemon.
type EmbeddingsSetup struct {
	Read   bool
	Status EmbeddingsStatus
	Err    error
	// Turning says a turn-on is out and has not answered: nothing sends another.
	Turning bool
	// Restarting says a daemon restart is out and has not answered: nothing sends another.
	Restarting bool
	// returning says a daemon restart was just answered: a read that fails while the daemon comes back is asked again.
	returning bool
	// reads numbers the asks, so an answer to an older one is dropped.
	reads Latest
}

// forDaemon is the setup of a new connection: nothing read, and the answers still out dropped.
func (s EmbeddingsSetup) forDaemon() EmbeddingsSetup {
	return EmbeddingsSetup{reads: s.reads.Discarded()}
}

// TurnOnOffered says kata would take a turn-on from this account now: it may change the setup, nothing is configured, and no job
// is downloading or waiting for a restart. A failed job is offered again.
func (s EmbeddingsStatus) TurnOnOffered() bool {
	return s.CanChange && s.Provider == EmbeddingsOff && (s.Job == nil || s.Job.State == EmbeddingsFailed)
}

// embeddingsRow is the Config Page's Search by meaning row. It reads the daemon the Pane is connected to, so it is titled with that
// daemon's name and holds no path. enter opens the card; s turns it on where kata allows it.
func embeddingsRow(daemon string, s EmbeddingsSetup) ConfigFile {
	if daemon == "" {
		daemon = "default"
	}
	row := ConfigFile{Key: ConfigEmbeddings, Title: "Search by meaning (kata daemon: " + oneLine(daemon) + ")", Actions: []ConfigAction{ConfigOpen}}
	if s.Restarting {
		row.Summary = "restarting the kata daemon…"
		return row
	}
	switch {
	case s.Err != nil:
		row.Status, row.Err = ConfigBroken, s.Err.Error()
		row.Summary = "kata cannot report search by meaning here: " + oneLine(s.Err.Error())
	default:
		row.Summary = s.Status.Words()
		switch {
		case s.Status.failed():
			row.Status = ConfigBroken
		case s.Status.Provider != EmbeddingsOff && s.Status.Job == nil:
			row.Status = ConfigLoads
		}
		if s.Status.TurnOnOffered() {
			row.Actions = append(row.Actions, ConfigTurnOn)
		}
		if s.Status.RestartOwed() && !s.Status.Remote {
			row.Actions = append(row.Actions, ConfigRestart)
		}
	}
	return row
}

// askEmbeddings reads the status again, After a pause. The ask supersedes any read out.
func askEmbeddings(m *Model, after time.Duration) Cmd {
	return LoadEmbeddingsStatus{Workspace: m.Workspace, Request: m.Embeddings.reads.Ask(), After: after}
}

// remoteRestartWords is the instruction for a daemon on another machine, which the Pane cannot restart.
func (s EmbeddingsStatus) remoteRestartWords() string {
	if s.Host == "" {
		return "run `kata daemon restart` on that machine"
	}
	return "run `kata daemon restart` on " + s.Host
}

// withEmbeddingsRow words the Config row again from the status last read, when the rows are listed.
func (m Model) withEmbeddingsRow() Model {
	if len(m.ConfigPage.Files) > 0 {
		m.ConfigPage.setFiles(m.Daemon.Name, m.Embeddings, m.ConfigPage.Files)
	}
	return m
}

// embeddingsWatched says something is showing the status: the Config Page or the Search by meaning card, wherever it was opened.
// Only then is a busy status read again.
func (m Model) embeddingsWatched() bool { return m.OnConfig() || m.EmbeddingsCard.Open }

// withEmbeddingsRow puts row after the Docbank row (at the end when there is none), replacing the one already listed.
func withEmbeddingsRow(files []ConfigFile, row ConfigFile) []ConfigFile {
	out := make([]ConfigFile, 0, len(files)+1)
	for _, f := range files {
		if f.Key != ConfigEmbeddings {
			out = append(out, f)
		}
	}
	at := len(out)
	for i, f := range out {
		if f.Key == ConfigDocbank {
			at = i + 1
		}
	}
	return append(out[:at], append([]ConfigFile{row}, out[at:]...)...)
}

// embeddingsLoaded takes the status answer: an older one is dropped. A busy answer asks again after the poll pause while the Page
// or the card is shown.
func embeddingsLoaded(m Model, msg EmbeddingsStatusLoaded) (Model, []Cmd) {
	if !m.Embeddings.reads.IsCurrent(msg.Request) {
		return m, nil
	}
	return embeddingsRead(m, msg.Status, msg.Err)
}

// embeddingsRead keeps a status as the one last read and acts on it: the Config row and the card follow it, and a busy one is
// read again while something shows it. A failed read keeps the last good status and is read again while something shows it, when
// that status was busy or a daemon restart was just answered (the daemon is coming back); otherwise it is final, as an old daemon's
// 404 is.
func embeddingsRead(m Model, status EmbeddingsStatus, err error) (Model, []Cmd) {
	owed := m.Embeddings.Read && m.Embeddings.Err == nil && m.Embeddings.Status.RestartOwed()
	retry := err != nil && (m.Embeddings.Status.Busy() || m.Embeddings.returning)
	m.Embeddings.Read, m.Embeddings.Err = true, err
	if err == nil {
		m.Embeddings.Status, m.Embeddings.returning = status, false
	}
	m = m.withEmbeddingsRow().embeddingsCardRead()
	if !owed {
		m = m.askRestartOnArrival()
	}
	if (err == nil && status.Busy() || retry) && m.embeddingsWatched() {
		cmd := askEmbeddings(&m, embeddingsPollEvery)
		return m, []Cmd{cmd}
	}
	return m, nil
}
