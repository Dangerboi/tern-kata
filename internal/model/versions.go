package model

import (
	"errors"
	"slices"
	"strconv"
	"time"

	"golang.org/x/mod/semver"
)

// Versions is what the Pane knows of the programs on its session host. Running is this binary's version, set at launch; the
// rest is what the last version check found.
type Versions struct {
	Running string
	// Installed is the version of the binary now at the Pane's own path, "" while that is still the file the Pane started from.
	Installed string
	// KataCLI is the version of the kata on PATH, KataLatest the newest kata release the hourly check found ("" when none is newer).
	KataCLI, KataLatest string
	checks              Latest
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
// changed, the kata CLI's version, the newest kata release (checked at most once an hour per machine) and the daemon's version.
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

// VersionNoticeKind names a version notice.
type VersionNoticeKind int

const (
	NoticeOldBinary VersionNoticeKind = iota
	NoticeOldDaemon
	NoticeKataRelease
)

// VersionNotice is a fact about the session host the user should act on. Subject names what has a new version, as the top bar's
// chip does; Headline says it and Action what to do, as the Menu entry shows them; Text says both, as the Notice line does.
type VersionNotice struct {
	Kind                            VersionNoticeKind
	Subject, Headline, Action, Text string
}

// VersionChip is the top bar's one marker for the notices that hold: "↑ kata v0.24.0" for one, "↑ 2 updates" for several, empty
// for none. A click on it opens the Menu, which names each notice.
func (m Model) VersionChip() string {
	notices := m.VersionNotices()
	switch len(notices) {
	case 0:
		return ""
	case 1:
		return "↑ " + notices[0].Subject
	}
	return "↑ " + strconv.Itoa(len(notices)) + " updates"
}

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
		subject := "tern-kata " + v.Installed
		notices = append(notices, VersionNotice{Kind: NoticeOldBinary, Subject: subject, Headline: subject + " is available", Action: "Reopen this Pane",
			Text: subject + " is available. Reopen this Pane to use it."})
	}
	if d := m.Daemon; d.Kind == DaemonLocal && newer(v.KataCLI, d.Version) {
		notices = append(notices, VersionNotice{Kind: NoticeOldDaemon, Subject: "kata daemon", Headline: "kata daemon " + d.Version + " is older than kata " + v.KataCLI, Action: "Restart the daemon…",
			Text: "The kata daemon runs " + d.Version + " and kata " + v.KataCLI + " is installed. Restart the daemon from the Menu to use it."})
	}
	if newer(v.KataLatest, v.KataCLI) {
		subject := "kata " + v.KataLatest
		notices = append(notices, VersionNotice{Kind: NoticeKataRelease, Subject: subject, Headline: subject + " is available", Action: "kata update",
			Text: subject + " is available. Run kata update to install it."})
	}
	return notices
}

// askVersions numbers a new check, which supersedes any still out, and asks for it after after.
func askVersions(m *Model, after time.Duration) Cmd {
	return CheckVersions{Workspace: m.Workspace, Request: m.Versions.checks.Ask(), Connection: m.Connection, After: after}
}

// versionsChecked takes a check's answer and asks for the next check; the top bar's chip shows the notices that now hold. The daemon's
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
	return m, []Cmd{askVersions(&m, VersionCheckEvery)}
}

// noticeKeyPrefix keys a notice's Menu entry, followed by its VersionNoticeKind.
const noticeKeyPrefix = "notice."

// NoticeOf is the notice of kind that holds now.
func (m Model) NoticeOf(kind VersionNoticeKind) (VersionNotice, bool) {
	notices := m.VersionNotices()
	if i := slices.IndexFunc(notices, func(n VersionNotice) bool { return n.Kind == kind }); i >= 0 {
		return notices[i], true
	}
	return VersionNotice{}, false
}

// openNotice is a pick of a notice's Menu entry. The daemon's asks before it restarts the daemon; the others can only be done
// outside the Pane, so the Notice line repeats what to do.
func openNotice(m Model, kind VersionNoticeKind) (Model, []Cmd) {
	notice, ok := m.NoticeOf(kind)
	switch {
	case !ok:
	case kind == NoticeOldDaemon:
		m.Versions.askRestart, m.HelpOpen = true, false
	default:
		m = m.say(notice.Text)
	}
	return m, nil
}

// errRestartOut is the refusal of a second restart while one is out, as the search by meaning card says it; the notice's restart
// clears it when it ends. The Notice line words the same refusal with errRestartPending.
var errRestartOut = errors.New("The kata daemon is restarting; wait for it.")

var errRestartPending = errors.New("a restart is already out")

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
		return m.fail(Cannot("restart", "the kata daemon", errRestartPending)), nil
	case m.Stale:
		return m.fail(Cannot("restart", "the kata daemon", ErrDisconnected)), nil
	}
	m.Versions.restarting = true
	m = m.say(Progress("Restarting", "the kata daemon"))
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
		return m.fail(Cannot("restart", "the kata daemon", msg.Err)), nil
	}
	m = m.say("The kata daemon restarted.")
	cmds := []Cmd{askVersions(&m, 0)}
	if m.Embeddings.Read {
		m.Embeddings.returning = true
		cmds = append(cmds, askEmbeddings(&m, 0))
	}
	return m, cmds
}
