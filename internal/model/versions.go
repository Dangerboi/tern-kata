package model

import (
	"errors"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Versions is what the Pane knows of the programs on its session host. Running is this binary's version, set at launch; the
// rest is what the last version check found.
type Versions struct {
	Running string
	// Installed is the version of the binary now at the Pane's own path, "" while that is still the file the Pane started from.
	Installed string
	// KataCLI is the version of the kata on PATH, KataLatest the newest kata release the daily check found ("" when none is newer).
	KataCLI, KataLatest string
	checks              Latest
	// toasted are the notices already toasted, by text, so each version is toasted once.
	toasted []string
	// askRestart says the daemon notice's restart question is shown.
	askRestart bool
	// restarting says the daemon notice's restart is out.
	restarting bool
}

const (
	// ModeNoticeRestart is the daemon notice's question whether the Pane restarts the kata daemon now.
	ModeNoticeRestart          Mode   = "notice-restart"
	ConfirmNoticeRestartAction Action = "confirm-notice-restart"
	DeclineNoticeRestartAction Action = "decline-notice-restart"
)

// VersionCheckEvery is how often the Pane checks the session host's versions: the old-binary notice follows an update within it.
const VersionCheckEvery = time.Minute

// CheckVersions asks what the session host holds now, after After: the version of the binary at the Pane's path once that file
// changed, the kata CLI's version, the newest kata release (checked at most once a day per machine) and the daemon's version.
// Request numbers the checks; Connection is the Pane's connection when it asked, as the daemon's version belongs to it.
type CheckVersions struct {
	Workspace  string
	Request    int64
	Connection int
	After      time.Duration
}

func (CheckVersions) cmd() {}

// VersionsChecked answers CheckVersions. A version that could not be read is "".
type VersionsChecked struct {
	Request    int64
	Connection int
	Installed  string
	KataCLI    string
	KataLatest string
	Daemon     string
}

func (VersionsChecked) msg() {}

// NoticeKind names a version notice.
type NoticeKind int

const (
	NoticeOldBinary NoticeKind = iota
	NoticeOldDaemon
	NoticeKataRelease
)

// VersionNotice is a fact about the session host the user should act on. Headline names it and Action what to do, as the Menu
// entry shows them; Text says both, as the toast and the Notice line do.
type VersionNotice struct {
	Kind                   NoticeKind
	Headline, Action, Text string
}

// NoticeMarker ends the top bar's title while any version notice holds.
const NoticeMarker = "  ↑ update"

// stamped reports whether version is one the Pane can compare: a build without a stamped version ("dev") is not.
func stamped(version string) bool { return semver.IsValid(version) }

// newer reports whether a is a stamped version newer than the stamped version b.
func newer(a, b string) bool { return stamped(a) && stamped(b) && semver.Compare(a, b) > 0 }

// VersionNotices are the notices that hold now, in the Menu's order. The daemon's notice is for a local daemon only: a remote one
// is restarted on its own machine, where the session host's kata says nothing.
func (m Model) VersionNotices() []VersionNotice {
	v := m.Versions
	var notices []VersionNotice
	if stamped(v.Running) && stamped(v.Installed) && v.Installed != v.Running {
		notices = append(notices, VersionNotice{Kind: NoticeOldBinary, Headline: "tern-kata " + v.Installed + " is installed", Action: "reopen this Pane",
			Text: "tern-kata " + v.Installed + " is installed: reopen this Pane to use it."})
	}
	if d := m.Daemon; d.Kind == DaemonLocal && newer(v.KataCLI, d.Version) {
		notices = append(notices, VersionNotice{Kind: NoticeOldDaemon, Headline: "kata daemon " + d.Version + " is older than kata " + v.KataCLI, Action: "restart the daemon…",
			Text: "The kata daemon runs " + d.Version + " and kata " + v.KataCLI + " is installed: restart the daemon from the Menu to use it."})
	}
	if newer(v.KataLatest, v.KataCLI) {
		notices = append(notices, VersionNotice{Kind: NoticeKataRelease, Headline: "kata " + v.KataLatest + " is out", Action: "run kata update",
			Text: "kata " + v.KataLatest + " is out: run kata update to install it."})
	}
	return notices
}

// askVersions numbers a new check, which supersedes any still out, and asks for it after after.
func askVersions(m *Model, after time.Duration) Cmd {
	return CheckVersions{Workspace: m.Workspace, Request: m.Versions.checks.Ask(), Connection: m.Connection, After: after}
}

// versionsChecked takes a check's answer, toasts each notice the first time it holds, and asks for the next check. The daemon's
// version is taken only from the connection it was read on.
func versionsChecked(m Model, msg VersionsChecked) (Model, []Cmd) {
	v := &m.Versions
	if !v.checks.IsCurrent(msg.Request) {
		return m, nil
	}
	v.Installed, v.KataCLI, v.KataLatest = msg.Installed, msg.KataCLI, msg.KataLatest
	if msg.Daemon != "" && msg.Connection == m.Connection && m.Daemon.Kind != "" {
		m.Daemon.Version = msg.Daemon
	}
	cmds := []Cmd{askVersions(&m, VersionCheckEvery)}
	var fresh []string
	for _, notice := range m.VersionNotices() {
		if text := notice.Text; !slices.Contains(v.toasted, text) {
			v.toasted = append(slices.Clip(v.toasted), text)
			fresh = append(fresh, text)
		}
	}
	if len(fresh) > 0 {
		var wait Cmd
		m, wait = m.raiseToast(Toast{Text: strings.Join(fresh, " ")})
		cmds = append(cmds, wait)
	}
	return m, cmds
}

// noticeKeyPrefix keys a notice's Menu entry, followed by its NoticeKind.
const noticeKeyPrefix = "notice."

// NoticeOf is the notice of kind that holds now.
func (m Model) NoticeOf(kind NoticeKind) (VersionNotice, bool) {
	notices := m.VersionNotices()
	if i := slices.IndexFunc(notices, func(n VersionNotice) bool { return n.Kind == kind }); i >= 0 {
		return notices[i], true
	}
	return VersionNotice{}, false
}

// openNotice is a pick of a notice's Menu entry. The daemon's asks before it restarts the daemon; the others can only be done
// outside the Pane, so the Notice line repeats what to do.
func openNotice(m Model, kind NoticeKind) (Model, []Cmd) {
	notice, ok := m.NoticeOf(kind)
	switch {
	case !ok:
	case kind == NoticeOldDaemon:
		m.Versions.askRestart, m.HelpOpen = true, false
	default:
		m.Notice = notice.Text
	}
	return m, nil
}

// restartOutNotice is the refusal of a second restart while one is out; errRestartOut is the same on the search by meaning card,
// which the notice's restart clears when it ends.
const restartOutNotice = "The kata daemon is restarting; wait for it."

var errRestartOut = errors.New(restartOutNotice)

// forDaemon is what a daemon switch keeps: everything but the daemon notice's question and restart, whose answer the switch
// drops.
func (v Versions) forDaemon() Versions {
	v.askRestart, v.restarting = false, false
	return v
}

// confirmNoticeRestart is y to the daemon notice's question: the one restart goes out. One already out, for this notice or for
// search by meaning, is waited for.
func confirmNoticeRestart(m Model) (Model, []Cmd) {
	m.Versions.askRestart = false
	switch {
	case m.Versions.restarting || m.Embeddings.Restarting:
		m.Notice = restartOutNotice
		return m, nil
	case m.Stale:
		m.Notice = "Daemon disconnected; reconnect before restarting the kata daemon."
		return m, nil
	}
	m.Versions.restarting = true
	m.Notice = "Restarting the kata daemon…"
	return m, []Cmd{RestartKataDaemon{Workspace: m.Workspace, Connection: m.Connection}}
}

// noticeRestartDone takes the answer to the daemon notice's restart. Success checks the versions at once, which reads the new
// daemon's version and so clears the notice, and reads search by meaning's status again, which a restart may have settled, as
// the card's own restart does; a failure says why and leaves the notice. Either way the card stops saying a restart is out.
func noticeRestartDone(m Model, msg KataDaemonRestarted) (Model, []Cmd) {
	m.Versions.restarting = false
	if errors.Is(m.EmbeddingsCard.Err, errRestartOut) {
		m.EmbeddingsCard.Err = nil
	}
	if msg.Err != nil {
		m.Notice = "The kata daemon was not restarted: " + oneLine(msg.Err.Error())
		return m, nil
	}
	m.Notice = "The kata daemon restarted."
	cmds := []Cmd{askVersions(&m, 0)}
	if m.Embeddings.Read {
		m.Embeddings.returning = true
		cmds = append(cmds, askEmbeddings(&m, 0))
	}
	return m, cmds
}
