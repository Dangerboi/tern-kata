package model

import (
	"errors"
	"strings"
)

// The Search by meaning card turns search by meaning on through kata and follows kata's setup job. It is one Popup card with no
// state of its own about the job: what it shows is the status the Pane last read (Model.Embeddings), so a card closed mid-job and
// opened again picks the job up. The card only remembers that its question is on screen, the refusal of the last turn-on, and
// that a status it waits for should open on the question.

const (
	// ModeEmbeddingsAsk is the question: y turns it on, n or esc closes the card.
	ModeEmbeddingsAsk Mode = "embeddings-ask"
	// ModeEmbeddings is the card following the setup, or showing a state with nothing to do: esc closes it.
	ModeEmbeddings Mode = "embeddings"
	// ModeEmbeddingsOff is the card while kata would take a turn-on again (a failed job, or a refusal on a status that was out of
	// date): s asks the question again.
	ModeEmbeddingsOff Mode = "embeddings-off"
	// ModeEmbeddingsRestart is the question whether the Pane restarts the kata daemon now: y restarts it, n or esc closes the card.
	ModeEmbeddingsRestart Mode = "embeddings-restart"
)

const (
	TurnOnEmbeddingsConfirm Action = "turn-on-embeddings-confirm"
	TurnOnEmbeddingsDecline Action = "turn-on-embeddings-decline"
	TurnOnEmbeddingsAsk     Action = "turn-on-embeddings-ask"
	EmbeddingsCardClose     Action = "embeddings-card-close"
	RestartDaemonConfirm    Action = "restart-daemon-confirm"
	RestartDaemonDecline    Action = "restart-daemon-decline"
	// ConfigRestartDaemon is r on the Config row that owes a restart.
	ConfigRestartDaemon Action = "config-restart-daemon"
)

// TurnOnEmbeddings asks the daemon the Pane is connected to to start its local search by meaning setup job: the user said yes to
// the card's question. kata answers 202 with the status, 403 without daemon administration, 409 while a job runs or embeddings are
// already configured.
type TurnOnEmbeddings struct {
	Workspace string
	// Connection is the Pane's connection number when it asked: an answer from an earlier daemon is dropped.
	Connection int
}

func (TurnOnEmbeddings) cmd() {}

// EmbeddingsTurnedOn answers TurnOnEmbeddings with the status kata starts with, or Err: kata's reason for the refusal. Connection
// is the asking command's.
type EmbeddingsTurnedOn struct {
	Status     EmbeddingsStatus
	Err        error
	Connection int
}

func (EmbeddingsTurnedOn) msg() {}

// RestartQuestion is what the card asks before the Pane restarts the kata daemon it is connected to.
const RestartQuestion = "Restart the kata daemon now? Other Panes, agents and the worker lose their connection for a moment and reconnect. y/n"

// RestartKataDaemon asks for `kata daemon restart` on the daemon the Pane is connected to: the user said yes to the question.
type RestartKataDaemon struct {
	Workspace string
	// Connection is the Pane's connection number when it asked: an answer from an earlier daemon is dropped.
	Connection int
}

func (RestartKataDaemon) cmd() {}

// KataDaemonRestarted answers RestartKataDaemon. Err is kata's last stderr line, or why the restart did not run.
type KataDaemonRestarted struct {
	Err        error
	Connection int
}

func (KataDaemonRestarted) msg() {}

// EmbeddingsCardState is the Search by meaning card. Asking says the question is shown. Err is kata's refusal of the last turn-on.
type EmbeddingsCardState struct {
	Open   bool
	Asking bool
	// AskingRestart says the restart question is shown.
	AskingRestart bool
	Err           error
}

// EmbeddingsQuestion is what the card asks before anything starts, worded from the status.
func EmbeddingsQuestion(s EmbeddingsStatus) string {
	const lead = "Turn on search by meaning? "
	switch {
	case s.OllamaAvailable:
		return lead + "kata will use your Ollama; nothing to download. y/n"
	case s.DownloadBytes == 0:
		return lead + "kata will use its built-in model, already on the kata daemon's machine; nothing to download. Search stays by words until the daemon restarts. y/n"
	}
	where := "this machine"
	if s.Remote {
		where = "another machine"
		if s.Host != "" {
			where = s.Host
		}
	}
	return lead + "kata downloads about " + megabytes(s.DownloadBytes) + " MB (shared by all Kenn apps) to the kata daemon's machine (" + where +
		"). Search stays by words until the daemon restarts. y/n"
}

// EmbeddingsProgress is a bar of the card. Fraction is nil when kata gives no total, which draws a bar that sweeps.
type EmbeddingsProgress struct {
	Fraction *float64
}

func progressOf(done, total int64) EmbeddingsProgress {
	if total <= 0 {
		return EmbeddingsProgress{}
	}
	f := min(1, max(0, float64(done)/float64(total)))
	return EmbeddingsProgress{Fraction: &f}
}

