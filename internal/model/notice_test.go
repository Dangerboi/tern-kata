package model

import (
	"errors"
	"testing"
)

func TestCannotNamesVerbThingAndReason(t *testing.T) {
	for _, tt := range []struct {
		verb, thing string
		err         error
		want        string
	}{
		{"undo", "", ErrDisconnected, "Cannot undo: disconnected from kata."},
		{"close", "#12", errors.New("issue not found."), "Cannot close #12: issue not found."},
		{"sweep", "", errors.New("first line\nsecond line"), "Cannot sweep: first line."},
		{"save", "the job", nil, "Cannot save the job: unknown error."},
		{"save", "", errors.New("  "), "Cannot save: unknown error."},
	} {
		if got := Cannot(tt.verb, tt.thing, tt.err); got != tt.want {
			t.Errorf("Cannot(%q, %q) = %q, want %q", tt.verb, tt.thing, got, tt.want)
		}
	}
}

func TestProgressAlwaysEndsInTheEllipsis(t *testing.T) {
	if got := Progress("Loading", "jobs"); got != "Loading jobs…" {
		t.Errorf("got %q", got)
	}
	if got := Progress("Saving", ""); got != "Saving…" {
		t.Errorf("got %q", got)
	}
}

func TestSayFailAndClearNotice(t *testing.T) {
	m := Model{}.say("Restored #12.")
	if m.Notice != "Restored #12." || m.NoticeKind != NoticeInfo {
		t.Fatalf("say: %q %v", m.Notice, m.NoticeKind)
	}
	m = m.fail(Cannot("undo", "", ErrDisconnected))
	if m.NoticeKind != NoticeFail {
		t.Fatalf("fail kind %v", m.NoticeKind)
	}
	if m = m.clearNotice(); m.Notice != "" || m.NoticeKind != NoticeInfo {
		t.Fatalf("clear: %q %v", m.Notice, m.NoticeKind)
	}
}

func TestKeyIsAKeycapMarker(t *testing.T) {
	if got := Key("W"); got != "{W}" {
		t.Errorf("got %q", got)
	}
}

func TestAToastKeepsTheKeyLabelOfItsChord(t *testing.T) {
	m, _ := Model{}.raiseToast(Toast{Text: "Closed #1.", Label: "Reopen", Chord: "r"})
	if m.Toast.Key == "" {
		t.Error("toast has no key label for its chord")
	}
	m, _ = m.raiseToast(Toast{Text: "Saved."})
	if m.Toast.Key != "" {
		t.Errorf("a toast with no chord has key %q", m.Toast.Key)
	}
}