// EmbeddingsCardBody is what the card says, in the order it says it. State is the line about the status; Failed and Pending tone
// it. Download and Indexing are the bars. KeepsGoing says kata continues when the card is closed.
type EmbeddingsCardBody struct {
	Question   string
	State      string
	Failed     bool
	Pending    bool
	Download   *EmbeddingsProgress
	Indexing   *EmbeddingsProgress
	KeepsGoing bool
	// Err is kata's reason for refusing the last turn-on.
	Err string
}

// EmbeddingsCardBody is what the open card says for the status last read.
func (m Model) EmbeddingsCardBody() EmbeddingsCardBody {
	var b EmbeddingsCardBody
	if err := m.EmbeddingsCard.Err; err != nil {
		b.Err = oneLine(err.Error())
	}
	s := m.Embeddings
	switch {
	case !s.Read:
		b.State, b.Pending = "Reading search by meaning from the kata daemon…", true
	case s.Restarting:
		b.State, b.Pending = "Restarting the kata daemon…", true
	case s.Err != nil:
		b.State, b.Failed = "kata cannot report search by meaning here: "+oneLine(s.Err.Error()), true
	case s.Turning:
		b.State, b.Pending = "Turning it on…", true
	case m.EmbeddingsCard.AskingRestart:
		b.Question = RestartQuestion
	case m.EmbeddingsCard.Asking:
		b.Question = EmbeddingsQuestion(s.Status)
	default:
		st := s.Status
		b.State, b.Failed = st.Words(), st.failed()
		if st.Job != nil && st.Job.State == EmbeddingsDownloading {
			p := progressOf(st.Job.Downloaded, st.Job.Total)
			b.Download = &p
		}
		if st.Job == nil && st.Provider != EmbeddingsOff && st.Indexing != nil && !st.Indexing.Failed {
			p := progressOf(st.Indexing.Done, st.Indexing.Total)
			b.Indexing = &p
		}
		b.KeepsGoing = st.Busy()
	}
	return b
}

// EmbeddingsCardMode is the keymap mode of the open card.
func (m Model) EmbeddingsCardMode() Mode {
	switch {
	case m.EmbeddingsCard.AskingRestart:
		return ModeEmbeddingsRestart
	case m.EmbeddingsCard.Asking:
		return ModeEmbeddingsAsk
	case m.embeddingsAskable():
		return ModeEmbeddingsOff
	}
	return ModeEmbeddings
}

// embeddingsAskable says kata would take a turn-on now, by the status last read, and none is out already.
func (m Model) embeddingsAskable() bool {
	s := m.Embeddings
	return s.Read && s.Err == nil && !s.Turning && s.Status.TurnOnOffered()
}

// openEmbeddingsCard opens the Search by meaning card, from the Config row or from anywhere else. Its first state asks, when kata
// would take a turn-on and the setup is not already under way, or asks to restart the daemon when a restart is owed and the daemon
// is local; otherwise it shows the status. Every opener has read a status already. One that could not be read or that is still busy
// is read again, which also starts the card's polling.
func openEmbeddingsCard(m Model) (Model, []Cmd) {
	m.HelpOpen = false
	m.EmbeddingsCard = EmbeddingsCardState{Open: true}
	// A failed job is read first, which shows kata's error; s asks again.
	m.EmbeddingsCard.Asking = m.embeddingsAskable() && m.Embeddings.Status.Job == nil
	m.EmbeddingsCard.AskingRestart = m.restartOffered()
	if m.Embeddings.Err != nil || m.Embeddings.Status.Busy() {
		return m, []Cmd{askEmbeddings(&m, 0)}
	}
	return m, nil
}

// turnOnEmbeddings is s on the Config row: the card opens on the question, also after a failed job.
func turnOnEmbeddings(m Model) (Model, []Cmd) {
	m, cmds := openEmbeddingsCard(m)
	if m.embeddingsAskable() {
		m.EmbeddingsCard.Asking = true
	}
	return m, cmds
}

// embeddingsCardRead settles the card for a status that has just been read: it drops the question when kata would no longer take a
// turn-on.
func (m Model) embeddingsCardRead() Model {
	c := &m.EmbeddingsCard
	if !c.Open {
		return m
	}
	c.Asking = c.Asking && m.embeddingsAskable()
	c.AskingRestart = c.AskingRestart && m.restartOffered()
	return m
}

// embeddingsCardKey is a key of the open card.
func embeddingsCardKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, ok := LookupBindingForMode(msg.Chord, m.EmbeddingsCardMode())
	if !ok {
		return m, nil
	}
	switch binding.Action {
	case TurnOnEmbeddingsConfirm:
		return confirmTurnOnEmbeddings(m)
	case TurnOnEmbeddingsAsk:
		m.EmbeddingsCard.Asking, m.EmbeddingsCard.Err = true, nil
	case TurnOnEmbeddingsDecline, EmbeddingsCardClose:
		m.EmbeddingsCard = EmbeddingsCardState{}
	case RestartDaemonConfirm:
		return confirmRestartDaemon(m)
	case RestartDaemonDecline:
		m.EmbeddingsCard = EmbeddingsCardState{}
	}
	return m, nil
}

// confirmTurnOnEmbeddings is y: the one POST goes out, and the card then follows what kata reports. A second y while it is out
// sends nothing.
func confirmTurnOnEmbeddings(m Model) (Model, []Cmd) {
	if !m.EmbeddingsCard.Asking || m.Embeddings.Turning {
		return m, nil
	}
	if m.Stale {
		m.EmbeddingsCard.Err = errors.New("Daemon disconnected; reconnect before turning on search by meaning.")
		return m, nil
	}
	m.EmbeddingsCard.Asking, m.EmbeddingsCard.Err = false, nil
	m.Embeddings.Turning = true
	return m, []Cmd{TurnOnEmbeddings{Workspace: m.Workspace, Connection: m.Connection}}
}

// embeddingsFailed shows text where the Pane is looking: on the card, or on the Config Page's message line, or as a Notice when
// neither is shown.
func (m Model) embeddingsFailed(text string) Model {
	switch {
	case m.EmbeddingsCard.Open:
		m.EmbeddingsCard.Err = errors.New(text)
	case m.OnConfig():
		m.ConfigPage.Message, m.ConfigPage.MessageErr = text, true
	default:
		m.Notice = text
	}
	return m
}

// embeddingsTurnedOn takes kata's answer to the turn-on. A refusal (403, 409) or a failure to reach the daemon is reported by
// embeddingsFailed, and the status is read again, as it has changed under the card or never allowed this. kata's 202 carries the
// status the job starts with, which is read like any other, so polling begins while the card or the Config Page shows it.
func embeddingsTurnedOn(m Model, msg EmbeddingsTurnedOn) (Model, []Cmd) {
	m.Embeddings.Turning = false
	if msg.Err != nil {
		m.EmbeddingsCard.Asking = false
		m = m.embeddingsFailed("Search by meaning was not turned on: " + oneLine(msg.Err.Error()))
		return m, []Cmd{askEmbeddings(&m, 0)}
	}
	m.Embeddings.reads.Discard()
	return embeddingsRead(m, msg.Status, nil)
}

// restartOffered says the Pane may restart the daemon: kata finished the setup, the daemon is local, and no restart is out.
func (m Model) restartOffered() bool {
	s := m.Embeddings
	return s.Read && s.Err == nil && !s.Restarting && s.Status.RestartOwed() && !s.Status.Remote
}

// askRestartOnArrival opens the restart question on a card that is showing when the status has just reached the restart state.
func (m Model) askRestartOnArrival() Model {
	if m.EmbeddingsCard.Open && m.restartOffered() {
		m.EmbeddingsCard.Asking, m.EmbeddingsCard.AskingRestart, m.EmbeddingsCard.Err = false, true, nil
	}
	return m
}

// confirmRestartDaemon is y: the one restart goes out. A second y while it is out sends nothing.
func confirmRestartDaemon(m Model) (Model, []Cmd) {
	if !m.EmbeddingsCard.AskingRestart || m.Embeddings.Restarting {
		return m, nil
	}
	if m.Stale {
		m.EmbeddingsCard.Err = errors.New("Daemon disconnected; reconnect before restarting the kata daemon.")
		return m, nil
	}
	m.EmbeddingsCard.AskingRestart, m.EmbeddingsCard.Err = false, nil
	m.Embeddings.Restarting = true
	m = m.withEmbeddingsRow()
	return m, []Cmd{RestartKataDaemon{Workspace: m.Workspace, Connection: m.Connection}}
}

// kataDaemonRestarted takes the restart's answer. Success reads the status again, which shows the indexing that follows. A
// failure is reported by embeddingsFailed.
func kataDaemonRestarted(m Model, msg KataDaemonRestarted) (Model, []Cmd) {
	if !m.Embeddings.Restarting {
		return m, nil
	}
	m.Embeddings.Restarting = false
	m = m.withEmbeddingsRow()
	if msg.Err == nil {
		m.Embeddings.returning = true
		return m, []Cmd{askEmbeddings(&m, 0)}
	}
	return m.embeddingsFailed("The kata daemon was not restarted: " + oneLine(msg.Err.Error())), nil
}

// updateEmbeddings takes the answers about search by meaning, whichever Page is shown.
func updateEmbeddings(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case EmbeddingsStatusLoaded:
		m, cmds := embeddingsLoaded(m, msg)
		return m, cmds, true
	case EmbeddingsTurnedOn:
		m, cmds := embeddingsTurnedOn(m, msg)
		return m, cmds, true
	case KataDaemonRestarted:
		m, cmds := kataDaemonRestarted(m, msg)
		return m, cmds, true
	}
	return m, nil, false
}

// EmbeddingsCardTitle names the daemon the card is about, as the Config row does.
func (m Model) EmbeddingsCardTitle() string { return daemonTitle(m.Daemon.Name) }

// daemonTitle is "kata daemon: name", the daemon named "default" when its name is empty. The card's title and the Config row's
// title are both built from it.
func daemonTitle(name string) string {
	if name = strings.TrimSpace(oneLine(name)); name == "" {
		name = "default"
	}
	return "kata daemon: " + name
}
